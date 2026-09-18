// Package datamanager provides bounded, explicitly authorized database tools.
package datamanager

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

// Scope admission control to interactive data tools, never the application's
// own database connections. Do not use process-wide SQLite heap limits.
var sqliteQuerySlots = make(chan struct{}, 4)
var ErrSQLiteBusy = errors.New("数据库查询繁忙，请稍后重试（最多同时执行 4 个请求）")

type SQLRequest struct {
	Path      string `json:"path"`
	SQL       string `json:"sql"`
	Write     bool   `json:"write"`
	Confirmed bool   `json:"confirmed"`
}

type SQLResult struct {
	Columns        []string `json:"columns"`
	Rows           [][]any  `json:"rows"`
	Affected       int64    `json:"affected"`
	Truncated      bool     `json:"truncated"`
	SystemDatabase bool     `json:"systemDatabase"`
}

// Only open existing SQLite files. A mistyped path must never create a database.
func SQLitePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("请选择服务器上的 SQLite 绝对路径")
	}
	path, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", errors.New("数据库文件不存在或无法访问")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("无法打开数据库文件")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("请选择普通 SQLite 文件")
	}
	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil || string(header) != "SQLite format 3\x00" {
		return "", errors.New("文件不是有效的 SQLite 数据库")
	}
	return path, nil
}

func QuerySQLite(ctx context.Context, input SQLRequest) (SQLResult, error) {
	result := SQLResult{Columns: []string{}, Rows: [][]any{}}
	select {
	case sqliteQuerySlots <- struct{}{}:
		defer func() { <-sqliteQuerySlots }()
	default:
		return result, ErrSQLiteBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if len(input.SQL) == 0 || len(input.SQL) > 64<<10 {
		return result, errors.New("SQL 不能为空，且不能超过 64 KiB")
	}
	if input.Write && !input.Confirmed {
		return result, errors.New("执行写入前请确认操作")
	}
	keyword, err := sqlKeyword(input.SQL)
	if err != nil {
		return result, err
	}
	allowed := " SELECT WITH EXPLAIN "
	if input.Write {
		allowed = " INSERT UPDATE DELETE REPLACE CREATE ALTER DROP WITH "
	}
	if !strings.Contains(allowed, " "+keyword+" ") {
		return result, errors.New("只读模式仅允许查询；写入模式允许单条数据或表结构语句，不支持 ATTACH、PRAGMA 与事务控制")
	}
	path, err := SQLitePath(input.Path)
	if err != nil {
		return result, err
	}
	mode := "ro"
	if input.Write {
		mode = "rw"
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	// URL.Path needs a slash before Windows drive letters.
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	q := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(1500)", "foreign_keys(1)", "cache_size(-2048)", "temp_store(1)"}}
	if !input.Write {
		q.Add("_pragma", "query_only(1)")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return result, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		return result, err
	}
	defer conn.Close()
	for id, value := range map[int]int{
		sqlitelib.SQLITE_LIMIT_LENGTH:          1 << 20,
		sqlitelib.SQLITE_LIMIT_SQL_LENGTH:      64 << 10,
		sqlitelib.SQLITE_LIMIT_COLUMN:          256,
		sqlitelib.SQLITE_LIMIT_EXPR_DEPTH:      100,
		sqlitelib.SQLITE_LIMIT_COMPOUND_SELECT: 50,
		sqlitelib.SQLITE_LIMIT_VARIABLE_NUMBER: 512,
		sqlitelib.SQLITE_LIMIT_ATTACHED:        0,
		sqlitelib.SQLITE_LIMIT_VDBE_OP:         50000,
		sqlitelib.SQLITE_LIMIT_WORKER_THREADS:  0,
	} {
		if _, err := sqlite.Limit(conn, id, value); err != nil {
			return result, err
		}
	}
	if input.Write {
		out, err := conn.ExecContext(ctx, input.SQL)
		if err != nil {
			return result, err
		}
		result.Affected, err = out.RowsAffected()
		return result, err
	}
	rows, err := conn.QueryContext(ctx, input.SQL)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Columns, err = rows.Columns()
	if err != nil {
		return result, err
	}
	size := 0
	for rows.Next() {
		if len(result.Rows) >= 200 {
			result.Truncated = true
			break
		}
		values := make([]any, len(result.Columns))
		pointers := make([]any, len(values))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return result, err
		}
		for i, value := range values {
			switch v := value.(type) {
			case []byte:
				values[i] = map[string]string{"base64": base64.StdEncoding.EncodeToString(v)}
			case int64:
				if v > 9007199254740991 || v < -9007199254740991 {
					values[i] = strconv.FormatInt(v, 10)
				}
			}
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return result, err
		}
		size += len(encoded) + 1
		if size > 1<<20 {
			result.Truncated = true
			break
		}
		result.Rows = append(result.Rows, values)
	}
	return result, rows.Err()
}

// Enforce one statement without confusing quoted semicolons or SQL comments.
// This prevents a read request disabling query_only and attaching writable files.
func sqlKeyword(query string) (string, error) {
	keyword := ""
	ended := false
	for i := 0; i < len(query); {
		c := query[i]
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		if strings.HasPrefix(query[i:], "--") {
			for i < len(query) && query[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(query[i:], "/*") {
			n := strings.Index(query[i+2:], "*/")
			if n < 0 {
				return "", errors.New("SQL 注释未闭合")
			}
			i += n + 4
			continue
		}
		if ended {
			return "", errors.New("每次只能执行一条 SQL")
		}
		if c == ';' {
			ended = true
			i++
			continue
		}
		if keyword == "" {
			start := i
			for i < len(query) && ((query[i] >= 'A' && query[i] <= 'Z') || (query[i] >= 'a' && query[i] <= 'z')) {
				i++
			}
			if start == i {
				return "", errors.New("SQL 开头无效")
			}
			keyword = strings.ToUpper(query[start:i])
			continue
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			end := c
			if c == '[' {
				end = ']'
			}
			i++
			closed := false
			for i < len(query) {
				if query[i] == end {
					i++
					if end != ']' && i < len(query) && query[i] == end {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return "", errors.New("SQL 引号未闭合")
			}
			continue
		}
		i++
	}
	if keyword == "" {
		return "", errors.New("SQL 不能为空")
	}
	return keyword, nil
}
