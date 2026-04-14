// Package storage provides SQLite persistence and migration adapters.
package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// runMigrations applies all pending .up.sql migrations in order.
func runMigrations(db *sql.DB) error {
	// Ensure migration tracking table exists
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at REAL NOT NULL DEFAULT (unixepoch())
	) STRICT`); err != nil {
		return fmt.Errorf("failed to create schema_migrations: %w", err)
	}

	// List embedded migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations dir: %w", err)
	}

	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version, err := parseMigrationVersion(entry.Name())
		if err != nil {
			return fmt.Errorf("invalid migration filename %q: %w", entry.Name(), err)
		}
		files = append(files, migrationFile{version: version, name: entry.Name()})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].version < files[j].version
	})

	for _, f := range files {
		var applied bool
		err := db.QueryRow("SELECT 1 FROM schema_migrations WHERE version = ?", f.version).Scan(&applied)
		if err == nil {
			// Already applied
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("failed to check migration %d: %w", f.version, err)
		}

		sqlBytes, err := migrationsFS.ReadFile(path.Join("migrations", f.name))
		if err != nil {
			return fmt.Errorf("failed to read migration %d: %w", f.version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin tx for migration %d: %w", f.version, err)
		}

		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to apply migration %d: %w", f.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", f.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", f.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", f.version, err)
		}
	}

	return nil
}

type migrationFile struct {
	version int
	name    string
}

func parseMigrationVersion(name string) (int, error) {
	// Expects format like 001_initial.up.sql
	parts := strings.SplitN(name, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("expected VERSION_name.up.sql")
	}
	return strconv.Atoi(parts[0])
}
