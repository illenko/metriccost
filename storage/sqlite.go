package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
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
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
		}

		slog.Debug("running migration", "file", entry.Name())

		if _, err := db.conn.Exec(string(content)); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func (db *DB) Stats(ctx context.Context) (*DBStats, error) {
	var stats DBStats

	row := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM metric_snapshots")
	if err := row.Scan(&stats.MetricSnapshotsCount); err != nil {
		return nil, err
	}

	row = db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM recommendations")
	if err := row.Scan(&stats.RecommendationsCount); err != nil {
		return nil, err
	}

	row = db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM dashboard_stats")
	if err := row.Scan(&stats.DashboardStatsCount); err != nil {
		return nil, err
	}

	row = db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM snapshots")
	if err := row.Scan(&stats.SnapshotsCount); err != nil {
		return nil, err
	}

	row = db.conn.QueryRowContext(ctx, "SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()")
	if err := row.Scan(&stats.SizeBytes); err != nil {
		stats.SizeBytes = 0
	}

	return &stats, nil
}

type DBStats struct {
	MetricSnapshotsCount int64
	RecommendationsCount int64
	DashboardStatsCount  int64
	SnapshotsCount       int64
	SizeBytes            int64
}

func (db *DB) Cleanup(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	var totalDeleted int64

	tables := []string{"metric_snapshots", "dashboard_stats", "snapshots"}
	for _, table := range tables {
		result, err := db.conn.ExecContext(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE collected_at < ?", table),
			cutoff,
		)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to cleanup %s: %w", table, err)
		}
		deleted, _ := result.RowsAffected()
		totalDeleted += deleted
	}

	if _, err := db.conn.ExecContext(ctx, "VACUUM"); err != nil {
		slog.Warn("failed to vacuum database", "error", err)
	}

	return totalDeleted, nil
}

func (db *DB) Conn() *sql.DB {
	return db.conn
}
