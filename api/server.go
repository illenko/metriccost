package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type Server struct {
	httpServer *http.Server
	handlers   *Handlers
}

type ServerConfig struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func NewServer(handlers *Handlers, cfg ServerConfig) *Server {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}

	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /health", handlers.Health)

	// Overview
	mux.HandleFunc("GET /api/overview", handlers.GetOverview)

	// Metrics
	mux.HandleFunc("GET /api/metrics", handlers.ListMetrics)
	mux.HandleFunc("GET /api/metrics/{name}", handlers.GetMetric)

	// Recommendations
	mux.HandleFunc("GET /api/recommendations", handlers.ListRecommendations)

	// Trends
	mux.HandleFunc("GET /api/trends", handlers.GetTrends)

	// Dashboards
	mux.HandleFunc("GET /api/dashboards/unused", handlers.GetUnusedDashboards)

	// Scan
	mux.HandleFunc("POST /api/scan", handlers.TriggerScan)
	mux.HandleFunc("GET /api/scan/status", handlers.GetScanStatus)

	handler := withMiddleware(mux)

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Host + ":" + strconv.Itoa(cfg.Port),
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
		handlers: handlers,
	}
}

func (s *Server) Start() error {
	slog.Info("starting HTTP server", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Recovery
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "error", err)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()

		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Logging
		next.ServeHTTP(w, r)

		slog.Debug("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
