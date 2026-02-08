package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/illenko/whodidthis/models"
)

type MetricsRepository struct {
	db *DB
}

func NewMetricsRepository(db *DB) *MetricsRepository {
	return &MetricsRepository{db: db}
}

func (r *MetricsRepository) Create(ctx context.Context, m *models.MetricSnapshot) (int64, error) {
	query := `
		INSERT INTO metric_snapshots (service_snapshot_id, metric_name, series_count, label_count)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.db.conn.ExecContext(ctx, query,
		m.ServiceSnapshotID,
		m.MetricName,
		m.SeriesCount,
		m.LabelCount,
	)
	if err != nil {
		return 0, fmt.Errorf("insert metric snapshot: %w", err)
	}
	return result.LastInsertId()
}

func (r *MetricsRepository) CreateBatch(ctx context.Context, metrics []*models.MetricSnapshot) error {
	tx, err := r.db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("failed to rollback metrics batch", "error", err)
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metric_snapshots (service_snapshot_id, metric_name, series_count, label_count)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, m := range metrics {
		if _, err = stmt.ExecContext(ctx, m.ServiceSnapshotID, m.MetricName, m.SeriesCount, m.LabelCount); err != nil {
			return fmt.Errorf("insert metric %s: %w", m.MetricName, err)
		}
	}

	return tx.Commit()
}

type MetricListOptions struct {
	Sort  string // "series", "name"
	Order string // "asc", "desc"
}

func (r *MetricsRepository) List(ctx context.Context, serviceSnapshotID int64, opts MetricListOptions) ([]models.MetricSnapshot, error) {
	query := `
		SELECT id, service_snapshot_id, metric_name, series_count, label_count
		FROM metric_snapshots
		WHERE service_snapshot_id = ?
	`

	switch opts.Sort {
	case "name":
		if opts.Order == "asc" {
			query += " ORDER BY metric_name ASC"
		} else {
			query += " ORDER BY metric_name DESC"
		}
	default:
		if opts.Order == "asc" {
			query += " ORDER BY series_count ASC"
		} else {
			query += " ORDER BY series_count DESC"
		}
	}

	rows, err := r.db.conn.QueryContext(ctx, query, serviceSnapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.MetricSnapshot
	for rows.Next() {
		var m models.MetricSnapshot
		if err := rows.Scan(&m.ID, &m.ServiceSnapshotID, &m.MetricName, &m.SeriesCount, &m.LabelCount); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

func (r *MetricsRepository) GetByName(ctx context.Context, serviceSnapshotID int64, name string) (*models.MetricSnapshot, error) {
	query := `
		SELECT id, service_snapshot_id, metric_name, series_count, label_count
		FROM metric_snapshots
		WHERE service_snapshot_id = ? AND metric_name = ?
	`
	var m models.MetricSnapshot
	err := r.db.conn.QueryRowContext(ctx, query, serviceSnapshotID, name).Scan(
		&m.ID, &m.ServiceSnapshotID, &m.MetricName, &m.SeriesCount, &m.LabelCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
