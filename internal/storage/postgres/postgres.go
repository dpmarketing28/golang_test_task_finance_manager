// Package postgres is the only place in the project that knows SQL. It
// implements the repository interfaces declared by the usecase package
// and owns schema migrations.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"

	// pgx registers itself under the "pgx" driver name for database/sql.
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect opens a connection pool to Postgres and verifies it is reachable.
func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return db, nil
}

// Migrate applies every embedded migration that has not run yet, in
// filename order, each in its own transaction, tracked in
// schema_migrations. It is safe to call on every startup.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("postgres: init schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("postgres: read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		if err := db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("postgres: check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		script, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("postgres: read %s: %w", name, err)
		}

		if err := applyMigration(ctx, db, name, string(script)); err != nil {
			return err
		}
		log.Printf("postgres: applied migration %s", name)
	}

	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, name, script string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: begin tx for %s: %w", name, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if already committed

	if _, err := tx.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("postgres: apply %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
		return fmt.Errorf("postgres: record %s: %w", name, err)
	}

	return tx.Commit()
}
