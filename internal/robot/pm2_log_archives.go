package robot

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const pm2ArchiveLayout = "2006-01-02_15-04-05"

// Find archives by exact current filename and known rotation patterns.
// A sibling that is itself a registered log takes precedence.
func pm2LogArchives(ctx context.Context, bases []pm2LogFile, registered []PM2Process, limit int) ([]pm2LogFile, bool, error) {
	type archive struct {
		file    pm2LogFile
		rotated time.Time
	}
	excluded := make(map[string]bool)
	for _, process := range registered {
		for _, path := range []string{process.OutputLog, process.ErrorLog} {
			if filepath.IsAbs(path) {
				excluded[pm2AbsolutePath("", path)] = true
				if resolved, err := filepath.EvalSymlinks(path); err == nil {
					excluded[pm2AbsolutePath("", resolved)] = true
				}
			}
		}
	}
	for _, base := range bases {
		excluded[pm2AbsolutePath("", base.Path)] = true
	}
	directories := make(map[string][]os.DirEntry)
	var archives []archive
	incomplete := false
	for _, base := range bases {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		dir, name := filepath.Split(base.Path)
		ext := filepath.Ext(name)
		if !strings.EqualFold(ext, ".log") {
			continue
		}
		entries, found := directories[dir]
		if !found {
			var err error
			entries, err = os.ReadDir(dir)
			incomplete = incomplete || (err != nil && !os.IsNotExist(err))
			directories[dir] = entries
		}
		for i, entry := range entries {
			if i%256 == 0 && ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			if !entry.Type().IsRegular() {
				continue
			}
			stamp, ok := pm2ArchiveTime(name, ext, entry)
			if !ok {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			key := pm2AbsolutePath("", path)
			if excluded[key] {
				continue
			}
			excluded[key] = true
			archives = append(archives, archive{file: pm2LogFile{Source: base.Source, Path: path}, rotated: stamp})
		}
	}
	// Read in increasing rotation order so untimed records retain file order.
	sort.Slice(archives, func(i, j int) bool {
		if archives[i].rotated.Equal(archives[j].rotated) {
			return archives[i].file.Path < archives[j].file.Path
		}
		return archives[i].rotated.Before(archives[j].rotated)
	})
	if len(archives) > limit {
		archives = archives[len(archives)-limit:]
		incomplete = true
	}
	files := make([]pm2LogFile, 0, len(archives))
	for _, archive := range archives {
		files = append(files, archive.file)
	}
	return files, incomplete, ctx.Err()
}

func pm2ArchiveTime(base, ext string, entry os.DirEntry) (time.Time, bool) {
	name := strings.TrimSuffix(entry.Name(), ".gz")
	if stem, ok := strings.CutSuffix(name, ext); ok {
		prefix := strings.TrimSuffix(base, ext) + "__"
		if suffix, ok := strings.CutPrefix(stem, prefix); ok && len(suffix) == len(pm2ArchiveLayout) {
			stamp, err := time.ParseInLocation(pm2ArchiveLayout, suffix, time.Local)
			return stamp, err == nil
		}
	}
	if suffix, ok := strings.CutPrefix(name, base+"."); ok && suffix != "" && suffix[0] >= '1' && suffix[0] <= '9' {
		if _, err := strconv.ParseUint(suffix, 10, 32); err == nil {
			if info, err := entry.Info(); err == nil {
				return info.ModTime(), true
			}
		}
	}
	return time.Time{}, false
}

func readCompressedPM2Log(ctx context.Context, file *os.File) ([]pm2AuditLogLine, bool, error) {
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, false, err
	}
	defer gz.Close()
	// Bound decompression. A clipped gzip member contributes complete lines
	// from its prefix, with a truncation flag so the caller can see the gap.
	data, err := io.ReadAll(io.LimitReader(pm2ContextReader{ctx, gz}, diagnosticLogBytes+1))
	if err != nil {
		return nil, false, err
	}
	truncated := int64(len(data)) > diagnosticLogBytes
	if truncated {
		end := bytes.LastIndexByte(data[:diagnosticLogBytes], '\n') + 1
		data = data[:end]
	}
	lines, cut, err := readPM2LogReader(pm2ContextReader{ctx, bytes.NewReader(data)}, diagnosticLogLines)
	return lines, cut || truncated, err
}
