# Service-Level Metrics Audit Tool - Implementation Plan v2

## Overview

This plan implements the **pivot** from a metric-centric audit tool to a **service-centric audit tool** as defined in `concept.md`. The new architecture discovers services via Prometheus labels and provides drill-down visibility: **Services → Metrics → Labels**.

### MVP Simplifications

> **Decision:** Skip trends/deltas for MVP. Only show current snapshot data.
> Trends require 2+ scans and add significant complexity. Can be added later.

| Aspect | Before (v1) | After (v2 MVP) |
|--------|-------------|----------------|
| Top-level grouping | Metrics globally | Services discovered via label |
| Team attribution | Regex patterns | Service label (e.g., `app`, `job`) |
| Hierarchy | Metric → Labels | Service → Metrics → Labels → Samples |
| Trends/Deltas | N/A | **Skipped for MVP** |

---

## Progress Summary

| Phase | Status | Description |
|-------|--------|-------------|
| 1.1 | ✅ Done | Database migration |
| 1.2 | ✅ Done | Storage layer (repos) |
| 2.1 | ✅ Done | Data models |
| 3.1 | ✅ Done | Configuration update |
| 4.1 | ✅ Done | Prometheus client |
| 5.1 | ✅ Done | Collector rewrite |
| 5.2 | ✅ Done | Remove unused code |
| 6.1 | ⬜ Todo | API endpoints (snapshot-centric) |
| 6.2 | ⬜ Todo | Update router |
| 7.1 | ⬜ Todo | Frontend types |
| 7.2 | ⬜ Todo | Scans list page (home) |
| 7.3 | ⬜ Todo | Services list page |
| 7.4 | ⬜ Todo | Metrics list page |
| 7.5 | ⬜ Todo | Labels list page |
| 8.1 | ⬜ Todo | Final cleanup |

---

## Phase 1: Database Schema ✅

### 1.1 Database Migration ✅

**File:** `storage/migrations/001_initial_schema.sql` (rewritten)

- [x] Snapshots table (one per scan)
- [x] Service snapshots table (FK → snapshots)
- [x] Metric snapshots table (FK → service_snapshots)
- [x] Label snapshots table (FK → metric_snapshots)
- [x] All indexes

### 1.2 Storage Layer ✅

- [x] `storage/sqlite.go` - Updated Stats/Cleanup
- [x] `storage/snapshots_repo.go` - Rewritten
- [x] `storage/services_repo.go` - Created
- [x] `storage/metrics_repo.go` - Rewritten
- [x] `storage/labels_repo.go` - Created
- [x] Removed: `recommendations_repo.go`, `dashboards_repo.go`

---

## Phase 2: Models ✅

### 2.1 Data Models ✅

**File:** `models/models.go` (rewritten)

```go
// Current models (no deltas/trends for MVP):
- Snapshot           // scan metadata
- ServiceSnapshot    // service in a scan
- MetricSnapshot     // metric in a service
- LabelSnapshot      // label in a metric (with sample values)
- Overview           // API response
- ScanStatus         // scan state
- HealthStatus       // health check
```

---

## Phase 3: Configuration ✅

### 3.1 Simplify Configuration ✅

**File:** `config/config.go`

- [x] Removed: `Teams`, `Grafana`, `Recommendations` configs
- [x] Added: `Discovery.ServiceLabel`
- [x] Added: `Scan.SampleValuesLimit`
- [x] Added: `Storage` config
- [x] Updated `config.yaml`

---

## Phase 4: Prometheus Client ✅

### 4.1 Add Service Discovery Queries ✅

**File:** `prometheus/client.go`

- [x] `DiscoverServices(serviceLabel)` → list services with series count
- [x] `GetMetricsForService(serviceLabel, serviceName)` → metrics with series count
- [x] `GetLabelsForMetric(...)` → labels with unique values + sample values

---

## Phase 5: Collector

### 5.1 Service-Centric Collection

**File:** `collector/prometheus_collector.go` (rewrite)

**Collection flow:**
1. Discover services via label
2. For each service: get metrics with counts
3. For each metric: get labels with counts + samples
4. Store full hierarchy in one transaction

**Tasks:**
- [ ] Implement new collection flow
- [ ] Add progress callback for status API
- [ ] Rate limiting (configurable concurrency)

### 5.2 Remove Unused Code

- [ ] Remove `collector/grafana_collector.go`
- [ ] Remove `analyzer/` directory
- [ ] Remove `grafana/` directory

---

## Phase 6: API

### 6.1 New Endpoints (Snapshot-Centric)

**File:** `api/handlers.go`

All data is accessed through a specific snapshot, enabling historical comparison.

```
GET  /health                                              → health check
POST /api/scan                                            → trigger scan
GET  /api/scan/status                                     → scan status

GET  /api/scans                                           → list all snapshots
GET  /api/scans/latest                                    → redirect to latest scan
GET  /api/scans/{id}                                      → snapshot details
GET  /api/scans/{id}/services                             → services in snapshot
GET  /api/scans/{id}/services/{service}                   → service detail
GET  /api/scans/{id}/services/{service}/metrics           → metrics in service
GET  /api/scans/{id}/services/{service}/metrics/{metric}  → metric detail
GET  /api/scans/{id}/services/{service}/metrics/{metric}/labels → labels
```

**Default behavior:** `/api/scans/latest` returns latest scan info (or 404 if no scans).
Frontend can use this to auto-navigate to latest scan on load.

**Response examples:**
```json
// GET /api/scans
[
  {"id": 3, "collected_at": "2024-01-15T02:00:00Z", "total_services": 127, "total_series": 2400000},
  {"id": 2, "collected_at": "2024-01-14T02:00:00Z", "total_services": 125, "total_series": 2350000},
  {"id": 1, "collected_at": "2024-01-13T02:00:00Z", "total_services": 124, "total_series": 2300000}
]

// GET /api/scans/3/services
[{"name": "payment-gateway", "total_series": 245231, "metric_count": 89}]

// GET /api/scans/3/services/payment-gateway/metrics
[{"name": "http_requests_total", "series_count": 38992, "label_count": 6}]

// GET /api/scans/3/services/payment-gateway/metrics/http_requests_total/labels
[{"name": "endpoint", "unique_values": 1234, "sample_values": ["/api/v1/pay", ...]}]
```

**Use case:** After fixing high-cardinality issue, run new scan and compare with previous.

### 6.2 Update Router

**File:** `api/server.go`

- [ ] Register new hierarchical routes
- [ ] Remove old endpoints

---

## Phase 7: Frontend

### 7.1 TypeScript Types

**File:** `web/src/api.ts`

```typescript
interface Scan {
  id: number;
  collected_at: string;
  total_services: number;
  total_series: number;
  duration_ms?: number;
}

interface Service {
  name: string;
  total_series: number;
  metric_count: number;
}

interface Metric {
  name: string;
  series_count: number;
  label_count: number;
}

interface Label {
  name: string;
  unique_values: number;
  sample_values: string[];
}
```

### 7.2 Scans List Page (Home)

```
┌─────────────────────────────────────────────────────────┐
│ Metrics Audit                         [Run Scan] [Status]
├─────────────────────────────────────────────────────────┤
│ Scan History                                            │
├─────────────────────────────────────────────────────────┤
│ Date                 │ Services │ Series    │ Duration  │
│ 2024-01-15 02:00     │ 127      │ 2,400,000 │ 12m 34s   │  ← click
│ 2024-01-14 02:00     │ 125      │ 2,350,000 │ 11m 22s   │
│ 2024-01-13 02:00     │ 124      │ 2,300,000 │ 10m 45s   │
└─────────────────────────────────────────────────────────┘
```

### 7.3 Services List Page (for selected scan)

```
┌─────────────────────────────────────────────────────────┐
│ ← Scans / 2024-01-15 02:00                              │
├─────────────────────────────────────────────────────────┤
│ 127 services │ 2.4M total series                        │
├─────────────────────────────────────────────────────────┤
│ [Search...]                              Sort: [Series ▼]
├─────────────────────────────────────────────────────────┤
│ Service              │ Series    │ Metrics              │
│ payment-gateway      │ 245,231   │ 89                   │
│ user-service         │ 189,442   │ 67                   │
└─────────────────────────────────────────────────────────┘
```

### 7.4 Service Detail Page (metrics list)

```
┌─────────────────────────────────────────────────────────┐
│ ← 2024-01-15 / payment-gateway                          │
├─────────────────────────────────────────────────────────┤
│ 245,231 series │ 89 metrics                             │
├─────────────────────────────────────────────────────────┤
│ Metric                        │ Series   │ Labels       │
│ http_request_duration_seconds │ 45,231   │ 8            │
│ http_requests_total           │ 38,992   │ 6            │
└─────────────────────────────────────────────────────────┘
```

### 7.5 Metric Detail Page (labels list)

```
┌─────────────────────────────────────────────────────────┐
│ ← payment-gateway / http_request_duration_seconds       │
├─────────────────────────────────────────────────────────┤
│ 45,231 series │ 8 labels                                │
├─────────────────────────────────────────────────────────┤
│ Label       │ Unique Values │ Sample Values             │
│ endpoint    │ 1,234         │ /api/v1/pay, /api/v1/...  │
│ status_code │ 8             │ 200, 201, 400, 404, ...   │
└─────────────────────────────────────────────────────────┘
```

### 7.6 Navigation

**Routes (hash-based):**
```
#/                                    → Scans list OR auto-redirect to latest
#/scans/{id}                          → Services list
#/scans/{id}/services/{name}          → Metrics list
#/scans/{id}/services/{s}/metrics/{m} → Labels list
```

**Default behavior:** On load, fetch `/api/scans/latest` and navigate to that scan's services.
User can go back to scans list to see history and compare.

- [ ] Breadcrumb navigation at each level
- [ ] Scan controls in header (trigger scan, status)
- [ ] "All Scans" link to see history

---

## Phase 8: Cleanup

### 8.1 Final Cleanup

- [ ] Remove all unused files
- [ ] Update `main.go`
- [ ] Update `config.yaml`
- [ ] Test full flow

---

## Files Summary

### Completed ✅
- `storage/migrations/001_initial_schema.sql` - rewritten
- `storage/sqlite.go` - updated
- `storage/snapshots_repo.go` - rewritten
- `storage/services_repo.go` - created
- `storage/metrics_repo.go` - rewritten
- `storage/labels_repo.go` - created
- `models/models.go` - rewritten

### Deleted ✅
- `storage/recommendations_repo.go`
- `storage/dashboards_repo.go`

### To Do
- `config/config.go` - simplify
- `prometheus/client.go` - add service discovery
- `collector/prometheus_collector.go` - rewrite
- `api/handlers.go` - rewrite
- `api/server.go` - update routes
- `web/src/api.ts` - update types
- `web/src/App.tsx` - rewrite UI
- `main.go` - update wiring

### To Remove
- `analyzer/` directory
- `grafana/` directory
- `collector/grafana_collector.go`

---

## Out of Scope (MVP)

- ❌ Trends/Deltas (Δ1d, Δ7d, Δ30d)
- ❌ Trend charts
- ❌ Grafana integration
- ❌ Recommendations
- ❌ Team attribution
- ❌ Alerts/notifications
- ❌ Authentication