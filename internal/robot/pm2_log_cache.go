package robot

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"os"
	"sync"
)

// Content-addressed blocks are shared by history and SSE snapshots. Identity,
// size and mtime alone cannot distinguish append from truncate-and-regrow.
const pm2ParsedCacheBudget = 128 << 20
const pm2ParsedBlockLines = 512
const pm2ParsedBlockBytes = 256 << 10

type pm2ParsedBlock struct {
	key   [32]byte
	lines []pm2AuditLogLine // immutable; readers copy the structs before sorting
	bytes int
}

type pm2BlockCache struct {
	sync.Mutex
	entries map[[32]byte]*list.Element
	lru     list.List
	bytes   int
	budget  int
}

var pm2ParsedCache = pm2BlockCache{entries: make(map[[32]byte]*list.Element), budget: pm2ParsedCacheBudget}

func (c *pm2BlockCache) get(key [32]byte) ([]pm2AuditLogLine, bool) {
	c.Lock()
	defer c.Unlock()
	if element := c.entries[key]; element != nil {
		c.lru.MoveToFront(element)
		return element.Value.(pm2ParsedBlock).lines, true
	}
	return nil, false
}

func (c *pm2BlockCache) put(key [32]byte, lines []pm2AuditLogLine) {
	cost := cap(lines)*96 + 256
	for _, line := range lines {
		cost += len(line.Text)
	}
	if cost > c.budget {
		return
	}
	c.Lock()
	defer c.Unlock()
	if element := c.entries[key]; element != nil {
		c.lru.MoveToFront(element)
		return
	}
	for c.bytes+cost > c.budget {
		oldest := c.lru.Back()
		block := oldest.Value.(pm2ParsedBlock)
		delete(c.entries, block.key)
		c.bytes -= block.bytes
		c.lru.Remove(oldest)
	}
	c.entries[key] = c.lru.PushFront(pm2ParsedBlock{key: key, lines: lines, bytes: cost})
	c.bytes += cost
}

func parsePM2LogBlock(texts []string) []pm2AuditLogLine {
	size := len(texts) * 8
	for _, text := range texts {
		size += len(text)
	}
	content := make([]byte, 0, size)
	for _, text := range texts {
		content = binary.LittleEndian.AppendUint64(content, uint64(len(text)))
		content = append(content, text...)
	}
	key := sha256.Sum256(content)
	if lines, ok := pm2ParsedCache.get(key); ok {
		return lines
	}
	lines := make([]pm2AuditLogLine, len(texts))
	for i, text := range texts {
		ts, has := parsePM2LogTimestamp(text)
		lines[i] = pm2AuditLogLine{Text: text, Timestamp: ts, HasTime: has, diagnosticMask: pm2DiagnosticMask(text)}
	}
	pm2ParsedCache.put(key, lines)
	return lines
}

// pm2LogTailStart finds the last N physical lines without parsing the discarded
// prefix. A trailing newline terminates a record; it is not an extra record.
func pm2LogTailStart(file io.ReaderAt, size int64, limit int) (int64, error) {
	return pm2LogTailStartContext(context.Background(), file, size, limit)
}

func pm2LogTailStartContext(ctx context.Context, file io.ReaderAt, size int64, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	buffer := make([]byte, 64*1024)
	remaining := limit
	for end := size; end > 0; {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		start := end - int64(len(buffer))
		if start < 0 {
			start = 0
		}
		n, err := file.ReadAt(buffer[:end-start], start)
		if err != nil && err != io.EOF {
			return 0, err
		}
		for i := n - 1; i >= 0; i-- {
			if buffer[i] != '\n' || start+int64(i) == size-1 {
				continue
			}
			remaining--
			if remaining == 0 {
				return start + int64(i) + 1, nil
			}
		}
		end = start
	}
	return 0, nil
}

func readCachedPM2LogFile(ctx context.Context, path string, maxLines int) ([]pm2AuditLogLine, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	start, err := pm2LogTailStartContext(ctx, file, info.Size(), maxLines)
	if err != nil {
		return nil, false, err
	}
	lines, cut, err := readPM2LogReader(pm2ContextReader{ctx, io.NewSectionReader(file, start, info.Size()-start)}, maxLines)
	return lines, cut || start > 0, err
}

// Check cancellation between buffered reads, including during timestamp parsing.
type pm2ContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r pm2ContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
