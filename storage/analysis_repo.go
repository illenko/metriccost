package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/illenko/whodidthis/models"
)

type AnalysisRepository struct {
	db *DB
}

func NewAnalysisRepository(db *DB) *AnalysisRepository {
	return &AnalysisRepository{db: db}
}

func (r *AnalysisRepository) Create(ctx context.Context, currentID, previousID int64) (*models.SnapshotAnalysis, error) {
	now := time.Now()
	query := `
		INSERT INTO snapshot_analyses (current_snapshot_id, previous_snapshot_id, status, created_at)
		VALUES (?, ?, ?, ?)
	`
	result, err := r.db.conn.ExecContext(ctx, query,
		currentID,
		previousID,
		models.AnalysisStatusPending,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &models.SnapshotAnalysis{
		ID:                 id,
		CurrentSnapshotID:  currentID,
		PreviousSnapshotID: previousID,
		Status:             models.AnalysisStatusPending,
		CreatedAt:          now,
	}, nil
}

func (r *AnalysisRepository) GetByPair(ctx context.Context, currentID, previousID int64) (*models.SnapshotAnalysis, error) {
	query := `
		SELECT id, current_snapshot_id, previous_snapshot_id, status, result, tool_calls, error, created_at, completed_at
		FROM snapshot_analyses
		WHERE current_snapshot_id = ? AND previous_snapshot_id = ?
	`
	return r.scanOne(r.db.conn.QueryRowContext(ctx, query, currentID, previousID))
}

func (r *AnalysisRepository) GetByID(ctx context.Context, id int64) (*models.SnapshotAnalysis, error) {
	query := `
		SELECT id, current_snapshot_id, previous_snapshot_id, status, result, tool_calls, error, created_at, completed_at
		FROM snapshot_analyses
		WHERE id = ?
	`
	return r.scanOne(r.db.conn.QueryRowContext(ctx, query, id))
}

func (r *AnalysisRepository) ListBySnapshot(ctx context.Context, snapshotID int64) ([]models.SnapshotAnalysis, error) {
	query := `
		SELECT id, current_snapshot_id, previous_snapshot_id, status, result, tool_calls, error, created_at, completed_at
		FROM snapshot_analyses
		WHERE current_snapshot_id = ? OR previous_snapshot_id = ?
		ORDER BY created_at DESC
	`
	rows, err := r.db.conn.QueryContext(ctx, query, snapshotID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var analyses []models.SnapshotAnalysis
	for rows.Next() {
		a, err := r.scanFromRows(rows)
		if err != nil {
			return nil, err
		}
		analyses = append(analyses, *a)
	}
	return analyses, rows.Err()
}

func (r *AnalysisRepository) Update(ctx context.Context, analysis *models.SnapshotAnalysis) error {
	toolCallsJSON, err := json.Marshal(analysis.ToolCalls)
	if err != nil {
		return err
	}

	var completedAt *string
	if analysis.CompletedAt != nil {
		t := analysis.CompletedAt.Format(time.RFC3339)
		completedAt = &t
	}

	query := `
		UPDATE snapshot_analyses
		SET status = ?, result = ?, tool_calls = ?, error = ?, completed_at = ?
		WHERE id = ?
	`
	_, err = r.db.conn.ExecContext(ctx, query,
		analysis.Status,
		analysis.Result,
		string(toolCallsJSON),
		analysis.Error,
		completedAt,
		analysis.ID,
	)
	return err
}

func (r *AnalysisRepository) Delete(ctx context.Context, currentID, previousID int64) error {
	query := `DELETE FROM snapshot_analyses WHERE current_snapshot_id = ? AND previous_snapshot_id = ?`
	_, err := r.db.conn.ExecContext(ctx, query, currentID, previousID)
	return err
}

func (r *AnalysisRepository) scanOne(row *sql.Row) (*models.SnapshotAnalysis, error) {
	var a models.SnapshotAnalysis
	var createdAt string
	var completedAt sql.NullString
	var result sql.NullString
	var toolCalls sql.NullString
	var errStr sql.NullString

	err := row.Scan(
		&a.ID,
		&a.CurrentSnapshotID,
		&a.PreviousSnapshotID,
		&a.Status,
		&result,
		&toolCalls,
		&errStr,
		&createdAt,
		&completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	a.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		t, err := time.Parse(time.RFC3339, completedAt.String)
		if err != nil {
			return nil, err
		}
		a.CompletedAt = &t
	}

	if result.Valid {
		a.Result = result.String
	}

	if errStr.Valid {
		a.Error = errStr.String
	}

	if toolCalls.Valid && toolCalls.String != "" {
		if err := json.Unmarshal([]byte(toolCalls.String), &a.ToolCalls); err != nil {
			return nil, err
		}
	}

	return &a, nil
}

func (r *AnalysisRepository) scanFromRows(rows *sql.Rows) (*models.SnapshotAnalysis, error) {
	var a models.SnapshotAnalysis
	var createdAt string
	var completedAt sql.NullString
	var result sql.NullString
	var toolCalls sql.NullString
	var errStr sql.NullString

	err := rows.Scan(
		&a.ID,
		&a.CurrentSnapshotID,
		&a.PreviousSnapshotID,
		&a.Status,
		&result,
		&toolCalls,
		&errStr,
		&createdAt,
		&completedAt,
	)
	if err != nil {
		return nil, err
	}

	a.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		t, err := time.Parse(time.RFC3339, completedAt.String)
		if err != nil {
			return nil, err
		}
		a.CompletedAt = &t
	}

	if result.Valid {
		a.Result = result.String
	}

	if errStr.Valid {
		a.Error = errStr.String
	}

	if toolCalls.Valid && toolCalls.String != "" {
		if err := json.Unmarshal([]byte(toolCalls.String), &a.ToolCalls); err != nil {
			return nil, err
		}
	}

	return &a, nil
}
