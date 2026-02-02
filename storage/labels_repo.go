package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/illenko/whodidthis/models"
)

type LabelsRepository struct {
	db *DB
}

func NewLabelsRepository(db *DB) *LabelsRepository {
	return &LabelsRepository{db: db}
}

func (r *LabelsRepository) Create(ctx context.Context, l *models.LabelSnapshot) (int64, error) {
	sampleJSON, err := json.Marshal(l.SampleValues)
	if err != nil {
		return 0, err
	}

	query := `
		INSERT INTO label_snapshots (metric_snapshot_id, label_name, unique_values_count, sample_values)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.db.conn.ExecContext(ctx, query,
		l.MetricSnapshotID,
		l.LabelName,
		l.UniqueValuesCount,
		string(sampleJSON),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *LabelsRepository) CreateBatch(ctx context.Context, labels []*models.LabelSnapshot) error {
	tx, err := r.db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO label_snapshots (metric_snapshot_id, label_name, unique_values_count, sample_values)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range labels {
		sampleJSON, _ := json.Marshal(l.SampleValues)
		_, err = stmt.ExecContext(ctx, l.MetricSnapshotID, l.LabelName, l.UniqueValuesCount, string(sampleJSON))
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *LabelsRepository) List(ctx context.Context, metricSnapshotID int64) ([]models.LabelSnapshot, error) {
	query := `
		SELECT id, metric_snapshot_id, label_name, unique_values_count, sample_values
		FROM label_snapshots
		WHERE metric_snapshot_id = ?
		ORDER BY unique_values_count DESC
	`
	rows, err := r.db.conn.QueryContext(ctx, query, metricSnapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []models.LabelSnapshot
	for rows.Next() {
		l, err := r.scanFromRows(rows)
		if err != nil {
			return nil, err
		}
		labels = append(labels, *l)
	}
	return labels, rows.Err()
}

func (r *LabelsRepository) GetByName(ctx context.Context, metricSnapshotID int64, name string) (*models.LabelSnapshot, error) {
	query := `
		SELECT id, metric_snapshot_id, label_name, unique_values_count, sample_values
		FROM label_snapshots
		WHERE metric_snapshot_id = ? AND label_name = ?
	`
	row := r.db.conn.QueryRowContext(ctx, query, metricSnapshotID, name)

	var l models.LabelSnapshot
	var sampleJSON sql.NullString
	err := row.Scan(&l.ID, &l.MetricSnapshotID, &l.LabelName, &l.UniqueValuesCount, &sampleJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sampleJSON.Valid && sampleJSON.String != "" {
		json.Unmarshal([]byte(sampleJSON.String), &l.SampleValues)
	}
	return &l, nil
}

func (r *LabelsRepository) scanFromRows(rows *sql.Rows) (*models.LabelSnapshot, error) {
	var l models.LabelSnapshot
	var sampleJSON sql.NullString

	err := rows.Scan(&l.ID, &l.MetricSnapshotID, &l.LabelName, &l.UniqueValuesCount, &sampleJSON)
	if err != nil {
		return nil, err
	}

	if sampleJSON.Valid && sampleJSON.String != "" {
		json.Unmarshal([]byte(sampleJSON.String), &l.SampleValues)
	}
	return &l, nil
}
