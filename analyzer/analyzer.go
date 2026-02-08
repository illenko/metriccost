package analyzer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/illenko/whodidthis/config"
	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/storage"
	"google.golang.org/genai"
)

const maxAgenticIterations = 20
const defaultGeminiModel = "gemini-2.5-pro"

type Analyzer struct {
	client       *genai.Client
	model        string
	geminiConfig config.GeminiConfig
	toolExecutor *ToolExecutor
	analysisRepo storage.AnalysisRepo
	snapshots    storage.SnapshotsRepo
	services     storage.ServicesRepo

	mu                 sync.RWMutex
	running            bool
	currentSnapshotID  int64
	previousSnapshotID int64
	progress           string
	logger             *slog.Logger
}

type Config struct {
	Gemini       config.GeminiConfig
	ToolExecutor *ToolExecutor
	AnalysisRepo storage.AnalysisRepo
	Snapshots    storage.SnapshotsRepo
	Services     storage.ServicesRepo
}

func New(ctx context.Context, cfg Config) (*Analyzer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.Gemini.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	model := cfg.Gemini.Model
	if model == "" {
		model = defaultGeminiModel
	}

	return &Analyzer{
		client:       client,
		model:        model,
		geminiConfig: cfg.Gemini,
		toolExecutor: cfg.ToolExecutor,
		analysisRepo: cfg.AnalysisRepo,
		snapshots:    cfg.Snapshots,
		services:     cfg.Services,
		logger:       slog.Default().With("component", "analyzer"),
	}, nil
}

func (a *Analyzer) StartAnalysis(ctx context.Context, currentID, previousID int64) (*models.SnapshotAnalysis, error) {
	currentSnapshot, err := a.snapshots.GetByID(ctx, currentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current snapshot: %w", err)
	}
	if currentSnapshot == nil {
		return nil, fmt.Errorf("current snapshot %d not found", currentID)
	}

	previousSnapshot, err := a.snapshots.GetByID(ctx, previousID)
	if err != nil {
		return nil, fmt.Errorf("failed to get previous snapshot: %w", err)
	}
	if previousSnapshot == nil {
		return nil, fmt.Errorf("previous snapshot %d not found", previousID)
	}

	existing, err := a.analysisRepo.GetByPair(ctx, currentID, previousID)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing analysis: %w", err)
	}
	if existing != nil && existing.Status == models.AnalysisStatusCompleted {
		a.logger.Info("returning existing completed analysis", "analysis_id", existing.ID)
		return existing, nil
	}

	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return nil, fmt.Errorf("another analysis is already running (snapshots %d vs %d)", a.currentSnapshotID, a.previousSnapshotID)
	}
	a.running = true
	a.currentSnapshotID = currentID
	a.previousSnapshotID = previousID
	a.progress = "Initializing"
	a.mu.Unlock()

	analysis, err := a.analysisRepo.Create(ctx, currentID, previousID)
	if err != nil {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		return nil, fmt.Errorf("failed to create analysis record: %w", err)
	}

	go a.runAnalysis(analysis, currentSnapshot, previousSnapshot)

	analysis.Status = models.AnalysisStatusRunning
	return analysis, nil
}

func (a *Analyzer) GetAnalysis(ctx context.Context, currentID, previousID int64) (*models.SnapshotAnalysis, error) {
	return a.analysisRepo.GetByPair(ctx, currentID, previousID)
}

func (a *Analyzer) ListAnalyses(ctx context.Context, snapshotID int64) ([]models.SnapshotAnalysis, error) {
	return a.analysisRepo.ListBySnapshot(ctx, snapshotID)
}

func (a *Analyzer) DeleteAnalysis(ctx context.Context, currentID, previousID int64) error {
	return a.analysisRepo.Delete(ctx, currentID, previousID)
}

func (a *Analyzer) GetGlobalStatus() models.AnalysisGlobalStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return models.AnalysisGlobalStatus{
		Running:            a.running,
		CurrentSnapshotID:  a.currentSnapshotID,
		PreviousSnapshotID: a.previousSnapshotID,
		Progress:           a.progress,
	}
}

func (a *Analyzer) completeAnalysisWithError(ctx context.Context, analysis *models.SnapshotAnalysis, err error) {
	now := time.Now()
	analysis.Status = models.AnalysisStatusFailed
	analysis.Error = err.Error()
	analysis.CompletedAt = &now

	if updateErr := a.analysisRepo.Update(ctx, analysis); updateErr != nil {
		a.logger.Error("failed to update analysis with error", "error", updateErr)
	}
}

func (a *Analyzer) updateProgress(progress string) {
	a.mu.Lock()
	a.progress = progress
	a.mu.Unlock()
}
