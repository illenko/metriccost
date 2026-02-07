# Stabilization Plan

> Generated: 2026-02-07
> Status: Complete (both review passes done)
> Details: [Pass 1 review](code-review-pass1.md) | [Pass 2 review](code-review-pass2.md)

This document tracks the full stabilization effort for whodidthis before adding new features.
The goal is to make the codebase production-ready: fix bugs, improve code quality, add tests, set up CI/CD.

**Scope exclusions** (private internal tool, not needed now):
- API authentication / CORS restriction
- Rate limiting
- API versioning
- OpenAPI / Swagger
- Self-exported Prometheus metrics

**Issue summary:** ~80 issues found, distilled into 46 actionable items (15 Critical, 28 Important, rest Suggestions)

---

## Phase 1: Build Toolchain & Project Setup

### 1.1 Fix Makefile [CRITICAL]
- **File:** `Makefile:6,10`
- `MAIN_PATH=./cmd/whodidthis` points to non-existent directory. Main is at root.
- `-ldflags "-X main.Version=$(VERSION)"` but `main.go` has no `Version` variable.
- Missing `.PHONY` for `deps` and `build-run`.
- No `docker-build` target.
- **Fix:**
  - Change `MAIN_PATH=.`
  - Add `version`, `commit`, `buildTime` vars to `main.go`
  - Update ldflags: `-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)`
  - Add `docker-build`, `install`, `test-coverage` targets
  - Add `.PHONY` for all targets

### 1.2 Fix Dockerfile [CRITICAL]
- **File:** `Dockerfile`
- Single-stage build expects pre-built binary — breaks `docker build` for anyone.
- Runs as root (no `USER` directive).
- No `HEALTHCHECK`.
- Alpine 3.19 is outdated.
- **Fix:**
  - Multi-stage build (golang:1.25-alpine builder + alpine:3.21 runtime)
  - Add non-root user `whodidthis` (uid 1000)
  - Add `HEALTHCHECK` directive
  - Strip binary with `-ldflags "-s -w"`

### 1.3 Fix .gitignore [CRITICAL]
- **File:** `.gitignore:46-48`
- Ignores `docker-compose.yml`, `Dockerfile`, and `data/` — these should be tracked.
- Missing `.env` entry.
- **Fix:** Remove Dockerfile/docker-compose from ignore, add `.env`

### 1.4 Fix docker-compose.yml [IMPORTANT]
- **File:** `docker-compose.yml:17`
- Hardcoded `WDT_GEMINI_API_KEY=123` — secret leak risk.
- No resource limits.
- No healthcheck.
- **Fix:** Use `${GEMINI_API_KEY}` from `.env`, add resource limits (512M/1cpu), add healthcheck

---

## Phase 2: Configuration

### 2.1 Rewrite config loading — two broken code paths [CRITICAL]
- **File:** `config/config.go:62-96`
- **Root problem:** Two completely separate code paths that behave differently:
  - **YAML found (line 84-95):** Uses `v.Unmarshal(&cfg)` — but Viper's `AutomaticEnv()` does NOT work with `Unmarshal()`, only with `v.Get*()` calls. So env vars like `WDT_GEMINI_API_KEY` are **silently ignored** when config.yaml exists.
  - **YAML not found (line 76-81):** Falls to `envConfig()` which manually maps every field via `v.Get*()`. This is a **duplicate mapping** that must be kept in sync. Also `Validate()` is never called on this path.
- **Additional issues:**
  - `applyDefaults()` runs AFTER `Validate()` on the YAML path — should be before.
  - Only validates 3 fields. Missing: `Scan.Interval`, `Scan.SampleValuesLimit`, `Storage.Path`, `Log.Level`.
  - `fmt.Println` used for error output instead of structured logging.
- **Desired behavior:** config.yaml as base, any value overridable by `WDT_*` env vars.
- **Fix:** Delete `envConfig()` entirely. Use Viper's `BindEnv` or `SetEnvKeyReplacer` to map nested keys to flat env vars (e.g., `prometheus.url` -> `WDT_PROMETHEUS_URL`). Single code path: read file (optional) -> bind env overrides -> unmarshal -> apply defaults -> validate. This is exactly what Viper is designed for.

### 2.2 Complete config.example.yaml [IMPORTANT]
- **File:** `config.example.yaml`
- Missing entire `gemini` section.
- Some default values don't match code.
- **Fix:** Add gemini section, align all defaults with code.

### 2.3 Add timeout configs [IMPORTANT]
- No configurable timeouts for Prometheus HTTP client or Gemini API calls.
- **Fix:** Add `prometheus.timeout` and `gemini.timeout` to config struct and wire them through.

---

## Phase 3: Main Entry Point

### 3.1 Refactor main.go error handling [CRITICAL]
- **File:** `main.go`
- Multiple `os.Exit(1)` calls scattered throughout — defers never run, resources leak.
- `defer db.Close()` doesn't check returned error.
- Prometheus client never closed.
- No version info printed at startup.
- DEBUG env var bypasses config log level (inconsistent).
- **Fix:**
  - Extract all init logic to `run() error`
  - Single `os.Exit(1)` in `main()` after `run()` returns error
  - Check `db.Close()` error in deferred func
  - Log version/commit/config on startup
  - Remove DEBUG env var hack, use config only

---

## Phase 4: Models

### 4.1 Add doc comments to exported types [SUGGESTION]
- **File:** `models/models.go`
- No doc comments on any exported type — violates Go conventions.
- **Fix:** Add godoc comments to all exported types and constants.

### 4.2 Add AnalysisStatus validation [SUGGESTION]
- **File:** `models/models.go:57`
- `AnalysisStatus` is a string type with no validation.
- **Fix:** Add `IsValid()` method.

---

## Phase 5: Project Structure & Testability

### 5.1 Split analyzer/analyzer.go (535 lines) [IMPORTANT]
- **File:** `analyzer/analyzer.go` — largest file, does too much in one place.
- Prompt building (130-line string), agentic loop, state management, and Gemini client setup all in one file.
- **Fix:** Split into:
  - `analyzer.go` — struct, `New()`, `Close()`, `StartAnalysis()`, state management
  - `prompt.go` — prompt building logic (or embed as `.txt` file)
  - `runner.go` — agentic loop (`runAnalysis`, message handling, iteration)

### 5.2 Add interfaces at package boundaries [IMPORTANT]
- No interfaces anywhere — every package depends on concrete types.
- `analyzer` imports `*storage.AnalysisRepository` directly, `collector` imports `*prometheus.Client` directly.
- Makes unit testing impossible without real DB/Prometheus.
- **Fix:**
  - `storage/` — export repository interfaces alongside structs (e.g., `SnapshotsRepo` interface, `snapshotsRepository` concrete type)
  - `prometheus/` — export a `Client` interface, concrete type implements it
  - Consumers accept interfaces, implementations are injected via constructors
  - This unblocks writing tests with mocks for phases 12.3-12.8

### 5.3 Rename `api/helpers` to `api/response.go` [SUGGESTION]
- **File:** `api/helpers/utils.go` — 32 lines, just `WriteJSON` and `WriteError`.
- `helpers` is a Go anti-pattern (too generic, says nothing about purpose).
- **Fix:** Move `WriteJSON`/`WriteError` to `api/response.go` in the `api` package. Delete `helpers/` sub-package.

### 5.4 models/models.go — leave as-is for now [NO ACTION]
- 89 lines, all related domain structs. Splitting would be over-engineering at this size.
- **Revisit** if it grows past ~200 lines.

---

## Phase 6: Storage Layer

### 6.1 Fix migration system [CRITICAL]
- **File:** `storage/sqlite.go:57-81`
- No migration tracking — re-runs ALL migrations on every start.
- Migrations not wrapped in transaction — partial failure leaves DB inconsistent.
- `ReadDir` order is OS-dependent, not explicitly sorted.
- **Fix:**
  - Add `schema_migrations` table to track applied versions
  - Wrap each migration in a transaction
  - Explicitly sort migration files
  - Skip already-applied migrations

### 6.2 Add context timeouts to all repos [CRITICAL]
- **Files:** All `storage/*_repo.go`
- No timeout enforcement on any database operation.
- **Fix:** Either set `busy_timeout` pragma (already done: 5000ms) and trust it, or wrap calls with `context.WithTimeout`.

### 6.3 Fix repository error handling [IMPORTANT]
- **Files:** All `storage/*_repo.go`
- `GetByName` methods return `nil, nil` when not found — callers must nil-check instead of `errors.Is(err, sql.ErrNoRows)`.
- Inconsistent error wrapping — some bare errors, some wrapped.
- `LastInsertId()` errors ignored in some batch ops.
- **Fix:** Always return wrapped errors. Return `sql.ErrNoRows` wrapped when not found. Check all `LastInsertId()` returns.

### 6.4 Fix batch transaction rollback handling [IMPORTANT]
- **Files:** All `*_repo.go` CreateBatch methods
- `defer tx.Rollback()` logs error after successful commit (rollback on committed tx).
- **Fix:** Check for `sql.ErrTxDone` in deferred rollback.

### 6.5 Remove collected_at UNIQUE or document constraint [IMPORTANT]
- **File:** `storage/migrations/001_initial_schema.sql:4`
- `collected_at TIMESTAMP NOT NULL UNIQUE` blocks same-second scans (e.g., rapid manual triggers).
- **Fix:** Remove UNIQUE or use nanosecond precision.

### 6.6 Fix potential SQL injection in services_repo [IMPORTANT]
- **File:** `storage/services_repo.go:73-93`
- ORDER BY clause built with string concatenation. `Sort` is validated but `Order` is not validated before concat.
- **Fix:** Validate `Order` is one of `ASC`, `DESC` before concatenation.

---

## Phase 7: Prometheus Client

### 7.1 Add HTTP transport timeouts [CRITICAL]
- **File:** `prometheus/client.go`
- HTTP client has no dial, TLS, or response header timeouts.
- One slow Prometheus hangs the entire scan.
- **Fix:** Configure `http.Transport` with DialContext (5s), TLSHandshakeTimeout (5s), ResponseHeaderTimeout (30s).

### 7.2 Bound labels query [IMPORTANT]
- `GetLabelsForMetric` fetches all series — can return millions of rows.
- **Fix:** Add context timeout per query, consider limits.

### 7.3 Add retry logic [SUGGESTION]
- Transient 5xx from Prometheus = entire scan fails.
- **Fix:** Simple retry with backoff for GET requests (1-2 retries).

---

## Phase 8: Collector

### 8.1 Fix semaphore management [CRITICAL]
- **File:** `collector/prometheus_collector.go`
- Semaphore double-release bug — error between acquire and release leaks slot.
- Context cancellation while waiting for semaphore = goroutine blocks forever.
- **Fix:**
  - Use `defer func() { <-sem }()` immediately after acquire
  - Use `select { case sem <- struct{}{}: case <-ctx.Done(): return }` pattern

### 8.2 Track and report service errors [IMPORTANT]
- Service errors logged but not accumulated — scan reports success even if 50% failed.
- **Fix:** Add error tracking to `CollectResult`, let caller decide.

### 8.3 Add per-service timeout [IMPORTANT]
- One slow service blocks a semaphore slot indefinitely.
- **Fix:** `context.WithTimeout(ctx, 2*time.Minute)` per service.

### 8.4 Fix progress callback under mutex [SUGGESTION]
- If callback does I/O, blocks the mutex.
- **Fix:** Use buffered channel, separate goroutine dispatches updates.

---

## Phase 9: Scheduler

### 9.1 Fix double-close panic [CRITICAL]
- **File:** `scheduler/scheduler.go`
- `close(s.stopCh)` with no guard — panics if Stop() called twice.
- **Fix:** Use `sync.Once` for Stop().

### 9.2 Fix race condition on status [CRITICAL]
- Multiple status fields updated in defer without holding lock for entire update.
- `GetStatus()` reads partial state.
- **Fix:** Hold lock for entire status update block.

### 9.3 Consolidate scan entry points [IMPORTANT]
- `TriggerScan` and `runScan` have different locking assumptions.
- `TriggerScan` uses `context.Background()` — no cancellation.
- **Fix:** Single entry point, proper context threading.

### 9.4 Add graceful shutdown for running scans [IMPORTANT]
- Stop returns immediately but scan may still be running.
- **Fix:** Wait for scan goroutine completion, or cancel scan context.

---

## Phase 10: API Layer

### 10.1 Fix middleware timeout for long-running endpoints [CRITICAL]
- **File:** `api/middleware.go`
- 30s timeout applied to ALL requests — AI analysis always times out.
- **Fix:** Exempt `POST /api/analysis` and `POST /api/scan` from global timeout.

### 10.2 Add panic recovery stack traces [IMPORTANT]
- Panic recovery only logs error value, no stack trace.
- **Fix:** Use `runtime/debug.Stack()`.

### 10.3 Fix JSON encoding error handling [IMPORTANT]
- **File:** `api/helpers/utils.go`
- JSON error swallowed — client gets status 200 with corrupt body.
- **Fix:** Buffer to `bytes.Buffer` first, then write.

### 10.4 Fix static handler error handling [IMPORTANT]
- **File:** `api/static.go`
- `fs.Sub(distFS, "dist")` error ignored — will panic if dist missing.
- **Fix:** Check error, fail fast.

### 10.5 Add request body size limits [IMPORTANT]
- No size limit on POST/DELETE handlers.
- **Fix:** `http.MaxBytesReader(w, r.Body, 1<<20)` before decode.

### 10.6 Add server timeout defaults [IMPORTANT]
- **File:** `api/server.go`
- No `IdleTimeout`, `ReadHeaderTimeout`, or `MaxHeaderBytes`.
- **Fix:** Add `IdleTimeout: 120s`, `ReadHeaderTimeout: 10s`, `MaxHeaderBytes: 1MB`.

---

## Phase 11: Analyzer

### 11.1 Fix context propagation [CRITICAL]
- **File:** `analyzer/analyzer.go`
- `context.Background()` in goroutine — analysis continues after shutdown, writes to closed DB.
- **Fix:** Thread context through analyzer, add `Close()` method.

### 11.2 Allow concurrent analysis of different snapshot pairs [IMPORTANT]
- Global `running` flag blocks ALL analysis while one runs.
- **Fix:** `map[string]bool` keyed by `"currentID:previousID"`.

### 11.3 Add Gemini API call timeouts [IMPORTANT]
- `SendMessage` has no timeout — hangs forever if Gemini is slow.
- **Fix:** `context.WithTimeout(ctx, 2*time.Minute)` per call.

### 11.4 Handle max iterations gracefully [IMPORTANT]
- Loop exits at 20 iterations — analysis silently truncated.
- **Fix:** Log warning, mark as partial/failed if incomplete.

### 11.5 Fix empty response handling [SUGGESTION]
- "No analysis generated." marked as Completed.
- **Fix:** Mark as Failed.

---

## Phase 12: Tests & CI/CD

### 12.1 Set up CI/CD pipeline [CRITICAL]
- No GitHub Actions, no linting, no automated builds.
- **Fix:** Create `.github/workflows/ci.yml` with: `go vet`, `golangci-lint`, `go test -race`, build, Docker build.

### 12.2 Add golangci-lint config [IMPORTANT]
- **Fix:** Create `.golangci.yml` with reasonable defaults (errcheck, govet, staticcheck, unused, etc.)

### 12.3 Add unit tests — storage layer [CRITICAL]
- Zero test files exist.
- **Priority:** repos (in-memory SQLite) -> config -> collector -> handlers

### 12.4 Add unit tests — config [IMPORTANT]
- Test validation, defaults, env var loading.

### 12.5 Add unit tests — collector [IMPORTANT]
- Test with mock Prometheus client. Run with `-race`.

### 12.6 Add unit tests — scheduler [IMPORTANT]
- Test state machine, concurrent triggers, shutdown. Run with `-race`.

### 12.7 Add unit tests — API handlers [IMPORTANT]
- Test with mock repos using httptest.

### 12.8 Add unit tests — analyzer [SUGGESTION]
- Test tool execution with mock repos. Test iteration limits.

---

## Phase 13: Documentation

### 13.1 Create README.md [IMPORTANT]
- No README exists.
- **Include:** What it does, architecture overview, quickstart (Docker + binary), full config reference.

---

## Execution Order (Recommended)

Work in this order to maximize stability at each step:

1. **Phase 1** (Build toolchain) — unblocks everything else
2. **Phase 2** (Config) + **Phase 3** (main.go) — foundational correctness
3. **Phase 5** (Structure & testability) — interfaces unblock writing tests
4. **Phase 6** (Storage) — data layer must be solid
5. **Phase 7** (Prometheus client) + **Phase 8** (Collector) — core scraping logic
6. **Phase 9** (Scheduler) — orchestration layer
7. **Phase 10** (API) + **Phase 11** (Analyzer) — user-facing layers
8. **Phase 4** (Models) — minor cleanup
9. **Phase 12** (Tests & CI/CD) — ideally write tests alongside each phase
10. **Phase 13** (Documentation) — last, after code is stable

---

## Tracking

| Phase | Items | Critical | Important | Status |
|-------|-------|----------|-----------|--------|
| 1. Build Toolchain | 1.1 - 1.4 | 3 | 1 | TODO |
| 2. Configuration | 2.1 - 2.3 | 1 | 2 | TODO |
| 3. Main Entry Point | 3.1 | 1 | 0 | TODO |
| 4. Models | 4.1 - 4.2 | 0 | 0 | TODO |
| 5. Structure & Testability | 5.1 - 5.4 | 0 | 2 | TODO |
| 6. Storage Layer | 6.1 - 6.6 | 2 | 4 | TODO |
| 7. Prometheus Client | 7.1 - 7.3 | 1 | 1 | TODO |
| 8. Collector | 8.1 - 8.4 | 1 | 2 | TODO |
| 9. Scheduler | 9.1 - 9.4 | 2 | 2 | TODO |
| 10. API Layer | 10.1 - 10.6 | 1 | 5 | TODO |
| 11. Analyzer | 11.1 - 11.5 | 1 | 3 | TODO |
| 12. Tests & CI/CD | 12.1 - 12.8 | 2 | 5 | TODO |
| 13. Documentation | 13.1 | 0 | 1 | TODO |
| **Total** | **46 items** | **15** | **28** | |