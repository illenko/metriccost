package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/illenko/metriccost/models"
)

type SnapshotsRepository struct {
	db *DB
}

func NewSnapshotsRepository(db *DB) *SnapshotsRepository {
	return &SnapshotsRepository{db: db}
}

func (r *SnapshotsRepository) Save(ctx context.Context, s *models.Snapshot) error {
	teamJSON, err := json.Marshal(s.TeamBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal team breakdown: %w", err)
	}

	query := `
		INSERT INTO snapshots (collected_at, total_metrics, total_cardinality, total_size_bytes, team_breakdown)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(collected_at) DO UPDATE SET
			total_metrics = excluded.total_metrics,
			total_cardinality = excluded.total_cardinality,
			total_size_bytes = excluded.total_size_bytes,
			team_breakdown = excluded.team_breakdown
	`

	_, err = r.db.conn.ExecContext(ctx, query,
		s.CollectedAt.Format(time.RFC3339), s.TotalMetrics, s.TotalCardinality, s.TotalSizeBytes, string(teamJSON),
	)
	return err
}

func (r *SnapshotsRepository) GetLatest(ctx context.Context) (*models.Snapshot, error) {
	query := `
		SELECT id, collected_at, total_metrics, total_cardinality, total_size_bytes, team_breakdown
		FROM snapshots
		ORDER BY collected_at DESC
		LIMIT 1
	`

	var s models.Snapshot
	var collectedAt string
	var teamJSON sql.NullString

	err := r.db.conn.QueryRowContext(ctx, query).Scan(
		&s.ID, &collectedAt, &s.TotalMetrics, &s.TotalCardinality, &s.TotalSizeBytes, &teamJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
	if teamJSON.Valid && teamJSON.String != "" {
		if err := json.Unmarshal([]byte(teamJSON.String), &s.TeamBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal team breakdown: %w", err)
		}
	}

	return &s, nil
}

func (r *SnapshotsRepository) GetTrends(ctx context.Context, since time.Time) ([]*models.Snapshot, error) {
	query := `
		SELECT id, collected_at, total_metrics, total_cardinality, total_size_bytes, team_breakdown
		FROM snapshots
		WHERE collected_at >= ?
		ORDER BY collected_at ASC
	`

	rows, err := r.db.conn.QueryContext(ctx, query, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []*models.Snapshot
	for rows.Next() {
		var s models.Snapshot
		var collectedAt string
		var teamJSON sql.NullString

		err := rows.Scan(
			&s.ID, &collectedAt, &s.TotalMetrics, &s.TotalCardinality, &s.TotalSizeBytes, &teamJSON,
		)
		if err != nil {
			return nil, err
		}

		s.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
		if teamJSON.Valid && teamJSON.String != "" {
			if err := json.Unmarshal([]byte(teamJSON.String), &s.TeamBreakdown); err != nil {
				return nil, fmt.Errorf("failed to unmarshal team breakdown: %w", err)
			}
		}

		snapshots = append(snapshots, &s)
	}

	return snapshots, rows.Err()
}

func (r *SnapshotsRepository) GetPrevious(ctx context.Context, before time.Time) (*models.Snapshot, error) {
	query := `
		SELECT id, collected_at, total_metrics, total_cardinality, total_size_bytes, team_breakdown
		FROM snapshots
		WHERE collected_at < ?
		ORDER BY collected_at DESC
		LIMIT 1
	`

	var s models.Snapshot
	var collectedAt string
	var teamJSON sql.NullString

	err := r.db.conn.QueryRowContext(ctx, query, before.Format(time.RFC3339)).Scan(
		&s.ID, &collectedAt, &s.TotalMetrics, &s.TotalCardinality, &s.TotalSizeBytes, &teamJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
	if teamJSON.Valid && teamJSON.String != "" {
		if err := json.Unmarshal([]byte(teamJSON.String), &s.TeamBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal team breakdown: %w", err)
		}
	}

	return &s, nil
}

func (r *SnapshotsRepository) CalculateTrend(ctx context.Context, current *models.Snapshot) (float64, error) {
	prev, err := r.GetPrevious(ctx, current.CollectedAt)
	if err != nil {
		return 0, err
	}
	if prev == nil || prev.TotalCardinality == 0 {
		return 0, nil
	}

	return float64(current.TotalCardinality-prev.TotalCardinality) / float64(prev.TotalCardinality) * 100, nil
}
