package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/illenko/whodidthis/models"
)

type SnapshotsRepository struct {
	db *DB
}

func NewSnapshotsRepository(db *DB) *SnapshotsRepository {
	return &SnapshotsRepository{db: db}
}

func (r *SnapshotsRepository) Create(ctx context.Context, s *models.Snapshot) (int64, error) {
	query := `
		INSERT INTO snapshots (collected_at, scan_duration_ms, total_services, total_series)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.db.conn.ExecContext(ctx, query,
		s.CollectedAt.Format(time.RFC3339),
		s.ScanDurationMs,
		s.TotalServices,
		s.TotalSeries,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *SnapshotsRepository) Update(ctx context.Context, s *models.Snapshot) error {
	query := `
		UPDATE snapshots
		SET scan_duration_ms = ?, total_services = ?, total_series = ?
		WHERE id = ?
	`
	_, err := r.db.conn.ExecContext(ctx, query,
		s.ScanDurationMs,
		s.TotalServices,
		s.TotalSeries,
		s.ID,
	)
	return err
}

func (r *SnapshotsRepository) GetLatest(ctx context.Context) (*models.Snapshot, error) {
	query := `
		SELECT id, collected_at, scan_duration_ms, total_services, total_series
		FROM snapshots
		ORDER BY collected_at DESC
		LIMIT 1
	`
	return r.scanOne(r.db.conn.QueryRowContext(ctx, query))
}

func (r *SnapshotsRepository) GetByID(ctx context.Context, id int64) (*models.Snapshot, error) {
	query := `
		SELECT id, collected_at, scan_duration_ms, total_services, total_series
		FROM snapshots
		WHERE id = ?
	`
	return r.scanOne(r.db.conn.QueryRowContext(ctx, query, id))
}

func (r *SnapshotsRepository) List(ctx context.Context, limit int) ([]models.Snapshot, error) {
	query := `
		SELECT id, collected_at, scan_duration_ms, total_services, total_series
		FROM snapshots
		ORDER BY collected_at DESC
		LIMIT ?
	`
	rows, err := r.db.conn.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []models.Snapshot
	for rows.Next() {
		s, err := r.scanFromRows(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, *s)
	}
	return snapshots, rows.Err()
}

func (r *SnapshotsRepository) GetByDate(ctx context.Context, date time.Time) (*models.Snapshot, error) {
	// Find snapshot closest to the given date (same day)
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
		SELECT id, collected_at, scan_duration_ms, total_services, total_series
		FROM snapshots
		WHERE collected_at >= ? AND collected_at < ?
		ORDER BY collected_at DESC
		LIMIT 1
	`
	return r.scanOne(r.db.conn.QueryRowContext(ctx, query,
		startOfDay.Format(time.RFC3339),
		endOfDay.Format(time.RFC3339),
	))
}

func (r *SnapshotsRepository) GetNDaysAgo(ctx context.Context, days int) (*models.Snapshot, error) {
	targetDate := time.Now().AddDate(0, 0, -days)
	return r.GetByDate(ctx, targetDate)
}

func (r *SnapshotsRepository) DeleteOlderThan(ctx context.Context, days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := r.db.conn.ExecContext(ctx,
		"DELETE FROM snapshots WHERE collected_at < ?",
		cutoff.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *SnapshotsRepository) scanOne(row *sql.Row) (*models.Snapshot, error) {
	var s models.Snapshot
	var collectedAt string
	var scanDuration sql.NullInt64

	err := row.Scan(&s.ID, &collectedAt, &scanDuration, &s.TotalServices, &s.TotalSeries)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.CollectedAt, err = time.Parse(time.RFC3339, collectedAt)
	if err != nil {
		return nil, err
	}
	if scanDuration.Valid {
		s.ScanDurationMs = int(scanDuration.Int64)
	}
	return &s, nil
}

func (r *SnapshotsRepository) scanFromRows(rows *sql.Rows) (*models.Snapshot, error) {
	var s models.Snapshot
	var collectedAt string
	var scanDuration sql.NullInt64

	err := rows.Scan(&s.ID, &collectedAt, &scanDuration, &s.TotalServices, &s.TotalSeries)
	if err != nil {
		return nil, err
	}

	s.CollectedAt, err = time.Parse(time.RFC3339, collectedAt)
	if err != nil {
		return nil, err
	}
	if scanDuration.Valid {
		s.ScanDurationMs = int(scanDuration.Int64)
	}
	return &s, nil
}
