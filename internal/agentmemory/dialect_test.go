package agentmemory

import "testing"

func TestPostgreSQLPlaceholderBinding(t *testing.T) {
	query := "SELECT * FROM identities WHERE agent_id=? ORDER BY version DESC LIMIT ?"
	want := "SELECT * FROM identities WHERE agent_id=$1 ORDER BY version DESC LIMIT $2"
	if got := bindPlaceholders(query, PostgreSQLDialect); got != want {
		t.Fatalf("bindPlaceholders() = %q, want %q", got, want)
	}
}

func TestSQLitePlaceholderBindingIsUnchanged(t *testing.T) {
	query := "SELECT * FROM identities WHERE agent_id=? LIMIT ?"
	if got := bindPlaceholders(query, SQLiteDialect); got != query {
		t.Fatalf("SQLite query changed: %q", got)
	}
}

func TestNormalizeDialect(t *testing.T) {
	if normalizeDialect("postgresql") != PostgreSQLDialect {
		t.Fatal("postgresql should normalize to the PostgreSQL dialect")
	}
	if normalizeDialect("sqlite") != SQLiteDialect {
		t.Fatal("sqlite should normalize to the SQLite dialect")
	}
}
