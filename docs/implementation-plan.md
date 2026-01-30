# Observability Cost Optimizer - Implementation Plan

## Document Overview

This document provides a detailed implementation plan for the metriccost MVP based on the requirements defined in `concept.md`.

> **Note:** This implementation focuses on **counts and storage size** (cardinality, MB/GB, sample counts) rather than USD cost estimates. Actual costs vary widely due to CUDs, discounts, and pricing models - users can apply their own cost calculations externally if needed.

---

## Phase 1: Project Setup & Foundation

### 1.1 Go Project Initialization

**Tasks:**
- [ ] Initialize Go module: `go mod init github.com/[org]/metriccost`
- [ ] Set up project directory structure:
  ```
  metriccost/
  ├── cmd/
  │   └── metriccost/
  │       └── main.go           # CLI entry point
  ├── internal/
  │   ├── config/               # Configuration parsing
  │   ├── prometheus/           # Prometheus API client
  │   ├── grafana/              # Grafana API client
  │   ├── storage/              # SQLite operations
  │   ├── collector/            # Data collection orchestration
  │   ├── analyzer/             # Size calculation & recommendations
  │   ├── api/                  # REST API handlers
  │   └── scheduler/            # Background job scheduler
  ├── pkg/
  │   └── models/               # Shared data models
  ├── web/                      # React frontend (added later)
  ├── configs/
  │   └── config.example.yaml   # Example configuration
  ├── migrations/               # SQLite schema migrations
  ├── docs/                     # Documentation
  ├── go.mod
  ├── go.sum
  └── Makefile
  ```
- [ ] Create `Makefile` with common tasks (build, test, run, lint)
- [ ] Add `.gitignore` for Go and Node.js artifacts

**Dependencies to add:**
```go
// go.mod dependencies
github.com/spf13/cobra          // CLI framework
github.com/spf13/viper          // Configuration
github.com/prometheus/client_golang/api  // Prometheus client
modernc.org/sqlite              // Pure Go SQLite (no CGO)
github.com/go-chi/chi/v5        // HTTP router
github.com/rs/zerolog           // Structured logging
gopkg.in/yaml.v3                // YAML parsing
```

**Deliverable:** Compilable Go project with basic structure

---

### 1.2 Configuration System

**File:** `internal/config/config.go`

**Tasks:**
- [ ] Define Config struct matching YAML schema from concept.md
- [ ] Implement YAML file loading with Viper
- [ ] Add environment variable overrides (METRICCOST_*)
- [ ] Implement config validation with sensible defaults
- [ ] Create example config file

**Config struct outline:**
```go
type Config struct {
    Prometheus      PrometheusConfig
    Grafana         GrafanaConfig
    Collection      CollectionConfig
    SizeModel       SizeModelConfig
    Teams           map[string]TeamConfig
    Recommendations RecommendationsConfig
    Server          ServerConfig
}
```

**Deliverable:** Working configuration loading with defaults

---

### 1.3 SQLite Database Layer

**File:** `internal/storage/sqlite.go`

**Tasks:**
- [ ] Implement database connection management
- [ ] Create migration system (embed SQL files)
- [ ] Implement all tables from concept.md:
  - `metric_snapshots`
  - `recommendations`
  - `dashboard_stats`
  - `snapshots` (overall cardinality/size history)
- [ ] Create repository interfaces for each entity
- [ ] Implement data retention cleanup (90-day default)
- [ ] Add database stats command support

**Migration files:**
```
migrations/
├── 001_initial_schema.sql
└── 002_indexes.sql
```

**Deliverable:** Working SQLite layer with all tables

---

## Phase 2: Prometheus Integration

### 2.1 Prometheus API Client

**File:** `internal/prometheus/client.go`

**Tasks:**
- [ ] Implement Prometheus HTTP client with configurable timeout
- [ ] Add authentication support (basic auth)
- [ ] Implement connection health check
- [ ] Add retry logic with exponential backoff (3 retries)

**API methods needed:**
```go
type Client interface {
    HealthCheck(ctx context.Context) error
    GetAllMetricNames(ctx context.Context) ([]string, error)
    GetMetricCardinality(ctx context.Context, metricName string) (int, error)
    GetMetricLabels(ctx context.Context, metricName string) ([]LabelInfo, error)
    GetConfig(ctx context.Context) (*PrometheusConfig, error)  // for scrape_interval
}
```

**Deliverable:** Working Prometheus client with health check

---

### 2.2 Metrics Collection Service

**File:** `internal/collector/prometheus_collector.go`

**Tasks:**
- [ ] Implement full metrics scan:
  1. Get all metric names
  2. For each metric, get cardinality
  3. Calculate estimated storage size
  4. Apply team attribution via regex patterns
- [ ] Add progress logging (processing X of Y metrics)
- [ ] Implement batching to avoid overwhelming Prometheus
- [ ] Store results in `metric_snapshots` table
- [ ] Calculate and store snapshot in `snapshots` table

**Performance considerations:**
- Batch metric queries (50 metrics per batch)
- Add configurable concurrency (default: 5 parallel requests)
- Timeout per metric query: 30 seconds

**Deliverable:** Can scan Prometheus and populate database

---

### 2.3 Team Attribution

**File:** `internal/analyzer/team_matcher.go`

**Tasks:**
- [ ] Implement regex-based team matching
- [ ] Support multiple patterns per team
- [ ] Handle "unassigned" metrics (no team match)
- [ ] Cache compiled regex patterns

**Example:**
```go
func (m *TeamMatcher) GetTeam(metricName string) string {
    for team, patterns := range m.patterns {
        for _, pattern := range patterns {
            if pattern.MatchString(metricName) {
                return team
            }
        }
    }
    return "unassigned"
}
```

**Deliverable:** Working team attribution

---

## Phase 3: Analysis Engine

### 3.1 Size Calculator

**File:** `internal/analyzer/size_calculator.go`

**Tasks:**
- [ ] Implement storage size estimation:
  ```
  estimated_size = cardinality × samples_per_day × retention_days × bytes_per_sample
  ```
- [ ] Support configurable parameters:
  - `bytes_per_sample`: default 2 bytes (Prometheus TSDB average)
  - `retention_days`: default 30
  - `scrape_interval`: default 15s → 5760 samples/day
- [ ] Calculate per-metric storage size (MB/GB)
- [ ] Calculate team breakdown totals (by cardinality and size)
- [ ] Calculate trend (% change from previous scan)

**Deliverable:** Accurate size estimations

---

### 3.2 Recommendations Engine

**File:** `internal/analyzer/recommendations.go`

**Tasks:**
- [ ] Implement detection algorithms:
  1. **High cardinality**: cardinality > 10,000 threshold
  2. **Unused metrics**: not in any Grafana dashboard queries
  3. **High-cardinality labels**: labels with >100 unique values
- [ ] Implement priority scoring:
  - HIGH: cardinality >10K AND low usage
  - HIGH: metric not used at all
  - MEDIUM: potential for aggregation
  - LOW: optimization suggestions
- [ ] Calculate potential size reduction per recommendation
- [ ] Generate actionable descriptions and suggested actions
- [ ] Filter recommendations by min_size_impact threshold (e.g., >100MB)

**Recommendation types:**
```go
const (
    RecommendationHighCardinality  = "high_cardinality"
    RecommendationUnused           = "unused"
    RecommendationRetention        = "over_retention"
    RecommendationRedundantLabels  = "redundant_labels"
)
```

**Deliverable:** Working recommendations with priorities

---

### 3.3 Trends Calculator

**File:** `internal/analyzer/trends.go`

**Tasks:**
- [ ] Calculate daily/weekly cardinality and size trends
- [ ] Calculate per-metric trend (% change)
- [ ] Support configurable trend periods (7d, 30d, 90d)
- [ ] Handle missing data points gracefully

**Deliverable:** Historical trend data

---

## Phase 4: REST API

### 4.1 API Server Setup

**File:** `internal/api/server.go`

**Tasks:**
- [ ] Set up Chi router with middleware:
  - Request logging
  - Recovery (panic handler)
  - CORS (for development)
  - Request timeout
- [ ] Implement graceful shutdown
- [ ] Add structured JSON responses

**Deliverable:** HTTP server foundation

---

### 4.2 API Endpoints

**Files:** `internal/api/handlers/*.go`

**Implement all endpoints from concept.md:**

| Endpoint | Handler | Description |
|----------|---------|-------------|
| `GET /api/overview` | `overview.go` | Total metrics, cardinality, size, team breakdown |
| `GET /api/metrics` | `metrics.go` | List metrics with filtering/sorting |
| `GET /api/recommendations` | `recommendations.go` | List recommendations by priority |
| `GET /api/trends` | `trends.go` | Historical cardinality/size data points |
| `GET /api/dashboards/unused` | `dashboards.go` | Unused Grafana dashboards |
| `GET /health` | `health.go` | Service health check |

**Query parameters to support:**
- `/api/metrics`: `?sort=size|cardinality|name`, `?limit=20`, `?team=backend-core`, `?search=http_`
- `/api/recommendations`: `?priority=high|medium|low`
- `/api/trends`: `?period=7d|30d|90d`

**Deliverable:** All REST endpoints working

---

## Phase 5: CLI Interface

### 5.1 CLI Framework

**File:** `cmd/metriccost/main.go`

**Tasks:**
- [ ] Set up Cobra CLI framework
- [ ] Implement global flags: `--config`, `--verbose`
- [ ] Add version command

**Deliverable:** CLI skeleton

---

### 5.2 CLI Commands

**Implement commands from concept.md:**

| Command | File | Description |
|---------|------|-------------|
| `metriccost init` | `cmd/init.go` | Create example config |
| `metriccost scan` | `cmd/scan.go` | One-time data collection |
| `metriccost report` | `cmd/report.go` | Print report to console |
| `metriccost metric <name>` | `cmd/metric.go` | Show specific metric details |
| `metriccost serve` | `cmd/serve.go` | Start web server |
| `metriccost export` | `cmd/export.go` | Export data to CSV/JSON |
| `metriccost db cleanup` | `cmd/db.go` | Database maintenance |
| `metriccost db stats` | `cmd/db.go` | Show database statistics |

**Report command flags:**
- `--format=table|json`
- `--top=20`
- `--sort=size|cardinality`

**Deliverable:** Full CLI functionality

---

## Phase 6: Grafana Integration

### 6.1 Grafana API Client

**File:** `internal/grafana/client.go`

**Tasks:**
- [ ] Implement Grafana HTTP client
- [ ] Add API token authentication
- [ ] Add basic auth support (optional)
- [ ] Implement connection health check

**API methods needed:**
```go
type Client interface {
    HealthCheck(ctx context.Context) error
    ListDashboards(ctx context.Context) ([]Dashboard, error)
    GetDashboard(ctx context.Context, uid string) (*DashboardDetail, error)
}
```

**Deliverable:** Working Grafana client

---

### 6.2 Dashboard Analysis

**File:** `internal/collector/grafana_collector.go`

**Tasks:**
- [ ] Fetch all dashboards via `/api/search`
- [ ] For each dashboard:
  - Parse panel queries to extract metric names
  - Get `last_viewed_at` from dashboard metadata
  - Count queries per dashboard
- [ ] Identify unused dashboards (not viewed for >90 days)
- [ ] Store results in `dashboard_stats` table
- [ ] Cross-reference metrics used in dashboards with collected metrics

**Query parsing:**
- Extract metric names from PromQL queries
- Handle various query formats (raw PromQL, templated variables)

**Deliverable:** Dashboard usage tracking

---

## Phase 7: React Frontend

### 7.1 React Project Setup

**Directory:** `web/`

**Tasks:**
- [ ] Initialize Vite + React + TypeScript project
- [ ] Install dependencies:
  ```
  tailwindcss
  @tailwindcss/forms
  recharts
  react-router-dom
  @tanstack/react-query
  lucide-react (icons)
  ```
- [ ] Configure Tailwind CSS
- [ ] Set up API client with React Query
- [ ] Create base layout component (nav, sidebar)

**Project structure:**
```
web/
├── src/
│   ├── components/
│   │   ├── layout/
│   │   ├── charts/
│   │   └── ui/
│   ├── pages/
│   │   ├── Dashboard.tsx
│   │   ├── Metrics.tsx
│   │   ├── Recommendations.tsx
│   │   └── Dashboards.tsx
│   ├── api/
│   │   └── client.ts
│   ├── hooks/
│   ├── types/
│   └── App.tsx
├── index.html
├── vite.config.ts
├── tailwind.config.js
└── package.json
```

**Deliverable:** React project foundation

---

### 7.2 Dashboard Page (/)

**File:** `web/src/pages/Dashboard.tsx`

**Components to build:**
- [ ] `MetricCard` - Hero stat cards (total metrics, total cardinality, total size, trend %)
- [ ] `SizeTrendChart` - Line chart showing 30-day cardinality/size trend (Recharts)
- [ ] `TopMetricsTable` - Table of top 10 largest metrics (by cardinality or size)
- [ ] `QuickWinsList` - Top 5 recommendations list

**Layout:** Match wireframe from concept.md

**Deliverable:** Working dashboard page

---

### 7.3 Metrics Page (/metrics)

**File:** `web/src/pages/Metrics.tsx`

**Features to implement:**
- [ ] Searchable data table with columns:
  - Metric Name
  - Cardinality (time series count)
  - Estimated Size (MB/GB)
  - Team
  - Trend (% change)
- [ ] Team filter dropdown
- [ ] Column sorting (click header)
- [ ] Pagination (20 items per page)
- [ ] Search input (filter by metric name)

**Deliverable:** Working metrics table with filtering

---

### 7.4 Recommendations Page (/recommendations)

**File:** `web/src/pages/Recommendations.tsx`

**Features to implement:**
- [ ] Tab navigation: High / Medium / Low priority
- [ ] Recommendation cards showing:
  - Metric name (linked)
  - Type badge (high-cardinality, unused, etc.)
  - Current cardinality / size
  - Potential size reduction (highlighted)
  - Description text
  - Suggested action (code block if applicable)
- [ ] Empty state when no recommendations

**Deliverable:** Working recommendations page

---

### 7.5 Dashboards Page (/dashboards)

**File:** `web/src/pages/Dashboards.tsx`

**Features to implement:**
- [ ] Table of unused Grafana dashboards:
  - Dashboard name (external link to Grafana)
  - Last viewed date
  - Days since last view
  - Metrics count
- [ ] Sort by days since last view

**Deliverable:** Working dashboards page

---

### 7.6 Frontend Embedding

**File:** `internal/api/static.go`

**Tasks:**
- [ ] Build React app for production (`npm run build`)
- [ ] Embed `web/dist/` into Go binary using `//go:embed`
- [ ] Serve static files from embedded filesystem
- [ ] Handle SPA routing (serve index.html for all non-API routes)

**Example:**
```go
//go:embed web/dist
var webFS embed.FS

func ServeStatic(r chi.Router) {
    r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
        // Serve from embedded FS
    })
}
```

**Deliverable:** Single binary with embedded frontend

---

## Phase 8: Background Scheduler

### 8.1 Scheduler Implementation

**File:** `internal/scheduler/scheduler.go`

**Tasks:**
- [ ] Implement background job runner
- [ ] Support configurable collection interval (default: 24h)
- [ ] Implement graceful shutdown
- [ ] Add job status tracking
- [ ] Log collection progress and results

**Jobs to schedule:**
1. Prometheus metrics collection
2. Grafana dashboard collection
3. Recommendations generation
4. Old data cleanup (>90 days)

**Deliverable:** Working background scheduler

---

## Phase 9: Testing & Quality

### 9.1 Unit Tests

**Priority tests:**
- [ ] `internal/analyzer/size_calculator_test.go` - Size formula accuracy
- [ ] `internal/analyzer/team_matcher_test.go` - Regex matching
- [ ] `internal/analyzer/recommendations_test.go` - Priority scoring
- [ ] `internal/config/config_test.go` - Config parsing and defaults

**Target:** 70% coverage on core analysis logic

---

### 9.2 Integration Tests

**Optional for MVP:**
- [ ] Prometheus client with mock server
- [ ] SQLite repository operations
- [ ] API endpoint responses

---

## Phase 10: Packaging & Deployment

### 10.1 Build System

**Makefile targets:**
```makefile
build:          # Build binary for current platform
build-all:      # Cross-compile for linux/darwin/windows
build-frontend: # Build React app
test:           # Run all tests
lint:           # Run golangci-lint
docker:         # Build Docker image
release:        # Create release artifacts
```

**Deliverable:** Automated build pipeline

---

### 10.2 Docker Support

**File:** `Dockerfile`

**Tasks:**
- [ ] Multi-stage Dockerfile:
  1. Stage 1: Build frontend (Node.js)
  2. Stage 2: Build Go binary
  3. Stage 3: Final minimal image (scratch or alpine)
- [ ] Create `docker-compose.yaml` for local development
- [ ] Document Docker usage in README

**Deliverable:** Working Docker image

---

## Implementation Order Summary

```
Week 1-2: Phase 1-2
├── 1.1 Go project setup
├── 1.2 Configuration system
├── 1.3 SQLite database layer
├── 2.1 Prometheus API client
├── 2.2 Metrics collection
└── 2.3 Team attribution

Week 3-4: Phase 3-5
├── 3.1 Size calculator
├── 3.2 Recommendations engine
├── 3.3 Trends calculator
├── 4.1 API server setup
├── 4.2 API endpoints
├── 5.1 CLI framework
└── 5.2 CLI commands

Week 5-6: Phase 7
├── 7.1 React project setup
├── 7.2 Dashboard page
├── 7.3 Metrics page
├── 7.4 Recommendations page
├── 7.5 Dashboards page
└── 7.6 Frontend embedding

Week 7-8: Phase 6, 8-10
├── 6.1 Grafana API client
├── 6.2 Dashboard analysis
├── 8.1 Background scheduler
├── 9.1 Unit tests
├── 10.1 Build system
└── 10.2 Docker support
```

---

## Technical Decisions

### Why These Libraries?

| Library | Purpose | Rationale |
|---------|---------|-----------|
| `modernc.org/sqlite` | SQLite | Pure Go, no CGO required, easier cross-compilation |
| `chi` | HTTP router | Lightweight, idiomatic, good middleware support |
| `cobra` | CLI | Industry standard, excellent UX |
| `viper` | Config | Supports YAML, env vars, defaults |
| `zerolog` | Logging | Fast, structured, zero allocation |
| `Vite` | Frontend build | Fast HMR, excellent DX |
| `React Query` | Data fetching | Caching, refetching, loading states |
| `Recharts` | Charts | Simple API, React-native, responsive |

### Database Choice

SQLite with `modernc.org/sqlite` (pure Go) chosen over:
- `mattn/go-sqlite3`: Requires CGO, complicates cross-compilation
- PostgreSQL: Adds external dependency, overkill for MVP
- BoltDB: Less mature, no SQL support

### Frontend Architecture

React + TypeScript chosen for:
- Type safety
- Component reusability
- Excellent tooling
- Easy to embed in Go binary

---

## Risk Mitigation During Implementation

| Risk | Mitigation |
|------|------------|
| Prometheus API rate limiting | Implement request batching and configurable concurrency |
| Large metric counts (>100K) | Add pagination, progress reporting, memory-efficient processing |
| SQLite performance | Add proper indexes, connection pooling, periodic VACUUM |
| Frontend bundle size | Code splitting, lazy loading routes |
| Cross-compilation issues | Use pure Go libraries (no CGO) |

---

## Definition of Done

A task is complete when:
1. Code compiles without errors
2. Unit tests pass (where applicable)
3. Manual testing confirms functionality
4. Code follows Go idioms and project structure
5. Error handling is implemented

---

## Next Steps

1. Start with Phase 1.1 - Go project initialization
2. Create the directory structure
3. Add initial dependencies
4. Begin implementing configuration system

Run `metriccost init` should be the first working command.