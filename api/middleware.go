package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

const requestTimeout = 30 * time.Second

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "error", err)
				sw.Header().Set("Content-Type", "application/json")
				sw.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(sw).Encode(map[string]string{"error": "internal server error"})
			}
		}()

		sw.Header().Set("Access-Control-Allow-Origin", "*")
		sw.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		sw.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			sw.WriteHeader(http.StatusOK)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()

		next.ServeHTTP(sw, r.WithContext(ctx))

		slog.Debug("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
	})
}
