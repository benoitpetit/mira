package agentmemory

import (
	"strconv"
	"strings"
)

// SQLDialect identifies the database syntax used by the shared MIRA store.
type SQLDialect string

const (
	SQLiteDialect     SQLDialect = "sqlite"
	PostgreSQLDialect SQLDialect = "postgres"
)

func normalizeDialect(value string) SQLDialect {
	if strings.EqualFold(strings.TrimSpace(value), string(PostgreSQLDialect)) || strings.EqualFold(strings.TrimSpace(value), "postgresql") {
		return PostgreSQLDialect
	}
	return SQLiteDialect
}

// bindPlaceholders keeps the runtime SQL readable while supporting both
// database/sql drivers used by MIRA. Queries in this package only use '?'
// placeholders and do not contain literal question marks.
func bindPlaceholders(query string, dialect SQLDialect) string {
	if dialect != PostgreSQLDialect || !strings.Contains(query, "?") {
		return query
	}
	var builder strings.Builder
	builder.Grow(len(query) + 8)
	index := 1
	for _, char := range query {
		if char == '?' {
			builder.WriteByte('$')
			builder.WriteString(intString(index))
			index++
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func intString(value int) string {
	return strconv.Itoa(value)
}
