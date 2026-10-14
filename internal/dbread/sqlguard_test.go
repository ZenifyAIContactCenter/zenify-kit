package dbread

import "testing"

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
