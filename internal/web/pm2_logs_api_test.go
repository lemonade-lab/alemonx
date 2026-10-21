package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"alemonx/internal/robot"
)

const pm2LogAPI = "/api/v1/robot/pm2-logs"

type pm2HTTPFixture struct {
	activeStreams           atomic.Int32
	root, out, errLog, base string
	client                  *http.Client
	records                 []robot.PM2AuditLine
}

// Exercise HTTP serialization, query parsing, handlers and real log files.
// Only process discovery is a fixture: never connect to the user's PM2 daemon.
func newPM2HTTPFixture(t *testing.T) *pm2HTTPFixture {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Fatal("PM2 HTTP integration tests require Node.js:", err)
	}
	root := t.TempDir()
	f := &pm2HTTPFixture{root: root, out: filepath.Join(root, "out.log"), errLog: filepath.Join(root, "err.log"), client: &http.Client{Timeout: 5 * time.Second}}
	write := func(path, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "package.json"), `{"name":"pm2-http-fixture"}`)
	// The project-local PM2 entry is invoked by the production discovery path.
	payload, _ := json.Marshal([]any{map[string]any{"pm2_env": map[string]string{"pm_cwd": root, "pm_out_log_path": f.out, "pm_err_log_path": f.errLog}}})
	write(filepath.Join(root, "node_modules/pm2/bin/pm2"), "process.stdout.write("+strconv.Quote(string(payload))+");")
	out := []string{
		"untimed intro",
		"[2026-10-20 23:59:59][INFO] alpha before",
		"[2026-10-21 00:00:00][INFO] alpha 边界",
		"[2026-10-21 12:00:00][INFO] Beta ready",
		"[2026-10-21 12:00:00][INFO] same second",
		"[2026-10-21 23:59:59][INFO] final today",
		"[2026-10-22 00:00:00][INFO] gamma next",
	}
	errLines := []string{
		"[2026-10-20 23:59:59][ERROR] alpha old",
		"[2026-10-21 12:00:00][ERROR] Beta failure",
		"[2026-10-22 00:00:00][ERROR] gamma next",
		"untimed stack",
	}
	write(f.out, strings.Join(out, "\r\n")+"\r\n")
	write(f.errLog, strings.Join(errLines, "\n")+"\n")
	for _, text := range out {
		f.records = append(f.records, robot.PM2AuditLine{Source: "out", Text: text})
	}
	for _, text := range errLines {
		f.records = append(f.records, robot.PM2AuditLine{Source: "err", Text: text})
	}
	s := &server{}
	mux := http.NewServeMux()
	mux.HandleFunc(pm2LogAPI, s.robotPM2LogsHandler)
	mux.HandleFunc(pm2LogAPI+"/days", s.robotPM2LogDaysHandler)
	mux.HandleFunc(pm2LogAPI+"/export", s.robotPM2LogExportHandler)
	mux.HandleFunc(pm2LogAPI+"/stream", func(w http.ResponseWriter, r *http.Request) {
		f.activeStreams.Add(1)
		defer f.activeStreams.Add(-1)
		s.robotPM2LogStreamHandler(w, r)
	})
	f.base = newIPv4TestServer(t, mux).URL
	return f
}

func (f *pm2HTTPFixture) request(t *testing.T, method, suffix string, params url.Values) (*http.Response, []byte) {
	t.Helper()
	if params == nil {
		params = url.Values{}
	}
	if suffix == "/stream" && !params.Has("snapshot") {
		params.Set("snapshot", "1")
	}
	if !params.Has("root") {
		params.Set("root", f.root)
	}
	request, err := http.NewRequest(method, f.base+pm2LogAPI+suffix+"?"+params.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := f.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body []byte
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		if params.Get("snapshot") == "1" {
			line, readErr := bufio.NewReader(response.Body).ReadString('\n')
			body, err = []byte(line), readErr
		} else {
			body, err = io.ReadAll(response.Body)
		}
	} else {
		body, err = io.ReadAll(response.Body)
	}
	if err != nil {
		t.Fatal(err)
	}
	return response, body
}

func (f *pm2HTTPFixture) page(t *testing.T, params url.Values) robot.PM2AuditPage {
	t.Helper()
	response, body := f.request(t, http.MethodGet, "", params)
	if response.StatusCode != 200 {
		t.Fatalf("GET %v: %d %s", params, response.StatusCode, body)
	}
	var page robot.PM2AuditPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func lineKeys(lines []robot.PM2AuditLine) []string {
	keys := make([]string, 0, len(lines))
	for _, line := range lines {
		keys = append(keys, line.Source+"|"+line.Text)
	}
	sort.Strings(keys)
	return keys
}

func TestPM2LogsHTTPFilterPaginationMatrix(t *testing.T) {
	f := newPM2HTTPFixture(t)
	combinations, requests := 0, 0
	for _, date := range []string{"", "2026-10-20", "2026-10-21", "2026-10-22", "2026-10-23"} {
		for _, source := range []string{"", "all", "out", "err"} {
			for _, query := range []string{"", "ALPHA", "Beta", "边界", "not-present"} {
				for _, size := range []int{1, 2, 1000} {
					combinations++
					var expected []robot.PM2AuditLine
					for _, line := range f.records {
						if source != "" && source != "all" && source != line.Source {
							continue
						}
						if date != "" && !strings.HasPrefix(line.Text, "["+date+" ") {
							continue
						}
						if query != "" && !strings.Contains(strings.ToLower(line.Text), strings.ToLower(query)) {
							continue
						}
						expected = append(expected, line)
					}
					var got []robot.PM2AuditLine
					for number := 1; ; number++ {
						params := url.Values{"date": {date}, "source": {source}, "query": {query}, "perPage": {strconv.Itoa(size)}, "page": {strconv.Itoa(number)}}
						page := f.page(t, params)
						requests++
						if page.Page != number || page.PerPage != size || page.Total != len(expected) || len(page.Lines) > size {
							t.Fatalf("metadata %v: %#v", params, page)
						}
						if page.HasNewer != (number > 1) {
							t.Fatalf("hasNewer %v = %v", params, page.HasNewer)
						}
						got = append(got, page.Lines...)
						if !page.HasOlder {
							break
						}
						if number > len(f.records) {
							t.Fatal("pagination did not terminate")
						}
					}
					if !reflect.DeepEqual(lineKeys(got), lineKeys(expected)) {
						t.Fatalf("date=%s source=%s query=%s size=%d\ngot=%q\nwant=%q", date, source, query, size, lineKeys(got), lineKeys(expected))
					}
					if size == 1 {
						response, body := f.request(t, "GET", "/export", url.Values{"date": {date}, "source": {source}, "query": {query}, "perPage": {"1"}})
						requests++
						if response.StatusCode != 200 {
							t.Fatalf("export=%d %s", response.StatusCode, body)
						}
						var exported []robot.PM2AuditLine
						if len(expected) > 0 {
							for _, row := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
								marker, text, ok := strings.Cut(row, "] ")
								if !ok {
									t.Fatalf("missing source marker: %s", row)
								}
								exported = append(exported, robot.PM2AuditLine{Source: strings.ToLower(strings.TrimPrefix(marker, "[")), Text: text})
							}
						}
						if !reflect.DeepEqual(lineKeys(exported), lineKeys(expected)) {
							t.Fatalf("export differs from all matching pages: %s", body)
						}
					}

				}
			}
		}
	}
	t.Logf("%d filter/page-size combinations, %d HTTP requests (including exports)", combinations, requests)
}

func TestPM2LogsHTTPDays(t *testing.T) {
	f := newPM2HTTPFixture(t)
	response, body := f.request(t, "GET", "/days", nil)
	var result struct {
		Days []robot.PM2LogDay `json:"days"`
	}
	if response.StatusCode != 200 || json.Unmarshal(body, &result) != nil {
		t.Fatalf("days: %d %s", response.StatusCode, body)
	}
	expected := []robot.PM2LogDay{
		{Date: "2026-10-22", Count: 2, Out: 1, Err: 1, First: "2026-10-22 00:00:00", Last: "2026-10-22 00:00:00"},
		{Date: "2026-10-21", Count: 5, Out: 4, Err: 1, First: "2026-10-21 00:00:00", Last: "2026-10-21 23:59:59"},
		{Date: "2026-10-20", Count: 2, Out: 1, Err: 1, First: "2026-10-20 23:59:59", Last: "2026-10-20 23:59:59"},
	}
	if !reflect.DeepEqual(result.Days, expected) {
		t.Fatalf("days=%#v; want %#v", result.Days, expected)
	}
}

func TestPM2LogsHTTPInvalidParameters(t *testing.T) {
	f := newPM2HTTPFixture(t)
	cases := []url.Values{
		{"page": {"0"}}, {"page": {"-1"}}, {"page": {"abc"}}, {"page": {"1.5"}}, {"page": {"9999999999999999999999"}},
		{"perPage": {"0"}}, {"perPage": {"-1"}}, {"perPage": {"abc"}},
		{"date": {"2026-02-30"}}, {"date": {"2026-13-01"}}, {"date": {"yesterday"}},
		{"since": {"bad"}}, {"until": {"bad"}}, {"since": {"2026-10-22 00:00:00"}, "until": {"2026-10-21 00:00:00"}},
		{"source": {"wat"}}, {"snapshot": {"invalid"}},
		{"page": {"1", "2"}}, {"root": {"/one", "/two"}}, {"source": {"out", "err"}}, {"date": {"2026-10-20", "2026-10-21"}},
	}
	for _, suffix := range []string{"", "/export", "/stream"} {
		for _, params := range cases {
			if params.Has("snapshot") && suffix != "/stream" {
				continue
			}
			t.Run(suffix+"/"+params.Encode(), func(t *testing.T) {
				response, body := f.request(t, "GET", suffix, params)
				if response.StatusCode != 400 {
					t.Fatalf("status=%d, want 400; body=%s", response.StatusCode, body)
				}
				var payload map[string]any
				if json.Unmarshal(body, &payload) != nil || payload["error"] == nil {
					t.Fatalf("invalid error payload: %s", body)
				}
			})
		}
	}
}

func TestPM2LogsHTTPTimeRangeAndPageBounds(t *testing.T) {
	f := newPM2HTTPFixture(t)
	for _, params := range []url.Values{
		{"since": {"2026-10-21 12:00:00"}, "until": {"2026-10-21 12:00:00"}},
		{"date": {"2026-10-21"}, "since": {time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local).UTC().Format(time.RFC3339)}, "until": {"2026-10-21 12:00:00"}},
	} {
		page := f.page(t, params)
		if page.Total != 3 {
			t.Fatalf("inclusive range %v: %#v", params, page)
		}
	}
	page := f.page(t, url.Values{"perPage": {"999999"}})
	if page.PerPage != 1000 {
		t.Fatalf("page-size cap=%d", page.PerPage)
	}
	page = f.page(t, url.Values{"page": {strconv.Itoa(int(^uint(0) >> 1))}, "perPage": {"1000"}})
	if len(page.Lines) != 0 || page.HasOlder || !page.HasNewer {
		t.Fatalf("out-of-range page=%#v", page)
	}
}

func TestPM2LogsHTTPMethodsAndRoots(t *testing.T) {
	f := newPM2HTTPFixture(t)
	file := filepath.Join(f.root, "package.json")
	for _, suffix := range []string{"", "/days", "/export", "/stream"} {
		for _, method := range []string{"POST", "PUT", "PATCH", "HEAD"} {
			response, _ := f.request(t, method, suffix, nil)
			if response.StatusCode != 405 {
				t.Fatalf("%s %s: %d", method, suffix, response.StatusCode)
			}
		}
		for _, root := range []string{"", "relative/path", filepath.Join(f.root, "missing"), file, t.TempDir()} {
			t.Run(suffix+"/root="+root, func(t *testing.T) {
				response, body := f.request(t, "GET", suffix, url.Values{"root": {root}, "snapshot": {"1"}})
				if response.StatusCode != 400 {
					t.Fatalf("invalid root: %d %s", response.StatusCode, body)
				}
			})
		}
	}
}

type pm2HTTPStream struct {
	response *http.Response
	reader   *bufio.Reader
}

func (f *pm2HTTPFixture) stream(t *testing.T, params url.Values) *pm2HTTPStream {
	t.Helper()
	if params == nil {
		params = url.Values{}
	}
	params.Set("root", f.root)
	if !params.Has("snapshot") {
		params.Set("snapshot", "1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	request, _ := http.NewRequestWithContext(ctx, "GET", f.base+pm2LogAPI+"/stream?"+params.Encode(), nil)
	response, err := f.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	if response.StatusCode != 200 || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream response=%v", response)
	}
	return &pm2HTTPStream{response: response, reader: bufio.NewReader(response.Body)}
}
func (s *pm2HTTPStream) next(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			t.Fatal("reading SSE frame:", err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var result map[string]json.RawMessage
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &result); err != nil {
			t.Fatal(err)
		}
		if result["error"] != nil {
			t.Fatalf("SSE error: %s", line)
		}
		return result
	}
}

func appendPM2HTTPLog(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(text)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestPM2LogsHTTPStreamFiltersSnapshotAndTail(t *testing.T) {
	f := newPM2HTTPFixture(t)
	stream := f.stream(t, url.Values{"source": {"err"}, "query": {"Beta"}, "date": {"2026-10-21"}})
	frame := stream.next(t)
	var snapshot robot.PM2AuditPage
	if err := json.Unmarshal(frame["snapshot"], &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != 1 || len(snapshot.Lines) != 1 || snapshot.Lines[0].Source != "err" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	appendPM2HTTPLog(t, f.out, "[2026-10-21 12:01:00] Beta excluded source\n")
	appendPM2HTTPLog(t, f.errLog, "[2026-10-21 12:01:00] excluded query\n[2026-10-22 12:01:00] Beta excluded date\n[2026-10-21 12:01:00] Beta accepted\n")
	frame = stream.next(t)
	var source, text string
	json.Unmarshal(frame["source"], &source)
	json.Unmarshal(frame["text"], &text)
	if source != "err" || text != "[2026-10-21 12:01:00] Beta accepted" {
		t.Fatalf("unfiltered tail source=%s text=%s", source, text)
	}
}

func TestPM2LogsHTTPExportAndRedaction(t *testing.T) {
	f := newPM2HTTPFixture(t)
	appendPM2HTTPLog(t, f.out, "[2026-10-21 12:02:00] {\"token\":\"private-test-token\",\"email\":\"private-test@example.com\"}\n")
	for _, suffix := range []string{"", "/export"} {
		response, body := f.request(t, "GET", suffix, url.Values{"date": {"2026-10-21"}, "source": {"out"}})
		if response.StatusCode != 200 || strings.Contains(string(body), "private-test") || !strings.Contains(string(body), "REDACTED") {
			t.Fatalf("%s: %d %s", suffix, response.StatusCode, body)
		}
		if suffix == "/export" && (!strings.Contains(response.Header.Get("Content-Disposition"), "pm2-2026-10-21.log") || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/plain") || strings.Contains(string(body), "[ERR]")) {
			t.Fatalf("export headers=%v body=%s", response.Header, body)
		}
	}
	stream := f.stream(t, nil)
	snapshot := stream.next(t)
	raw, _ := json.Marshal(snapshot)
	if strings.Contains(string(raw), "private-test") {
		t.Fatalf("SSE snapshot leaked test secret: %s", raw)
	}
	appendPM2HTTPLog(t, f.errLog, "[2026-10-21 12:03:00] {\"token\":\"private-test-tail\"}\n")
	raw, _ = json.Marshal(stream.next(t))
	if strings.Contains(string(raw), "private-test") || !strings.Contains(string(raw), "REDACTED") {
		t.Fatalf("SSE tail not redacted: %s", raw)
	}
}

func TestPM2LogsHTTPDeleteScopesAndIdempotency(t *testing.T) {
	for _, source := range []string{"", "all", "out", "err"} {
		t.Run("source="+source, func(t *testing.T) {
			f := newPM2HTTPFixture(t)
			before, err := os.Stat(f.out)
			if err != nil {
				t.Fatal(err)
			}
			params := url.Values{"date": {"2026-10-21"}, "source": {source}}
			for pass := 0; pass < 2; pass++ {
				response, body := f.request(t, "DELETE", "", params)
				var result robot.PM2LogDeleteResult
				if response.StatusCode != 200 || json.Unmarshal(body, &result) != nil {
					t.Fatalf("delete: %d %s", response.StatusCode, body)
				}
				want := 5
				if source == "out" {
					want = 4
				}
				if source == "err" {
					want = 1
				}
				if pass == 1 {
					want = 0
				}
				if result.Deleted != want {
					t.Fatalf("deleted=%d want=%d", result.Deleted, want)
				}
			}
			after, _ := os.Stat(f.out)
			if !os.SameFile(before, after) {
				t.Fatal("deletion replaced PM2's open log inode")
			}
			page := f.page(t, nil)
			for _, line := range f.records {
				remove := strings.HasPrefix(line.Text, "[2026-10-21 ") && (source == "" || source == "all" || source == line.Source)
				found := false
				for _, item := range page.Lines {
					if item == line {
						found = true
					}
				}
				if found == remove {
					t.Fatalf("scope mismatch line=%#v removed=%v found=%v", line, remove, found)
				}
			}
		})
	}
}

func TestPM2LogsHTTPDeleteRejectsAmbiguousFilters(t *testing.T) {
	f := newPM2HTTPFixture(t)
	for _, params := range []url.Values{
		{}, {"date": {"bad"}}, {"date": {"2026-10-21"}, "source": {"bad"}},
		{"date": {"2026-10-21"}, "query": {"Beta"}},
		{"date": {"2026-10-21"}, "since": {"2026-10-21 12:00:00"}},
		{"date": {"2026-10-21"}, "until": {"2026-10-21 12:00:00"}},
		{"date": {"2026-10-21"}, "page": {"2"}},
		{"date": {"2026-10-21"}, "perPage": {"1"}},
	} {
		response, body := f.request(t, "DELETE", "", params)
		if response.StatusCode != 400 {
			t.Fatalf("ambiguous delete %v: %d %s", params, response.StatusCode, body)
		}
	}
	if page := f.page(t, nil); page.Total != len(f.records) {
		t.Fatal("invalid delete mutated logs")
	}
}

func TestPM2LogsHTTPConcurrentReadsAndDeletes(t *testing.T) {
	f := newPM2HTTPFixture(t)
	// Warm discovery before concurrent clients; real filesystem reads remain concurrent.
	f.page(t, nil)
	var wg sync.WaitGroup
	failures := make(chan string, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			suffix := ""
			if i%2 == 0 {
				suffix = "/export"
			}
			response, err := f.client.Get(f.base + pm2LogAPI + suffix + "?" + url.Values{"root": {f.root}}.Encode())
			if err != nil {
				failures <- err.Error()
				return
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 200 {
				failures <- fmt.Sprintf("read=%d %s", response.StatusCode, body)
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	// Distinct date deletions must serialize their read/modify/write transactions.
	for _, date := range []string{"2026-10-20", "2026-10-21", "2026-10-22"} {
		wg.Add(1)
		go func(date string) {
			defer wg.Done()
			response, body := f.request(t, "DELETE", "", url.Values{"date": {date}})
			if response.StatusCode != 200 {
				t.Errorf("delete=%d %s", response.StatusCode, body)
			}
		}(date)
	}
	wg.Wait()
	page := f.page(t, nil)
	if page.Total != 2 || !strings.Contains(page.Output, "untimed intro") || !strings.Contains(page.Output, "untimed stack") {
		t.Fatalf("concurrent deletes lost preservation: %#v", page)
	}
}

func TestPM2LogsHTTPEmptyAndMissingFiles(t *testing.T) {
	for _, mode := range []string{"empty", "no-timestamps", "missing-out", "missing-both"} {
		t.Run(mode, func(t *testing.T) {
			f := newPM2HTTPFixture(t)
			write := func(path, text string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := 0
			switch mode {
			case "empty":
				write(f.out, "")
				write(f.errLog, "")
			case "no-timestamps":
				write(f.out, "plain\n")
				write(f.errLog, "stack\n")
				want = 2
			case "missing-out":
				if err := os.Remove(f.out); err != nil {
					t.Fatal(err)
				}
				want = 4
			case "missing-both":
				os.Remove(f.out)
				os.Remove(f.errLog)
			}
			page := f.page(t, nil)
			if page.Total != want {
				t.Fatalf("total=%d want=%d", page.Total, want)
			}
			response, body := f.request(t, "GET", "/days", nil)
			if response.StatusCode != 200 {
				t.Fatalf("days=%d %s", response.StatusCode, body)
			}
			if mode != "missing-out" && strings.TrimSpace(string(body)) != "{\"days\":[]}" {
				t.Fatalf("empty dates must be a successful empty list: %s", body)
			}
			response, body = f.request(t, "GET", "/export", nil)
			if response.StatusCode != 200 {
				t.Fatalf("export=%d %s", response.StatusCode, body)
			}
			stream := f.stream(t, nil)
			var snapshot robot.PM2AuditPage
			if err := json.Unmarshal(stream.next(t)["snapshot"], &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Total != want {
				t.Fatalf("snapshot.Total=%d want=%d", snapshot.Total, want)
			}
		})
	}
}

func TestPM2LogsHTTPNormalizationAndLiteralSearch(t *testing.T) {
	f := newPM2HTTPFixture(t)
	for _, params := range []url.Values{
		{"source": {" OUT "}, "query": {" beta "}, "date": {" 2026-10-21 "}},
		{"source": {" ALL "}, "query": {" 边界 "}},
	} {
		page := f.page(t, params)
		if page.Total != 1 {
			t.Fatalf("normalized %v: %#v", params, page)
		}
	}
	text := "[2026-10-21 12:04:00] [] literal.* <script>alert(1)</script>"
	appendPM2HTTPLog(t, f.out, text+"\n")
	for _, query := range []string{"[]", "literal.*", "<script>"} {
		page := f.page(t, url.Values{"query": {query}})
		if page.Total != 1 || page.Lines[0].Text != text {
			t.Fatalf("literal query %q: %#v", query, page)
		}
	}
}

func TestPM2LogsHTTPStreamFileLifecycle(t *testing.T) {
	for _, mode := range []string{"append-partial", "truncate", "replace-larger", "created-after-missing"} {
		t.Run(mode, func(t *testing.T) {
			f := newPM2HTTPFixture(t)
			if mode == "created-after-missing" {
				if err := os.Remove(f.out); err != nil {
					t.Fatal(err)
				}
			}
			stream := f.stream(t, url.Values{"source": {"out"}})
			if stream.next(t)["snapshot"] == nil {
				t.Fatal("missing initial snapshot")
			}
			expected := "[2026-10-21 12:05:00] complete"
			switch mode {
			case "append-partial":
				appendPM2HTTPLog(t, f.out, "[2026-10-21 12:05:00] com")
				time.Sleep(500 * time.Millisecond) // Cross a tail poll without a newline.
				appendPM2HTTPLog(t, f.out, "plete\r\n")
			case "truncate", "created-after-missing":
				if err := os.WriteFile(f.out, []byte(expected+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replace-larger":
				// More bytes than the original file: size-only rotation detection fails.
				expected += " " + strings.Repeat("replacement", 100)
				replacement := filepath.Join(f.root, "rotated.log")
				if err := os.WriteFile(replacement, []byte(expected+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, f.out); err != nil {
					t.Fatal(err)
				}
			}
			var text string
			json.Unmarshal(stream.next(t)["text"], &text)
			if text != expected {
				t.Fatalf("tail=%q want=%q", text, expected)
			}
			stream.response.Body.Close()
			// Reconnection replaces history; the next frame must only contain the new append.
			reconnected := f.stream(t, url.Values{"source": {"out"}})
			var page robot.PM2AuditPage
			json.Unmarshal(reconnected.next(t)["snapshot"], &page)
			occurrences := 0
			for _, line := range page.Lines {
				if line.Text == expected {
					occurrences++
				}
			}
			if occurrences != 1 {
				t.Fatalf("reconnected history contains %d copies", occurrences)
			}
			appendPM2HTTPLog(t, f.out, "[2026-10-21 12:06:00] after reconnect\n")
			json.Unmarshal(reconnected.next(t)["text"], &text)
			if text != "[2026-10-21 12:06:00] after reconnect" {
				t.Fatalf("replayed old tail on reconnect: %q", text)
			}
		})
	}
}

func TestPM2LogsHTTPStreamDisconnectAndLegacyHeaders(t *testing.T) {
	f := newPM2HTTPFixture(t)
	for _, params := range []url.Values{{}, {"snapshot": {"0"}}} {
		started := time.Now()
		stream := f.stream(t, params)
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("stream headers blocked for %s", elapsed)
		}
		if params.Get("snapshot") != "0" {
			stream.next(t)
		}
		stream.response.Body.Close()
		deadline := time.After(2 * time.Second)
		ticker := time.NewTicker(10 * time.Millisecond)
		for f.activeStreams.Load() != 0 {
			select {
			case <-ticker.C:
			case <-deadline:
				t.Fatal("stream handler survived client disconnect")
			}
		}
		ticker.Stop()
	}
}

func TestPM2LogsHTTPDiscoveryFailures(t *testing.T) {
	for _, script := range []string{"process.stdout.write('not JSON')", "process.exit(7)"} {
		t.Run(script, func(t *testing.T) {
			f := newPM2HTTPFixture(t)
			if err := os.WriteFile(filepath.Join(f.root, "node_modules/pm2/bin/pm2"), []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			for _, suffix := range []string{"", "/days", "/export"} {
				response, body := f.request(t, "GET", suffix, nil)
				if response.StatusCode != 400 || !strings.Contains(string(body), "error") {
					t.Fatalf("discovery failure %s: %d %s", suffix, response.StatusCode, body)
				}
			}
			response, body := f.request(t, "GET", "/stream", nil)
			if response.StatusCode != 200 || !strings.Contains(string(body), `"error"`) {
				t.Fatalf("runtime SSE failure: %d %s", response.StatusCode, body)
			}
		})
	}
}

func TestPM2LogsHTTPAdditionalTimeBounds(t *testing.T) {
	f := newPM2HTTPFixture(t)
	for _, test := range []struct {
		params url.Values
		total  int
	}{
		{url.Values{"since": {"2026-10-21 00:00:00"}}, 7},
		{url.Values{"until": {"2026-10-20 23:59:59"}}, 2},
		{url.Values{"since": {"2026-10-22 00:00"}, "until": {"2026-10-22 00:00:00"}}, 2},
		{url.Values{"date": {"2026-10-21"}, "since": {"2026-10-22 00:00:00"}}, 0},
		{url.Values{"page": {""}, "perPage": {""}, "date": {""}, "source": {""}, "query": {""}}, 11},
	} {
		page := f.page(t, test.params)
		if page.Total != test.total {
			t.Fatalf("%v total=%d want=%d", test.params, page.Total, test.total)
		}
	}
	response, _ := f.request(t, "GET", "/days", url.Values{"root": {f.root, f.root}})
	if response.StatusCode != 400 {
		t.Fatalf("duplicate date-list root=%d", response.StatusCode)
	}
}

func TestPM2LogsHTTPRejectsMalformedQueryEncoding(t *testing.T) {
	f := newPM2HTTPFixture(t)
	for _, suffix := range []string{"", "/days", "/export", "/stream"} {
		for _, extra := range []string{"&query=%zz", "&query=Beta;source=err", "&date=%E0%A4%A"} {
			methods := []string{"GET"}
			if suffix == "" {
				methods = append(methods, "DELETE")
			}
			for _, method := range methods {
				params := url.Values{"root": {f.root}, "date": {"2026-10-21"}, "snapshot": {"1"}}
				request, err := http.NewRequest(method, f.base+pm2LogAPI+suffix+"?"+params.Encode()+extra, nil)
				if err != nil {
					t.Fatal(err)
				}
				response, err := f.client.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode != 400 {
					t.Fatalf("%s %s%s = %d %s", method, suffix, extra, response.StatusCode, body)
				}
			}
		}
	}
	if page := f.page(t, nil); page.Total != len(f.records) {
		t.Fatal("malformed query allowed a destructive request")
	}
}
