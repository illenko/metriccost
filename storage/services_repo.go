package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/illenko/whodidthis/models"
)

type ServicesRepository struct {
	db *DB
}

func NewServicesRepository(db *DB) *ServicesRepository {
	return &ServicesRepository{db: db}
}

func (r *ServicesRepository) Create(ctx context.Context, s *models.ServiceSnapshot) (int64, error) {
	query := `
		INSERT INTO service_snapshots (snapshot_id, service_name, total_series, metric_count)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.db.conn.ExecContext(ctx, query,
		s.SnapshotID,
		s.ServiceName,
		s.TotalSeries,
		s.MetricCount,
	)
	if err != nil {
		return 0, fmt.Errorf("insert service snapshot: %w", err)
	}
	return result.LastInsertId()
}

func (r *ServicesRepository) CreateBatch(ctx context.Context, services []*models.ServiceSnapshot) error {
	tx, err := r.db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("failed to rollback services batch", "error", err)
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO service_snapshots (snapshot_id, service_name, total_series, metric_count)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, s := range services {
		if _, err = stmt.ExecContext(ctx, s.SnapshotID, s.ServiceName, s.TotalSeries, s.MetricCount); err != nil {
			return fmt.Errorf("insert service %s: %w", s.ServiceName, err)
		}
	}

	return tx.Commit()
}

type ServiceListOptions struct {
	Sort   string // "series", "name"
	Order  string // "asc", "desc"
	Search string
}

func (r *ServicesRepository) List(ctx context.Context, snapshotID int64, opts ServiceListOptions) ([]models.ServiceSnapshot, error) {
	query := `
		SELECT id, snapshot_id, service_name, total_series, metric_count
		FROM service_snapshots
		WHERE snapshot_id = ?
	`
	args := []interface{}{snapshotID}

	if opts.Search != "" {
		query += " AND service_name LIKE ?"
		args = append(args, "%"+opts.Search+"%")
	}

	switch opts.Sort {
	case "name":
		if opts.Order == "asc" {
			query += " ORDER BY service_name ASC"
		} else {
			query += " ORDER BY service_name DESC"
		}
	default:
		if opts.Order == "asc" {
			query += " ORDER BY total_series ASC"
		} else {
			query += " ORDER BY total_series DESC"
		}
	}

	rows, err := r.db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []models.ServiceSnapshot
	for rows.Next() {
		var s models.ServiceSnapshot
		if err := rows.Scan(&s.ID, &s.SnapshotID, &s.ServiceName, &s.TotalSeries, &s.MetricCount); err != nil {
			return nil, err
		}
		services = append(services, s)
	}
	return services, rows.Err()
}

func (r *ServicesRepository) GetByName(ctx context.Context, snapshotID int64, name string) (*models.ServiceSnapshot, error) {
	query := `
		SELECT id, snapshot_id, service_name, total_series, metric_count
		FROM service_snapshots
		WHERE snapshot_id = ? AND service_name = ?
	`
	var s models.ServiceSnapshot
	err := r.db.conn.QueryRowContext(ctx, query, snapshotID, name).Scan(
		&s.ID, &s.SnapshotID, &s.ServiceName, &s.TotalSeries, &s.MetricCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
