package storage

import (
	"context"
	"time"

	"github.com/illenko/whodidthis/models"
)

type SnapshotsRepo interface {
	Create(ctx context.Context, s *models.Snapshot) (int64, error)
	Update(ctx context.Context, s *models.Snapshot) error
	GetLatest(ctx context.Context) (*models.Snapshot, error)
	GetByID(ctx context.Context, id int64) (*models.Snapshot, error)
	List(ctx context.Context, limit int) ([]models.Snapshot, error)
	GetByDate(ctx context.Context, date time.Time) (*models.Snapshot, error)
	GetNDaysAgo(ctx context.Context, days int) (*models.Snapshot, error)
	DeleteOlderThan(ctx context.Context, days int) (int64, error)
}

type ServicesRepo interface {
	Create(ctx context.Context, s *models.ServiceSnapshot) (int64, error)
	CreateBatch(ctx context.Context, services []*models.ServiceSnapshot) error
	List(ctx context.Context, snapshotID int64, opts ServiceListOptions) ([]models.ServiceSnapshot, error)
	GetByName(ctx context.Context, snapshotID int64, name string) (*models.ServiceSnapshot, error)
}

type MetricsRepo interface {
	Create(ctx context.Context, m *models.MetricSnapshot) (int64, error)
	CreateBatch(ctx context.Context, metrics []*models.MetricSnapshot) error
	List(ctx context.Context, serviceSnapshotID int64, opts MetricListOptions) ([]models.MetricSnapshot, error)
	GetByName(ctx context.Context, serviceSnapshotID int64, name string) (*models.MetricSnapshot, error)
}

type LabelsRepo interface {
	Create(ctx context.Context, l *models.LabelSnapshot) (int64, error)
	CreateBatch(ctx context.Context, labels []*models.LabelSnapshot) error
	List(ctx context.Context, metricSnapshotID int64) ([]models.LabelSnapshot, error)
	GetByName(ctx context.Context, metricSnapshotID int64, name string) (*models.LabelSnapshot, error)
}

type AnalysisRepo interface {
	Create(ctx context.Context, currentID, previousID int64) (*models.SnapshotAnalysis, error)
	GetByPair(ctx context.Context, currentID, previousID int64) (*models.SnapshotAnalysis, error)
	GetByID(ctx context.Context, id int64) (*models.SnapshotAnalysis, error)
	ListBySnapshot(ctx context.Context, snapshotID int64) ([]models.SnapshotAnalysis, error)
	Update(ctx context.Context, analysis *models.SnapshotAnalysis) error
	Delete(ctx context.Context, currentID, previousID int64) error
}
