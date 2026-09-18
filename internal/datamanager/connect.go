package datamanager

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

// Connections are supplied per request. Credentials are never stored in a registry.
type Connection struct {
	Engine      string `json:"engine"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Database    string `json:"database"`
	Path        string `json:"path"`
	TLS         string `json:"tls"`
	querySchema string
}
type ConnectRequest struct {
	Connection Connection `json:"connection"`
	Action     string     `json:"action"`
	Schema     string     `json:"schema"`
	Table      string     `json:"table"`
	Offset     int        `json:"offset"`
	SQL        string     `json:"sql"`
	Write      bool       `json:"write"`
	Confirmed  bool       `json:"confirmed"`
}

var connectSlots = make(chan struct{}, 4)

type silentSQLLogger struct{}

func (silentSQLLogger) Print(...any) {}

func quoteObject(engine, name string) string {
	quote := `"`
	if engine == "mysql" || engine == "mariadb" {
		quote = "`"
	}
	return quote + strings.ReplaceAll(name, quote, quote+quote) + quote
}
func sqlLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

// Metadata identifiers are quoted; values never become unescaped SQL fragments.
func connectStatement(r ConnectRequest) (string, error) {
	c := r.Connection
	if c.Engine != "sqlite" && c.Engine != "mysql" && c.Engine != "mariadb" && c.Engine != "postgres" {
		return "", errors.New("不支持的数据库类型")
	}
	if r.Offset < 0 || r.Offset > 1000000 {
		return "", errors.New("分页偏移超出范围")
	}
	if len(r.Schema) > 256 || len(r.Table) > 256 || strings.ContainsAny(r.Schema+r.Table, "\x00\\") {
		return "", errors.New("对象名称无效")
	}
	switch r.Action {
	case "test":
		return "SELECT 1 AS connected", nil
	case "databases":
		if c.Engine == "sqlite" {
			return "SELECT 'main' AS name", nil
		}
		if c.Engine == "postgres" {
			return "SELECT datname AS name FROM pg_database WHERE datallowconn AND has_database_privilege(datname, 'CONNECT') ORDER BY datname", nil
		}
		return "SELECT schema_name AS name FROM information_schema.schemata ORDER BY schema_name", nil
	case "schemas":
		if c.Engine == "sqlite" {
			return "SELECT 'main' AS name", nil
		}
		return "SELECT schema_name AS name FROM information_schema.schemata ORDER BY schema_name", nil
	case "tables":
		if c.Engine == "sqlite" {
			return "SELECT name, type FROM sqlite_schema WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name", nil
		}
		return "SELECT table_name AS name, table_type AS type FROM information_schema.tables WHERE table_schema=" + sqlLiteral(r.Schema) + " ORDER BY table_name", nil
	case "structure":
		if c.Engine == "sqlite" {
			return "SELECT * FROM pragma_table_xinfo(" + sqlLiteral(r.Table) + ")", nil
		}
		return "SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema=" + sqlLiteral(r.Schema) + " AND table_name=" + sqlLiteral(r.Table) + " ORDER BY ordinal_position", nil
	case "indexes":
		if c.Engine == "sqlite" {
			return "SELECT * FROM pragma_index_list(" + sqlLiteral(r.Table) + ")", nil
		}
		if c.Engine == "postgres" {
			return "SELECT indexname, indexdef FROM pg_indexes WHERE schemaname=" + sqlLiteral(r.Schema) + " AND tablename=" + sqlLiteral(r.Table) + " ORDER BY indexname", nil
		}
		return "SELECT index_name, column_name, non_unique, seq_in_index FROM information_schema.statistics WHERE table_schema=" + sqlLiteral(r.Schema) + " AND table_name=" + sqlLiteral(r.Table) + " ORDER BY index_name, seq_in_index", nil
	case "browse":
		if r.Table == "" {
			return "", errors.New("请选择表或视图")
		}
		target := quoteObject(c.Engine, r.Table)
		if c.Engine != "sqlite" {
			if r.Schema == "" {
				return "", errors.New("请选择 Schema 或数据库")
			}
			target = quoteObject(c.Engine, r.Schema) + "." + target
		}
		return fmt.Sprintf("SELECT * FROM %s LIMIT 100 OFFSET %d", target, r.Offset), nil
	case "query":
		if len(r.SQL) == 0 || len(r.SQL) > 64<<10 {
			return "", errors.New("SQL 不能为空，且不能超过 64 KiB")
		}
		// The initial editor deliberately excludes dialect constructs that the
		// single-statement scanner does not understand. Never silently split SQL.
		if strings.ContainsAny(r.SQL, "\\$") || strings.Contains(r.SQL, "/*") || strings.Contains(r.SQL, "#") {
			return "", errors.New("当前单语句编辑器暂不支持反斜杠、美元引号或块注释")
		}
		word, err := sqlKeyword(r.SQL)
		if err != nil {
			return "", err
		}
		allowed := " SELECT WITH EXPLAIN "
		if r.Write {
			allowed = " INSERT UPDATE DELETE REPLACE CREATE ALTER DROP WITH "
		}
		if !strings.Contains(allowed, " "+word+" ") {
			return "", errors.New("仅支持单条查询或确认后的数据/结构变更，不支持事务控制和管理命令")
		}
		// Read-only transactions do not prevent MySQL server-file exports.
		if !r.Write && strings.Contains(strings.ToUpper(r.SQL), "INTO") {
			return "", errors.New("只读查询不支持 INTO 子句")
		}
		return r.SQL, nil
	}
	return "", errors.New("未知数据库操作")
}

func openRemote(c Connection) (*sql.DB, error) {
	if c.Host == "" || c.Username == "" || c.Port < 1 || c.Port > 65535 || strings.ContainsAny(c.Host, "/\\\x00") {
		return nil, errors.New("请检查主机、端口和用户名")
	}
	if c.TLS != "verify-full" && c.TLS != "disable" {
		return nil, errors.New("请选择验证证书的 TLS 或明确关闭 TLS")
	}
	address := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	if c.Engine == "postgres" {
		u := url.URL{Scheme: "postgres", Host: address, User: url.UserPassword(c.Username, c.Password), Path: "/" + c.Database}
		if c.Database == "" {
			u.Path = "/postgres"
		}
		u.RawQuery = url.Values{"sslmode": {c.TLS}, "connect_timeout": {"5"}, "statement_timeout": {"7000"}, "application_name": {"alemonx-connect"}}.Encode()
		if c.querySchema != "" {
			q := u.Query()
			q.Set("search_path", quoteObject("postgres", c.querySchema))
			u.RawQuery = q.Encode()
		}
		connector, err := pq.NewConnector(u.String())
		if err != nil {
			return nil, errors.New("PostgreSQL 连接参数无效")
		}
		return sql.OpenDB(connector), nil
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = c.Username, c.Password, "tcp", address, c.Database
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 8*time.Second, 8*time.Second
	cfg.MaxAllowedPacket = 2 << 20
	cfg.TLSConfig = "true"
	if c.TLS == "disable" {
		cfg.TLSConfig = "false"
	}
	cfg.Logger = silentSQLLogger{} // Never leak driver errors/credentials to default logs.
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, errors.New("MySQL 连接参数无效")
	}
	return sql.OpenDB(connector), nil
}

func Connect(ctx context.Context, r ConnectRequest) (SQLResult, error) {
	result := SQLResult{Columns: []string{}, Rows: [][]any{}}
	statement, err := connectStatement(r)
	if err != nil {
		return result, err
	}
	if r.Write && (r.Action != "query" || !r.Confirmed) {
		return result, errors.New("写入仅允许明确确认后的 SQL 操作")
	}
	if r.Connection.Engine == "sqlite" {
		return QuerySQLite(ctx, SQLRequest{Path: r.Connection.Path, SQL: statement, Write: r.Write, Confirmed: r.Confirmed})
	}
	select {
	case connectSlots <- struct{}{}:
		defer func() { <-connectSlots }()
	default:
		return result, ErrSQLiteBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	connection := r.Connection
	connection.querySchema = r.Schema
	db, err := openRemote(connection)
	if err != nil {
		return result, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	// Discard raw driver messages: server errors can echo SQL and credentials.
	safeError := func() (SQLResult, error) {
		return result, errors.New("数据库请求失败，请检查连接、TLS、权限及 SQL；超时后可重试，写入前先核对是否已生效")
	}
	if r.Write {
		out, err := db.ExecContext(ctx, statement)
		if err != nil {
			return safeError()
		}
		result.Affected, err = out.RowsAffected()
		if err != nil {
			return safeError()
		}
		return result, nil
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return safeError()
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return safeError()
	}
	defer rows.Close()
	result, err = readRemoteRows(rows)
	if err != nil {
		return safeError()
	}
	return result, nil
}

func readRemoteRows(rows *sql.Rows) (SQLResult, error) {
	result := SQLResult{Columns: []string{}, Rows: [][]any{}}
	var err error
	result.Columns, err = rows.Columns()
	if err != nil {
		return result, err
	}
	if len(result.Columns) > 256 {
		return result, errors.New("列数超过限制")
	}
	types, err := rows.ColumnTypes()
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
		for i, v := range values {
			if value, ok := v.([]byte); ok && len(value) > 1<<20 {
				result.Truncated = true
				return result, nil
			}
			if value, ok := v.(string); ok && len(value) > 1<<20 {
				result.Truncated = true
				return result, nil
			}
			switch value := v.(type) {
			case []byte:
				kind := strings.ToUpper(types[i].DatabaseTypeName())
				if strings.Contains(kind, "BLOB") || strings.Contains(kind, "BINARY") || kind == "BYTEA" || kind == "BIT" {
					values[i] = map[string]string{"base64": base64.StdEncoding.EncodeToString(value)}
				} else {
					values[i] = string(value)
				}
			case int64:
				if value > 9007199254740991 || value < -9007199254740991 {
					values[i] = strconv.FormatInt(value, 10)
				}
			case uint64:
				values[i] = strconv.FormatUint(value, 10)
			case time.Time:
				values[i] = value.Format(time.RFC3339Nano)
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
