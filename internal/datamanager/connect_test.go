package datamanager

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestConnectStatementBoundaries(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "mariadb", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			r := ConnectRequest{Connection: Connection{Engine: engine}, Action: "browse", Schema: "odd'schema", Table: "a\"`table", Offset: 100}
			statement, err := connectStatement(r)
			if err != nil || !strings.Contains(statement, quoteObject(engine, r.Table)) || !strings.HasSuffix(statement, "OFFSET 100") {
				t.Fatal(statement, err)
			}
			r.Offset = -1
			if _, err := connectStatement(r); err == nil {
				t.Fatal("negative offset accepted")
			}
			for _, query := range []string{"SELECT 1; DELETE FROM x", "SELECT 1 /*! INTO OUTFILE '/tmp/x' */", "COMMIT", "SET TRANSACTION READ WRITE", "SELECT $$a;COMMIT$$", "SELECT 'a\\'; COMMIT", "SELECT 1 INTO OUTFILE '/tmp/x'"} {
				r.Action, r.SQL, r.Offset = "query", query, 0
				if _, err := connectStatement(r); err == nil {
					t.Fatalf("unsafe query accepted: %s", query)
				}
			}
		})
	}
	if _, err := Connect(context.Background(), ConnectRequest{Connection: Connection{Engine: "postgres"}, Action: "query", SQL: "DELETE FROM demo", Write: true}); err == nil {
		t.Fatal("unconfirmed write accepted")
	}
}

func TestConnectSQLite(t *testing.T) {
	path, err := EnsureDefaultSQLite(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := Connection{Engine: "sqlite", Path: path}
	for _, query := range []string{"CREATE TABLE demo(id INTEGER PRIMARY KEY, value TEXT)", "INSERT INTO demo VALUES(1, 'hello')"} {
		if _, err := Connect(context.Background(), ConnectRequest{Connection: c, Action: "query", SQL: query, Write: true, Confirmed: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, action := range []string{"test", "databases", "schemas", "tables", "structure", "indexes", "browse"} {
		result, err := Connect(context.Background(), ConnectRequest{Connection: c, Action: action, Schema: "main", Table: "demo"})
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if action == "browse" && (len(result.Rows) != 1 || result.Rows[0][1] != "hello") {
			t.Fatal(result)
		}
	}
}

// Only explicitly provisioned loopback test containers are used; no user DSNs.
func TestConnectRemoteIntegration(t *testing.T) {
	for _, item := range []struct{ engine, env, user, database string }{
		{"postgres", "ALX_CONNECT_TEST_PG_PORT", "postgres", "postgres"},
		{"mariadb", "ALX_CONNECT_TEST_MARIA_PORT", "root", "alx_connect_test"},
		{"mysql", "ALX_CONNECT_TEST_MYSQL_PORT", "root", "alx_connect_test"},
	} {
		t.Run(item.engine, func(t *testing.T) {
			port, _ := strconv.Atoi(os.Getenv(item.env))
			if port == 0 {
				t.Skip("explicit loopback test container not configured")
			}
			c := Connection{Engine: item.engine, Host: "127.0.0.1", Port: port, Username: item.user, Password: "alx-test-only", Database: item.database, TLS: "disable"}
			scope := item.database
			if item.engine == "postgres" {
				scope = "public"
			}
			ctx := context.Background()
			table := "alx_connect_fixture"
			qualified := quoteObject(item.engine, scope) + "." + quoteObject(item.engine, table)
			db, err := openRemote(c)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err = db.ExecContext(ctx, "CREATE TABLE "+qualified+" (id BIGINT PRIMARY KEY, value VARCHAR(100))"); err != nil {
				t.Fatal(err)
			}
			defer db.ExecContext(ctx, "DROP TABLE "+qualified)
			if _, err = Connect(ctx, ConnectRequest{Connection: c, Action: "query", SQL: "INSERT INTO " + qualified + " VALUES (9007199254740993, 'hello')", Write: true, Confirmed: true}); err != nil {
				t.Fatal(err)
			}
			for _, action := range []string{"test", "databases", "schemas", "tables", "structure", "indexes", "browse"} {
				result, err := Connect(ctx, ConnectRequest{Connection: c, Action: action, Schema: scope, Table: table})
				if err != nil {
					t.Fatalf("%s: %v", action, err)
				}
				if action == "browse" && (len(result.Rows) != 1 || result.Rows[0][0] != "9007199254740993" || result.Rows[0][1] != "hello") {
					t.Fatal(result)
				}
			}
			if item.engine == "postgres" {
				if _, err := db.ExecContext(ctx, "CREATE SCHEMA alx_scope_test"); err != nil {
					t.Fatal(err)
				}
				defer db.ExecContext(ctx, "DROP SCHEMA alx_scope_test")
				result, err := Connect(ctx, ConnectRequest{Connection: c, Action: "query", Schema: "alx_scope_test", SQL: "SELECT current_schema()"})
				if err != nil || len(result.Rows) != 1 || result.Rows[0][0] != "alx_scope_test" {
					t.Fatal("schema context", result, err)
				}
				_, err = Connect(ctx, ConnectRequest{Connection: c, Action: "query", SQL: "WITH changed AS (DELETE FROM " + qualified + " RETURNING *) SELECT * FROM changed"})
				if err == nil {
					t.Fatal("read-only transaction allowed persistent write")
				}
			}
			bad := c
			bad.Password = "do-not-echo-password"
			_, err = Connect(ctx, ConnectRequest{Connection: bad, Action: "test"})
			if err == nil || strings.Contains(err.Error(), bad.Password) {
				t.Fatal("failed auth leaked or succeeded", err)
			}
			tls := c
			tls.TLS = "verify-full"
			if _, err = Connect(ctx, ConnectRequest{Connection: tls, Action: "test"}); err == nil {
				t.Fatal("untrusted test server certificate accepted")
			}
		})
	}
}
