package api

import "testing"

func TestDevSQL_RowChannelsRefused(t *testing.T) {
	for _, sql := range []string{
		// boolean channel through SHOW … WHERE
		"SHOW TABLES WHERE (SELECT ASCII(SUBSTR(password,1,1)) FROM users LIMIT 1)>77",
		"SHOW INDEX FROM users WHERE (SELECT ASCII(SUBSTR(password,1,1)) FROM users LIMIT 1)>77",
		"SHOW COLUMNS FROM users WHERE (SELECT ASCII(SUBSTR(password,1,1)) FROM users LIMIT 1)>77",
		"SHOW TABLE STATUS WHERE (SELECT ASCII(SUBSTR(password,1,1)) FROM users LIMIT 1)>77",
		"SHOW TABLES WHERE Tables_in_db = 'x'",
		// error channel
		"SHOW TABLES WHERE EXTRACTVALUE(1,CONCAT(0x7e,(SELECT password FROM users LIMIT 1)))",
		"SHOW TABLES LIKE (SELECT password FROM users LIMIT 1)",
		// EXPLAIN ANALYZE executes
		"explain analyze select 1",
		"EXPLAIN ANALYZE SELECT * FROM users",
		"EXPLAIN FORMAT=TREE ANALYZE SELECT * FROM users",
		"EXPLAIN ANALYZE FORMAT=TREE SELECT * FROM users",
		// constant subqueries are evaluated while planning
		"EXPLAIN SELECT * FROM users WHERE id = (SELECT 1 FROM users WHERE ASCII(password)>77)",
		"EXPLAIN SELECT * FROM users WHERE id IN (TABLE t)",
		"EXPLAIN SELECT 1 UNION SELECT password FROM users",
		"EXPLAIN WITH t AS (SELECT 1) SELECT * FROM t",
		"EXPLAIN FORMAT = JSON WITH t AS (SELECT 1) SELECT * FROM t",
		// DESC must name a table
		"DESC (SELECT password FROM users)",
		"DESCRIBE (SELECT 1)",
		"DESC",
		// shape
		"SHOW TABLES; SELECT 1",
		"EXPLAIN SELECT 1 /* x */",
		"SHOW TABLES LIKE 'x",
	} {
		if devSQL(sql) {
			t.Errorf("devSQL(%q) = true, want false", sql)
		}
	}
}

func TestDevSQL_PlanAndSchemaAllowed(t *testing.T) {
	for _, sql := range []string{
		"EXPLAIN SELECT * FROM users WHERE email = 'x'",
		"EXPLAIN SELECT u.id FROM users u JOIN orders o ON o.user_id = u.id WHERE u.email = 'select'",
		"EXPLAIN FORMAT=JSON SELECT * FROM users WHERE id = 1",
		"DESC SELECT * FROM users WHERE id = 1",
		"SHOW INDEX FROM users",
		"SHOW CREATE TABLE users",
		"DESCRIBE users",
		"DESCRIBE `users`",
		"  desc db.users",
		"SHOW TABLE STATUS LIKE 'users'",
		"SHOW TABLE STATUS",
		"SHOW COLUMNS FROM users",
		"SHOW COLUMNS FROM users IN db LIKE 'e%'",
		"SHOW TABLES;",
		"/* x */ EXPLAIN SELECT 1",
	} {
		if !devSQL(sql) {
			t.Errorf("devSQL(%q) = false, want true", sql)
		}
	}
}
