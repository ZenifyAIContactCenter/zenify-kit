package dbread

import (
	"strings"
	"testing"
)

func TestCheckSQL(t *testing.T) {
	pass := []string{
		"SELECT 1",
		"select created_at, is_deleted from t",
		"  WITH c AS (SELECT 1) SELECT * FROM c",
		"SHOW TABLES",
		"DESC t",
		"EXPLAIN SELECT 1",
		"SELECT 'a;b'",
		`SELECT 'a\\.b'`,
		"SELECT '-- x # y /* z'",
		"SELECT 1;",
		"SELECT update_time, inserted_by FROM t",
		"SELECT REPLACE(a,'x','y') FROM t",
		"SELECT 'delete into update' FROM t",
		"SELECT `into` FROM t",
		`SELECT 'a\''`,
		"SELECT 'a'''",
	}
	for _, s := range pass {
		if err := CheckSQL(s); err != nil {
			t.Errorf("CheckSQL(%q) = %v, want nil", s, err)
		}
	}
	fail := []string{
		"SELECT 1; DELETE FROM t",
		"UPDATE t SET a=1",
		"DROP TABLE t",
		"CREATE TABLE t(a int)",
		`SELECT 1 \! ls`,
		`SELECT 5 \. f.sql`,
		"source f.sql",
		"delimiter //",
		"",
		"SELECT 'a' -- '\n; DROP TABLE x",
		"SELECT 1 # '\n; DROP TABLE x",
		"SELECT 1 /*! ; DROP TABLE x */",
		"/* c */ SELECT 1",
		"WITH c AS (SELECT 1) DELETE FROM t",
		"WITH c AS (SELECT 1) UPDATE t SET a=1",
		"SELECT 1 INTO OUTFILE '/tmp/x'",
		"SELECT a INTO @v FROM t",
		"SELECT * FROM t FOR UPDATE",
		"SELECT * FROM t FOR SHARE",
		"SELECT * FROM t LOCK IN SHARE MODE",
		"SELECT 'a",
		"(SELECT 1)",
	}
	for _, s := range fail {
		if err := CheckSQL(s); err == nil {
			t.Errorf("CheckSQL(%q) = nil, want error", s)
		}
	}
}

func TestWrapSQL(t *testing.T) {
	got := WrapSQL("SELECT 1;", 50)
	want := "START TRANSACTION READ ONLY; SET SESSION sql_mode='ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'; SET SESSION MAX_EXECUTION_TIME=30000; SET SESSION SQL_SELECT_LIMIT=51; SELECT 1; ROLLBACK;"
	if got != want {
		t.Errorf("WrapSQL = %q, want %q", got, want)
	}
}

func TestCheckSQL_ClientDelimiterCommand(t *testing.T) {
	for _, s := range []string{
		"SELECT 1\ndelimiter //\nDROP TABLE t//",
		"SELECT 1\r\ncharset latin1",
	} {
		if err := CheckSQL(s); err == nil {
			t.Errorf("CheckSQL(%q) = nil, want error", s)
		}
	}
}

func TestWrapSQL_FlattensNewlinesOutsideLiterals(t *testing.T) {
	got := WrapSQL("SELECT 1\ndelimiter //\r\nDROP TABLE t//", 50)
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("WrapSQL kept a line break outside literals: %q", got)
	}
	stmt := "SELECT a,\n  'x\ny', `c\nd`, \"e\\\"\nf\"\nFROM t\r\nWHERE b = 1"
	if err := CheckSQL(stmt); err != nil {
		t.Fatalf("multi-line SELECT refused: %v", err)
	}
	got = WrapSQL(stmt, 50)
	if !strings.Contains(got, "SELECT a,   'x\ny', `c\nd`, \"e\\\"\nf\" FROM t  WHERE b = 1;") {
		t.Errorf("WrapSQL = %q, want outer breaks flattened, literal breaks kept", got)
	}
}

func TestWrapSQLNoLimit(t *testing.T) {
	got := WrapSQLNoLimit("SHOW TABLES")
	if strings.Contains(got, "SQL_SELECT_LIMIT") {
		t.Errorf("WrapSQLNoLimit = %q, want no SQL_SELECT_LIMIT", got)
	}
	if !strings.HasPrefix(got, "START TRANSACTION READ ONLY;") || !strings.Contains(got, "MAX_EXECUTION_TIME=30000") || !strings.HasSuffix(got, "SHOW TABLES; ROLLBACK;") {
		t.Errorf("WrapSQLNoLimit = %q", got)
	}
}
