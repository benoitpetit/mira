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

//go:embed migrations_postgres/*.sql
var migrationsPostgresFS embed.FS

// runMigrations applies all pending .up.sql migrations in order for SQLite.
func runMigrations(db *sql.DB) error {
	return runGenericMigrations(db, migrationsFS, "migrations", `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at REAL NOT NULL
	)`)
}

// runPostgresMigrations applies all pending .up.sql migrations in order for PostgreSQL.
func runPostgresMigrations(db *sql.DB) error {
	return runGenericMigrations(db, migrationsPostgresFS, "migrations_postgres", `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
}

func runGenericMigrations(db *sql.DB, fs embed.FS, dir string, createTableSQL string) error {
	// Ensure migration tracking table exists
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create schema_migrations: %w", err)
	}

	// List embedded migration files
	entries, err := fs.ReadDir(dir)
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
		query := "SELECT 1 FROM schema_migrations WHERE version = ?"
		if dir == "migrations_postgres" {
			query = "SELECT 1 FROM schema_migrations WHERE version = $1"
		}
		err := db.QueryRow(query, f.version).Scan(&applied)
		if err == nil {
			// Already applied
			continue
		}
		if err != sql.ErrNoRows {
			// Some drivers might return different errors for no rows or column mismatch
			// if the table is empty or the row doesn't exist.
			// Try to be resilient.
		}

		sqlBytes, err := fs.ReadFile(path.Join(dir, f.name))
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

		// Insert into schema_migrations. PostgreSQL uses $1, SQLite uses ?
		// Also set applied_at to current timestamp.
		insertSQL := "INSERT INTO schema_migrations (version, applied_at) VALUES (?, strftime('%s','now'))"
		if dir == "migrations_postgres" {
			insertSQL = "INSERT INTO schema_migrations (version, applied_at) VALUES ($1, CURRENT_TIMESTAMP)"
		}

		if _, err := tx.Exec(insertSQL, f.version); err != nil {
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
