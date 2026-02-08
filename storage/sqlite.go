package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}

	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	db := &DB{conn: conn}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	if _, err := db.conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		var count int
		if err := db.conn.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", entry.Name()).Scan(&count); err != nil {
			return fmt.Errorf("failed to check migration %s: %w", entry.Name(), err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
		}

		slog.Info("applying migration", "file", entry.Name())

		tx, err := db.conn.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", entry.Name(), err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", entry.Name(), err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			entry.Name(), time.Now().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", entry.Name(), err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func (db *DB) Stats(ctx context.Context) (*DBStats, error) {
	var stats DBStats

	row := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM snapshots")
	if err := row.Scan(&stats.SnapshotsCount); err != nil {
		return nil, err
	}

	row = db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM service_snapshots")
	if err := row.Scan(&stats.ServiceSnapshotsCount); err != nil {
		return nil, err
	}

	row = db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM metric_snapshots")
	if err := row.Scan(&stats.MetricSnapshotsCount); err != nil {
		return nil, err
	}

	row = db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM label_snapshots")
	if err := row.Scan(&stats.LabelSnapshotsCount); err != nil {
		return nil, err
	}

	row = db.conn.QueryRowContext(ctx, "SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()")
	if err := row.Scan(&stats.SizeBytes); err != nil {
		stats.SizeBytes = 0
	}

	return &stats, nil
}

type DBStats struct {
	SnapshotsCount        int64
	ServiceSnapshotsCount int64
	MetricSnapshotsCount  int64
	LabelSnapshotsCount   int64
	SizeBytes             int64
}

func (db *DB) Cleanup(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention).Format(time.RFC3339)

	// Due to CASCADE deletes, we only need to delete from snapshots
	result, err := db.conn.ExecContext(ctx,
		"DELETE FROM snapshots WHERE collected_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup snapshots: %w", err)
	}
	deleted, _ := result.RowsAffected()

	if _, err := db.conn.ExecContext(ctx, "VACUUM"); err != nil {
		slog.Warn("failed to vacuum database", "error", err)
	}

	return deleted, nil
}

func (db *DB) Conn() *sql.DB {
	return db.conn
}
