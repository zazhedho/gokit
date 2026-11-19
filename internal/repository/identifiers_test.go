package repository

import (
	"strings"
	"testing"
)

func TestIdentifierValidationIsExact(t *testing.T) {
	for _, column := range []string{"name", "users.email", "_internal_id", "role_id2"} {
		if !isSafeColumnIdentifier(column) {
			t.Errorf("%q must be safe", column)
		}
	}
	for _, column := range []string{
		"", "1name", "name;", "users..email", "name OR 1=1", "name DESC",
		" name", "name ", "\tname", "name\n", "a.b.c", "na\"me", "name--", "é",
	} {
		if isSafeColumnIdentifier(column) {
			t.Errorf("%q must be unsafe (exact string is validated, not a trimmed copy)", column)
		}
	}
}

func TestQuotingUsesClauseColumn(t *testing.T) {
	db := newDryRunDB(t)
	var rows []sampleRecord
	q := db.Model(&sampleRecord{}).Where(whereEq("users.email", "x")).Find(&rows)
	if sql := q.Statement.SQL.String(); !strings.Contains(sql, `"users"."email" = `) {
		t.Fatalf("identifiers must be quoted, got %s", sql)
	}
}
