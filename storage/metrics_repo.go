package storage

import (
	"context"
	"database/sql"
	"fmt"

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
		return 0, err
	}
	return result.LastInsertId()
}

func (r *MetricsRepository) CreateBatch(ctx context.Context, metrics []*models.MetricSnapshot) error {
	tx, err := r.db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metric_snapshots (service_snapshot_id, metric_name, series_count, label_count)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range metrics {
		_, err = stmt.ExecContext(ctx, m.ServiceSnapshotID, m.MetricName, m.SeriesCount, m.LabelCount)
		if err != nil {
			return err
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

	orderDir := "DESC"
	if opts.Order == "asc" {
		orderDir = "ASC"
	}

	switch opts.Sort {
	case "name":
		query += fmt.Sprintf(" ORDER BY metric_name %s", orderDir)
	default:
		query += fmt.Sprintf(" ORDER BY series_count %s", orderDir)
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
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
