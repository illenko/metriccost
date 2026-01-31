package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/illenko/metriccost/models"
)

type DashboardsRepository struct {
	db *DB
}

func NewDashboardsRepository(db *DB) *DashboardsRepository {
	return &DashboardsRepository{db: db}
}

func (r *DashboardsRepository) Save(ctx context.Context, d *models.DashboardStats) error {
	metricsJSON, err := json.Marshal(d.MetricsUsed)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	query := `
		INSERT INTO dashboard_stats (collected_at, dashboard_uid, dashboard_name, folder_name, last_viewed_at, query_count, metrics_used)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(dashboard_uid, collected_at) DO UPDATE SET
			dashboard_name = excluded.dashboard_name,
			folder_name = excluded.folder_name,
			last_viewed_at = excluded.last_viewed_at,
			query_count = excluded.query_count,
			metrics_used = excluded.metrics_used
	`

	var lastViewed *string
	if !d.LastViewedAt.IsZero() {
		s := d.LastViewedAt.Format(time.RFC3339)
		lastViewed = &s
	}
	_, err = r.db.conn.ExecContext(ctx, query,
		d.CollectedAt.Format(time.RFC3339), d.DashboardUID, d.DashboardName, d.FolderName,
		lastViewed, d.QueryCount, string(metricsJSON),
	)
	return err
}

func (r *DashboardsRepository) SaveBatch(ctx context.Context, dashboards []*models.DashboardStats) error {
	tx, err := r.db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO dashboard_stats (collected_at, dashboard_uid, dashboard_name, folder_name, last_viewed_at, query_count, metrics_used)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(dashboard_uid, collected_at) DO UPDATE SET
			dashboard_name = excluded.dashboard_name,
			folder_name = excluded.folder_name,
			last_viewed_at = excluded.last_viewed_at,
			query_count = excluded.query_count,
			metrics_used = excluded.metrics_used
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range dashboards {
		metricsJSON, _ := json.Marshal(d.MetricsUsed)
		var lastViewed *string
		if !d.LastViewedAt.IsZero() {
			s := d.LastViewedAt.Format(time.RFC3339)
			lastViewed = &s
		}
		_, err = stmt.ExecContext(ctx,
			d.CollectedAt.Format(time.RFC3339), d.DashboardUID, d.DashboardName, d.FolderName,
			lastViewed, d.QueryCount, string(metricsJSON),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *DashboardsRepository) GetLatestCollectionTime(ctx context.Context) (time.Time, error) {
	var s sql.NullString
	err := r.db.conn.QueryRowContext(ctx,
		"SELECT MAX(collected_at) FROM dashboard_stats",
	).Scan(&s)
	if err != nil {
		return time.Time{}, err
	}
	if !s.Valid || s.String == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s.String)
}

func (r *DashboardsRepository) GetAllMetricsUsed(ctx context.Context) (map[string]struct{}, error) {
	latestTime, err := r.GetLatestCollectionTime(ctx)
	if err != nil {
		return nil, err
	}
	if latestTime.IsZero() {
		return make(map[string]struct{}), nil
	}

	rows, err := r.db.conn.QueryContext(ctx,
		"SELECT metrics_used FROM dashboard_stats WHERE collected_at = ?",
		latestTime,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	metricsUsed := make(map[string]struct{})
	for rows.Next() {
		var metricsJSON sql.NullString
		if err := rows.Scan(&metricsJSON); err != nil {
			return nil, err
		}
		if metricsJSON.Valid && metricsJSON.String != "" {
			var metrics []string
			if err := json.Unmarshal([]byte(metricsJSON.String), &metrics); err != nil {
				continue
			}
			for _, m := range metrics {
				metricsUsed[m] = struct{}{}
			}
		}
	}

	return metricsUsed, rows.Err()
}

func (r *DashboardsRepository) GetUnusedDashboards(ctx context.Context, daysSinceView int) ([]*models.UnusedDashboard, error) {
	latestTime, err := r.GetLatestCollectionTime(ctx)
	if err != nil {
		return nil, err
	}
	if latestTime.IsZero() {
		return nil, nil
	}

	cutoff := time.Now().AddDate(0, 0, -daysSinceView)

	query := `
		SELECT dashboard_uid, dashboard_name, folder_name, last_viewed_at, query_count, metrics_used
		FROM dashboard_stats
		WHERE collected_at = ? AND (last_viewed_at < ? OR last_viewed_at IS NULL)
		ORDER BY last_viewed_at ASC
	`

	rows, err := r.db.conn.QueryContext(ctx, query, latestTime, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dashboards []*models.UnusedDashboard
	now := time.Now()

	for rows.Next() {
		var d models.UnusedDashboard
		var lastViewed sql.NullString
		var queryCount int
		var metricsJSON sql.NullString

		err := rows.Scan(&d.UID, &d.Name, &d.FolderName, &lastViewed, &queryCount, &metricsJSON)
		if err != nil {
			return nil, err
		}

		if lastViewed.Valid && lastViewed.String != "" {
			d.LastViewed, _ = time.Parse(time.RFC3339, lastViewed.String)
			d.DaysSinceView = int(now.Sub(d.LastViewed).Hours() / 24)
		} else {
			d.DaysSinceView = 9999
		}

		if metricsJSON.Valid && metricsJSON.String != "" {
			var metrics []string
			if err := json.Unmarshal([]byte(metricsJSON.String), &metrics); err == nil {
				d.MetricsCount = len(metrics)
			}
		}

		dashboards = append(dashboards, &d)
	}

	return dashboards, rows.Err()
}

func (r *DashboardsRepository) List(ctx context.Context) ([]*models.DashboardStats, error) {
	latestTime, err := r.GetLatestCollectionTime(ctx)
	if err != nil {
		return nil, err
	}
	if latestTime.IsZero() {
		return nil, nil
	}

	query := `
		SELECT id, collected_at, dashboard_uid, dashboard_name, folder_name, last_viewed_at, query_count, metrics_used
		FROM dashboard_stats
		WHERE collected_at = ?
		ORDER BY dashboard_name
	`

	rows, err := r.db.conn.QueryContext(ctx, query, latestTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dashboards []*models.DashboardStats
	for rows.Next() {
		var d models.DashboardStats
		var collectedAt string
		var lastViewed sql.NullString
		var metricsJSON sql.NullString
		var folderName sql.NullString

		err := rows.Scan(&d.ID, &collectedAt, &d.DashboardUID, &d.DashboardName, &folderName, &lastViewed, &d.QueryCount, &metricsJSON)
		if err != nil {
			return nil, err
		}

		d.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
		if folderName.Valid {
			d.FolderName = folderName.String
		}
		if lastViewed.Valid && lastViewed.String != "" {
			d.LastViewedAt, _ = time.Parse(time.RFC3339, lastViewed.String)
		}
		if metricsJSON.Valid && metricsJSON.String != "" {
			json.Unmarshal([]byte(metricsJSON.String), &d.MetricsUsed)
		}

		dashboards = append(dashboards, &d)
	}

	return dashboards, rows.Err()
}
