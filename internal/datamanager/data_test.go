package datamanager

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func database(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "测试 # data.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE demo (id INTEGER PRIMARY KEY, value TEXT); INSERT INTO demo VALUES (1, 'before')"); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSQLiteForeignKeysAndEngineLimits(t *testing.T) {
	path := database(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE parent(id INTEGER PRIMARY KEY); CREATE TABLE child(id INTEGER REFERENCES parent(id) ON DELETE CASCADE); INSERT INTO parent VALUES(1); INSERT INTO child VALUES(1)"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := QuerySQLite(ctx, SQLRequest{Path: path, SQL: "INSERT INTO child VALUES(999)", Write: true, Confirmed: true}); err == nil {
		t.Fatal("foreign key check bypassed")
	}
	if _, err := QuerySQLite(ctx, SQLRequest{Path: path, SQL: "DELETE FROM parent WHERE id=1", Write: true, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM child").Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade failed: %d %v", count, err)
	}
	if _, err := QuerySQLite(ctx, SQLRequest{Path: path, SQL: "SELECT zeroblob(2097152)"}); err == nil {
		t.Fatal("oversized value accepted")
	}
	if err := db.QueryRow("SELECT length(zeroblob(2097152))").Scan(&count); err != nil || count != 2097152 {
		t.Fatal("interactive limit affected other connections", err)
	}
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := QuerySQLite(short, SQLRequest{Path: path, SQL: "WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n) SELECT count(*) FROM n"}); err == nil {
		t.Fatal("long query ignored deadline")
	}
}

func TestSQLiteAdmissionControl(t *testing.T) {
	for i := 0; i < cap(sqliteQuerySlots); i++ {
		sqliteQuerySlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(sqliteQuerySlots); i++ {
			<-sqliteQuerySlots
		}
	}()
	if _, err := QuerySQLite(context.Background(), SQLRequest{}); err == nil || !strings.Contains(err.Error(), "繁忙") {
		t.Fatal("query not rejected at capacity", err)
	}
}

func TestRedisCompareAndSet(t *testing.T) {
	s := miniredis.RunT(t)
	if err := s.Set("key", "original"); err != nil {
		t.Fatal(err)
	}
	s.SetTTL("key", time.Minute)
	original := "original"
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := RedisCommand(context.Background(), RedisRequest{Address: s.Addr(), Args: []string{"SET", "key", fmt.Sprint(i), "KEEPTTL"}, Confirmed: true, Expected: &original})
			if err == nil {
				won.Add(1)
			} else if !errors.Is(err, ErrRedisConflict) {
				t.Errorf("unexpected failure: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("expected one winner, got %d", won.Load())
	}
	if s.TTL("key") != time.Minute {
		t.Fatal("TTL changed")
	}
	s.Del("key")
	if _, err := RedisCommand(context.Background(), RedisRequest{Address: s.Addr(), Args: []string{"SET", "key", "new", "KEEPTTL"}, Confirmed: true, Expected: &original}); !errors.Is(err, ErrRedisConflict) {
		t.Fatalf("deleted key recreated: %v", err)
	}
}

func TestSQLiteReadWriteBoundary(t *testing.T) {
	path := database(t)
	ctx := context.Background()
	for _, statement := range []string{
		"UPDATE demo SET value='oops'", "SELECT 1; DELETE FROM demo", "PRAGMA query_only=0", "ATTACH DATABASE ':memory:' AS other", "WITH x AS (SELECT 1) DELETE FROM demo", "/* comment */ PRAGMA query_only=0", "SELECT 1; -- comment\nUPDATE demo SET value='bad'",
	} {
		if _, err := QuerySQLite(ctx, SQLRequest{Path: path, SQL: statement}); err == nil {
			t.Fatalf("accepted read-only mutation %s", statement)
		}
	}
	if _, err := QuerySQLite(ctx, SQLRequest{Path: path, SQL: "DELETE FROM demo", Write: true}); err == nil {
		t.Fatal("accepted unconfirmed write")
	}
	result, err := QuerySQLite(ctx, SQLRequest{Path: path, SQL: "-- comment\n SELECT value, ';' AS quoted, 9223372036854775807 AS big FROM demo; /* trailing */"})
	if err != nil || len(result.Rows) != 1 || result.Rows[0][0] != "before" || result.Rows[0][2] != "9223372036854775807" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	result, err = QuerySQLite(ctx, SQLRequest{Path: path, SQL: "UPDATE demo SET value='after' WHERE id=1", Write: true, Confirmed: true})
	if err != nil || result.Affected != 1 {
		t.Fatalf("write=%+v err=%v", result, err)
	}
	result, err = QuerySQLite(ctx, SQLRequest{Path: path, SQL: "SELECT value FROM demo"})
	if err != nil || result.Rows[0][0] != "after" {
		t.Fatalf("persist=%+v err=%v", result, err)
	}
}

func TestSQLiteLimitsAndInvalidFiles(t *testing.T) {
	path := database(t)
	result, err := QuerySQLite(context.Background(), SQLRequest{Path: path, SQL: "WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<1000) SELECT x FROM n"})
	if err != nil || !result.Truncated || len(result.Rows) != 200 {
		t.Fatalf("bounded=%+v %v", result, err)
	}
	for _, query := range []string{"SELECT 1; SELECT 2", "SELECT 'unclosed", "/* unclosed"} {
		if _, err := sqlKeyword(query); err == nil {
			t.Fatal("accepted malformed SQL")
		}
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	if _, err := QuerySQLite(context.Background(), SQLRequest{Path: missing, SQL: "SELECT 1"}); err == nil {
		t.Fatal("accepted missing file")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("created missing database")
	}
	if _, err := SQLitePath("relative.db"); err == nil {
		t.Fatal("accepted relative file")
	}
	if _, err := SQLitePath(t.TempDir()); err == nil {
		t.Fatal("accepted directory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := QuerySQLite(ctx, SQLRequest{Path: path, SQL: "SELECT 1"}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestRedisManagement(t *testing.T) {
	s := miniredis.RunT(t)
	ctx := context.Background()
	request := RedisRequest{Address: s.Addr(), DB: 2, Args: []string{"SET", "a key", "line\nvalue"}}
	if _, err := RedisCommand(ctx, request); err == nil {
		t.Fatal("unconfirmed write")
	}
	request.Confirmed = true
	if _, err := RedisCommand(ctx, request); err != nil {
		t.Fatal(err)
	}
	request.Confirmed = false
	request.Args = []string{"GET", "a key"}
	value, err := RedisCommand(ctx, request)
	if err != nil || value != "line\nvalue" {
		t.Fatalf("get=%v %v", value, err)
	}
	request.DB = 0
	value, err = RedisCommand(ctx, request)
	if err != nil || value != nil {
		t.Fatalf("DB isolation=%v %v", value, err)
	}
	for _, command := range []string{"FLUSHDB", "FLUSHALL", "EVAL", "CONFIG", "SHUTDOWN"} {
		request.Args = []string{command}
		request.Confirmed = true
		if _, err := RedisCommand(ctx, request); err == nil {
			t.Fatalf("accepted %s", command)
		}
	}
	s.RequireAuth("private")
	request.Args = []string{"PING"}
	if _, err := RedisCommand(ctx, request); err == nil {
		t.Fatal("accepted missing auth")
	}
	request.Password = "private"
	if _, err := RedisCommand(ctx, request); err != nil {
		t.Fatal(err)
	}
}

func TestRedisResponseBounds(t *testing.T) {
	for _, response := range []string{"$2000000\r\n", "*999999\r\n", "$1\r\n\xff\r\n", "$3\r\nab"} {
		if _, err := readRedis(bufio.NewReader(strings.NewReader(response)), 0); err == nil {
			t.Fatalf("accepted invalid/oversize response %q", response)
		}
	}
}
