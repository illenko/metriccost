package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/illenko/metriccost/models"
)

type MetricsRepository struct {
	db *DB
}

func NewMetricsRepository(db *DB) *MetricsRepository {
	return &MetricsRepository{db: db}
}

func (r *MetricsRepository) Save(ctx context.Context, m *models.MetricSnapshot) error {
	labelsJSON, err := json.Marshal(m.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	query := `
		INSERT INTO metric_snapshots (collected_at, metric_name, cardinality, estimated_size_bytes, sample_count, team, labels_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(metric_name, collected_at) DO UPDATE SET
			cardinality = excluded.cardinality,
			estimated_size_bytes = excluded.estimated_size_bytes,
			sample_count = excluded.sample_count,
			team = excluded.team,
			labels_json = excluded.labels_json
	`

	_, err = r.db.conn.ExecContext(ctx, query,
		m.CollectedAt.Format(time.RFC3339), m.MetricName, m.Cardinality, m.EstimatedSizeBytes,
		m.SampleCount, m.Team, string(labelsJSON),
	)
	return err
}

func (r *MetricsRepository) SaveBatch(ctx context.Context, metrics []*models.MetricSnapshot) error {
	tx, err := r.db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO metric_snapshots (collected_at, metric_name, cardinality, estimated_size_bytes, sample_count, team, labels_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(metric_name, collected_at) DO UPDATE SET
			cardinality = excluded.cardinality,
			estimated_size_bytes = excluded.estimated_size_bytes,
			sample_count = excluded.sample_count,
			team = excluded.team,
			labels_json = excluded.labels_json
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range metrics {
		labelsJSON, _ := json.Marshal(m.Labels)
		_, err = stmt.ExecContext(ctx,
			m.CollectedAt.Format(time.RFC3339), m.MetricName, m.Cardinality, m.EstimatedSizeBytes,
			m.SampleCount, m.Team, string(labelsJSON),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

type ListOptions struct {
	Limit  int
	Offset int
	SortBy string // cardinality, size, name
	Team   string
	Search string
}

func (r *MetricsRepository) List(ctx context.Context, collectedAt time.Time, opts ListOptions) ([]*models.MetricSnapshot, error) {
	query := `
		SELECT id, collected_at, metric_name, cardinality, estimated_size_bytes, sample_count, team, labels_json
		FROM metric_snapshots
		WHERE collected_at = ?
	`
	args := []interface{}{collectedAt}

	if opts.Team != "" {
		query += " AND team = ?"
		args = append(args, opts.Team)
	}

	if opts.Search != "" {
		query += " AND metric_name LIKE ?"
		args = append(args, "%"+opts.Search+"%")
	}

	switch opts.SortBy {
	case "cardinality":
		query += " ORDER BY cardinality DESC"
	case "size":
		query += " ORDER BY estimated_size_bytes DESC"
	case "name":
		query += " ORDER BY metric_name ASC"
	default:
		query += " ORDER BY estimated_size_bytes DESC"
	}

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := r.db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMetrics(rows)
}

func (r *MetricsRepository) GetLatestCollectionTime(ctx context.Context) (time.Time, error) {
	var s sql.NullString
	err := r.db.conn.QueryRowContext(ctx,
		"SELECT MAX(collected_at) FROM metric_snapshots",
	).Scan(&s)
	if err != nil {
		return time.Time{}, err
	}
	if !s.Valid || s.String == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s.String)
}

func (r *MetricsRepository) GetByName(ctx context.Context, name string) (*models.MetricSnapshot, error) {
	query := `
		SELECT id, collected_at, metric_name, cardinality, estimated_size_bytes, sample_count, team, labels_json
		FROM metric_snapshots
		WHERE metric_name = ?
		ORDER BY collected_at DESC
		LIMIT 1
	`

	row := r.db.conn.QueryRowContext(ctx, query, name)
	return r.scanMetric(row)
}

func (r *MetricsRepository) GetTrend(ctx context.Context, name string, current, previous time.Time) (float64, error) {
	var currentCard, previousCard int

	err := r.db.conn.QueryRowContext(ctx,
		"SELECT cardinality FROM metric_snapshots WHERE metric_name = ? AND collected_at = ?",
		name, current,
	).Scan(&currentCard)
	if err != nil {
		return 0, err
	}

	err = r.db.conn.QueryRowContext(ctx,
		"SELECT cardinality FROM metric_snapshots WHERE metric_name = ? AND collected_at = ?",
		name, previous,
	).Scan(&previousCard)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	if previousCard == 0 {
		return 0, nil
	}

	return float64(currentCard-previousCard) / float64(previousCard) * 100, nil
}

func (r *MetricsRepository) scanMetrics(rows *sql.Rows) ([]*models.MetricSnapshot, error) {
	var metrics []*models.MetricSnapshot
	for rows.Next() {
		m, err := r.scanMetricFromRows(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

func (r *MetricsRepository) scanMetricFromRows(rows *sql.Rows) (*models.MetricSnapshot, error) {
	var m models.MetricSnapshot
	var collectedAt string
	var labelsJSON sql.NullString
	var team sql.NullString
	var sampleCount sql.NullInt64

	err := rows.Scan(
		&m.ID, &collectedAt, &m.MetricName, &m.Cardinality,
		&m.EstimatedSizeBytes, &sampleCount, &team, &labelsJSON,
	)
	if err != nil {
		return nil, err
	}

	m.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
	if team.Valid {
		m.Team = team.String
	}
	if sampleCount.Valid {
		m.SampleCount = int(sampleCount.Int64)
	}
	if labelsJSON.Valid && labelsJSON.String != "" {
		if err := json.Unmarshal([]byte(labelsJSON.String), &m.Labels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
		}
	}

	return &m, nil
}

func (r *MetricsRepository) scanMetric(row *sql.Row) (*models.MetricSnapshot, error) {
	var m models.MetricSnapshot
	var collectedAt string
	var labelsJSON sql.NullString
	var team sql.NullString
	var sampleCount sql.NullInt64

	err := row.Scan(
		&m.ID, &collectedAt, &m.MetricName, &m.Cardinality,
		&m.EstimatedSizeBytes, &sampleCount, &team, &labelsJSON,
	)
	if err != nil {
		return nil, err
	}

	m.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
	if team.Valid {
		m.Team = team.String
	}
	if sampleCount.Valid {
		m.SampleCount = int(sampleCount.Int64)
	}
	if labelsJSON.Valid && labelsJSON.String != "" {
		if err := json.Unmarshal([]byte(labelsJSON.String), &m.Labels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
		}
	}

	return &m, nil
}
