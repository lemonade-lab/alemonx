package robot

import (
	"container/list"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestPM2FileTailMatchesScanner(t *testing.T) {
	for _, input := range []string{"", "\n", "a", "a\n", "a\nb", "a\nb\n", "a\n\nb\n", "a\r\nb\r\n", strings.Repeat("long line ", 10000) + "\nlast\n"} {
		for _, limit := range []int{0, 1, 2, 3} {
			path := filepath.Join(t.TempDir(), "log")
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			want, wantCut, wantErr := readPM2LogReader(strings.NewReader(input), limit)
			got, cut, err := readPM2LogFile(path, limit)
			if err != wantErr || cut != wantCut || !reflect.DeepEqual(got, want) {
				t.Fatalf("length=%d limit=%d got=%v cut=%v err=%v want=%v cut=%v", len(input), limit, got, cut, err, want, wantCut)
			}
		}
	}
}

func TestPM2ParsedCacheLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	check := func(want string) {
		t.Helper()
		got, cut, err := readPM2LogFile(path, pm2LogMaxLines)
		expected, _, _ := readPM2LogReader(strings.NewReader(want), pm2LogMaxLines)
		if err != nil || cut || !reflect.DeepEqual(got, expected) {
			t.Fatalf("got=%v cut=%v err=%v want=%v", got, cut, err, expected)
		}
		// Audit callers must not mutate the cached slice.
		if len(got) > 0 {
			got[0].Text = "mutated"
		}
	}
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	appendLog := func(value string) {
		t.Helper()
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err = f.WriteString(value); err != nil {
			t.Fatal(err)
		}
	}
	write("first\n")
	check("first\n")
	check("first\n")
	appendLog("second\n")
	check("first\nsecond\n")
	appendLog("partial")
	check("first\nsecond\npartial")
	appendLog(" completed\n")
	check("first\nsecond\npartial completed\n")
	write("short\n")
	check("short\n")
	write("other\n")
	check("other\n")
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	write("rotated\n")
	check("rotated\n")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readPM2LogFile(path, pm2LogMaxLines); !os.IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
}

func BenchmarkPM2AuditPage(b *testing.B) {
	for _, unique := range []bool{false, true} {
		name := "repeated"
		if unique {
			name = "unique"
		}
		b.Run(name, func(b *testing.B) {
			var data strings.Builder
			for i := 0; i < 100000; i++ {
				if unique {
					fmt.Fprintf(&data, "[2026-10-26 12:00:00][INFO] record %d\n", i)
				} else {
					data.WriteString("[2026-10-26 12:00:00][INFO] ordinary record\n")
				}
			}
			path := filepath.Join(b.TempDir(), "log")
			if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
				b.Fatal(err)
			}
			if _, _, err := readPM2LogFile(path, pm2LogMaxLines); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lines, cut, err := readPM2LogFile(path, pm2LogMaxLines)
				if err != nil {
					b.Fatal(err)
				}
				sortPM2AuditLines(lines)
				if _, err := buildPM2AuditPage(lines, []string{"out"}, cut, PM2AuditQuery{PerPage: 120}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestPM2CancelledRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := readCachedPM2LogFile(ctx, "unused", pm2LogMaxLines); !errors.Is(err, context.Canceled) {
		t.Fatalf("read: %v", err)
	}
	if _, err := pm2LogTailStartContext(ctx, strings.NewReader("abc\n"), 4, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("tail: %v", err)
	}
}

type pm2CountingReader struct {
	*strings.Reader
	read int
}

func (r *pm2CountingReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := r.Reader.ReadAt(p, off)
	r.read += n
	return n, err
}
func TestPM2TailSkipsLargePrefix(t *testing.T) {
	input := strings.Repeat("old line\n", 100000) + "recent\nlatest\n"
	reader := &pm2CountingReader{Reader: strings.NewReader(input)}
	start, err := pm2LogTailStart(reader, int64(len(input)), 2)
	if err != nil || input[start:] != "recent\nlatest\n" {
		t.Fatalf("start=%d err=%v", start, err)
	}
	if reader.read > 64*1024 {
		t.Fatalf("read discarded prefix: %d bytes", reader.read)
	}
}

func TestPM2RewriteRegrowDoesNotReusePrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("old\nsame\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readPM2LogFile(path, pm2LogMaxLines); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new\nsame\nmore\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lines, _, err := readPM2LogFile(path, pm2LogMaxLines)
	if err != nil {
		t.Fatal(err)
	}
	if lines[0].Text != "new" {
		t.Fatalf("returned stale prefix: %q", lines[0].Text)
	}
}

func TestPM2BlockCacheBudgetAndConcurrentAccess(t *testing.T) {
	cache := pm2BlockCache{entries: make(map[[32]byte]*list.Element), budget: 1024}
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for i := 0; i < 100; i++ {
				text := fmt.Sprintf("worker %d record %d", worker, i)
				key := sha256.Sum256([]byte(text))
				cache.put(key, []pm2AuditLogLine{{Text: text}})
				if lines, ok := cache.get(key); ok && lines[0].Text != text {
					t.Errorf("wrong cached text")
				}
			}
		}(worker)
	}
	group.Wait()
	if cache.bytes > cache.budget || cache.lru.Len() != len(cache.entries) {
		t.Fatalf("cache budget/accounting: %d", cache.bytes)
	}
	huge := strings.Repeat("x", 2048)
	key := sha256.Sum256([]byte(huge))
	cache.put(key, []pm2AuditLogLine{{Text: huge}})
	if _, ok := cache.get(key); ok {
		t.Fatal("oversized block was retained")
	}
}

func TestPM2CachedDiagnosticsMatchFreshRules(t *testing.T) {
	input := strings.Repeat("[2026-10-26 12:00:00] ordinary\n", pm2ParsedBlockLines) +
		"[2026-10-26 12:01:00] [Yunzai] Worker started\n" +
		"[2026-10-26 12:02:00] Cannot find module 'new-module'\n" +
		"[2026-10-26 12:03:00] {\"token\":\"test-secret\"}\n"
	for pass := 0; pass < 2; pass++ {
		lines, _, err := readPM2LogReader(strings.NewReader(input), pm2LogMaxLines)
		if err != nil {
			t.Fatal(err)
		}
		sortPM2AuditLines(lines)
		got := diagnosePM2LogLines(lines)
		for i := range lines {
			lines[i].diagnosticMask = 0
		}
		want := diagnosePM2LogLines(lines)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("cached=%v fresh=%v", got, want)
		}
		codes := diagnosticCodes(got)
		if codes["plugin-module-missing"].Count != 1 || codes["sensitive-login-data"].Count != 1 {
			t.Fatalf("missing diagnostics: %v", got)
		}
	}
}

func TestPM2RedactionFastPathPreservesPatterns(t *testing.T) {
	reference := func(text string) string {
		text = pm2SensitiveJSONField.ReplaceAllString(text, `${1}[REDACTED]${3}`)
		text = pm2SensitiveJSField.ReplaceAllString(text, `${1}[REDACTED]${3}`)
		text = pm2SensitiveTokenPair.ReplaceAllString(text, `${1}[REDACTED]`)
		return pm2SensitiveDataPair.ReplaceAllString(text, `${1}[REDACTED]`)
	}
	for _, field := range append(strings.Split(pm2SensitiveFieldNames, "|"), "access-token", "refresh-token", "accessToken", "toKen", "acceſs_token") {
		for _, text := range []string{fmt.Sprintf(`{"%s":"secret-value-123"}`, field), fmt.Sprintf("'%s':'secret-value-123'", strings.ToUpper(field)), field + "=secret-value-123"} {
			if got, want := redactPM2LogText(text), reference(text); got != want {
				t.Fatalf("redaction mismatch for %q", field)
			}
		}
	}
}

func TestPM2CancelledPage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := buildPM2AuditPageContext(ctx, []pm2AuditLogLine{{Text: "ordinary"}}, nil, false, PM2AuditQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("page cancellation: %v", err)
	}
}
