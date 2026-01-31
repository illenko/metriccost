package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/illenko/metriccost/models"
)

type RecommendationsRepository struct {
	db *DB
}

func NewRecommendationsRepository(db *DB) *RecommendationsRepository {
	return &RecommendationsRepository{db: db}
}

func (r *RecommendationsRepository) Save(ctx context.Context, rec *models.Recommendation) error {
	query := `
		INSERT INTO recommendations (
			created_at, metric_name, type, priority,
			current_cardinality, current_size_bytes, potential_reduction_bytes,
			description, suggested_action
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.conn.ExecContext(ctx, query,
		rec.CreatedAt.Format(time.RFC3339), rec.MetricName, rec.Type, rec.Priority,
		rec.CurrentCardinality, rec.CurrentSizeBytes, rec.PotentialReductionBytes,
		rec.Description, rec.SuggestedAction,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err == nil {
		rec.ID = id
	}

	return nil
}

func (r *RecommendationsRepository) SaveBatch(ctx context.Context, recs []*models.Recommendation) error {
	tx, err := r.db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO recommendations (
			created_at, metric_name, type, priority,
			current_cardinality, current_size_bytes, potential_reduction_bytes,
			description, suggested_action
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, rec := range recs {
		_, err = stmt.ExecContext(ctx,
			rec.CreatedAt.Format(time.RFC3339), rec.MetricName, rec.Type, rec.Priority,
			rec.CurrentCardinality, rec.CurrentSizeBytes, rec.PotentialReductionBytes,
			rec.Description, rec.SuggestedAction,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *RecommendationsRepository) List(ctx context.Context, priority string) ([]*models.Recommendation, error) {
	query := `
		SELECT id, created_at, metric_name, type, priority,
			   current_cardinality, current_size_bytes, potential_reduction_bytes,
			   description, suggested_action
		FROM recommendations
	`
	var args []interface{}

	if priority != "" {
		query += " WHERE priority = ?"
		args = append(args, priority)
	}

	query += " ORDER BY CASE priority WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END, potential_reduction_bytes DESC"

	rows, err := r.db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRecommendations(rows)
}

func (r *RecommendationsRepository) GetByMetricName(ctx context.Context, metricName string) ([]*models.Recommendation, error) {
	query := `
		SELECT id, created_at, metric_name, type, priority,
			   current_cardinality, current_size_bytes, potential_reduction_bytes,
			   description, suggested_action
		FROM recommendations
		WHERE metric_name = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.conn.QueryContext(ctx, query, metricName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRecommendations(rows)
}

func (r *RecommendationsRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.conn.ExecContext(ctx,
		"DELETE FROM recommendations WHERE created_at < ?",
		before.Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *RecommendationsRepository) Clear(ctx context.Context) error {
	_, err := r.db.conn.ExecContext(ctx, "DELETE FROM recommendations")
	return err
}

func (r *RecommendationsRepository) Count(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT priority, COUNT(*) as count
		FROM recommendations
		GROUP BY priority
	`

	rows, err := r.db.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var priority string
		var count int
		if err := rows.Scan(&priority, &count); err != nil {
			return nil, err
		}
		counts[priority] = count
	}

	return counts, rows.Err()
}

func (r *RecommendationsRepository) scanRecommendations(rows *sql.Rows) ([]*models.Recommendation, error) {
	var recs []*models.Recommendation
	for rows.Next() {
		var rec models.Recommendation
		var createdAt string
		var currentCard, currentSize, potentialReduction sql.NullInt64
		var description, suggestedAction sql.NullString

		err := rows.Scan(
			&rec.ID, &createdAt, &rec.MetricName, &rec.Type, &rec.Priority,
			&currentCard, &currentSize, &potentialReduction,
			&description, &suggestedAction,
		)
		if err != nil {
			return nil, err
		}

		rec.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if currentCard.Valid {
			rec.CurrentCardinality = int(currentCard.Int64)
		}
		if currentSize.Valid {
			rec.CurrentSizeBytes = currentSize.Int64
		}
		if potentialReduction.Valid {
			rec.PotentialReductionBytes = potentialReduction.Int64
		}
		if description.Valid {
			rec.Description = description.String
		}
		if suggestedAction.Valid {
			rec.SuggestedAction = suggestedAction.String
		}

		recs = append(recs, &rec)
	}

	return recs, rows.Err()
}
