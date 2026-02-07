# Code Review — Pass 2: Application Layers

> Reviewed: 2026-02-07
> Scope: Prometheus client, collector, scheduler, API layer, analyzer

---

## 1. Prometheus Client (`prometheus/client.go`)

### Issues

**[CRITICAL] No HTTP transport timeouts**
- HTTP client created with no dial, TLS, or response header timeouts
- One slow Prometheus can hang the entire scan indefinitely
- **Fix:** Configure `http.Transport` with `DialContext` timeout (5s), `TLSHandshakeTimeout` (5s), `ResponseHeaderTimeout` (30s)

**[IMPORTANT] Labels query unbounded**
- `GetLabelsForMetric` fetches all series for a metric, can return millions of rows
- No pagination or limit on response size
- **Fix:** Add context timeout per query, consider using `/api/v1/labels` endpoint instead

**[IMPORTANT] No retry logic**
- Transient 5xx from Prometheus = entire scan fails
- **Fix:** Add simple retry with backoff for GET requests (1-2 retries)

**[SUGGESTION] No pre-allocation**
- Slices for services, metrics, labels grow dynamically
- **Fix:** Pre-allocate with `make([]T, 0, expectedLen)` where size is known

---

## 2. Collector (`collector/prometheus_collector.go`)

### Issues

**[CRITICAL] Semaphore double-release bug**
- `collectService` acquires semaphore at line ~160, releases at line ~186
- If error occurs between acquire and release, semaphore slot leaks
- Nested metric goroutines also acquire/release same semaphore
- **Fix:** Use `defer func() { <-sem }()` immediately after acquire, restructure to avoid double-release

**[CRITICAL] Context cancellation + semaphore = goroutine leak**
- If context cancelled while goroutine waits for semaphore, it blocks forever
- **Fix:** Use `select { case sem <- struct{}{}: ... case <-ctx.Done(): return }`

**[IMPORTANT] Service errors silently dropped**
- Service collection error logged but not accumulated
- Scan reports success even if 50% of services failed
- **Fix:** Track errors per service, include in `CollectResult`, let caller decide

**[IMPORTANT] No per-service timeout**
- One slow service blocks a semaphore slot indefinitely (only limited by parent context)
- **Fix:** `context.WithTimeout(ctx, 2*time.Minute)` per service

**[IMPORTANT] Progress callback under mutex**
- If callback does I/O (HTTP response), blocks the mutex
- **Fix:** Use buffered channel for progress, separate goroutine dispatches

**[SUGGESTION] Type inconsistency**
- `totalSeries` is `atomic.Int64` but `ServiceInfo.SeriesCount` is `int` — overflow risk on 32-bit

---

## 3. Scheduler (`scheduler/scheduler.go`)

### Issues

**[CRITICAL] Double close panic**
- `close(s.stopCh)` with no guard — if `Stop()` called twice, panic
- **Fix:** Use `sync.Once` for Stop()

**[CRITICAL] Race condition on status**
- Multiple status fields updated in defer without holding lock for entire update
- `GetStatus()` can read partial state during multi-field update
- **Fix:** Hold lock for entire status update block in defer

**[IMPORTANT] TriggerScan uses context.Background**
- Ignores caller's context, creates unbounded background context
- No way to cancel a triggered scan
- **Fix:** Thread context from caller, or store cancel func

**[IMPORTANT] Confusing dual entry points**
- `TriggerScan` and `runScan` have different locking assumptions
- `runScanAlreadyLocked` name misleading — doesn't require lock
- **Fix:** Consolidate into single entry point with consistent locking

**[IMPORTANT] No graceful shutdown for running scan**
- Stop/cancel returns immediately, but scan may still be running
- May write to closed DB
- **Fix:** Wait for scan goroutine completion or track scan context

**[SUGGESTION] Retention=0 disables cleanup silently**
- No log message when cleanup is disabled
- **Fix:** Log during scheduler init

---

## 4. API Layer

### Server (`api/server.go`) [IMPORTANT]
- No `IdleTimeout` or `ReadHeaderTimeout` — connection leaks possible
- No `MaxHeaderBytes` limit
- **Fix:** Add `IdleTimeout: 120s`, `ReadHeaderTimeout: 10s`, `MaxHeaderBytes: 1<<20`

### Middleware (`api/middleware.go`)

**[CRITICAL] 30s timeout applied to ALL requests**
- AI analysis can take minutes — will always timeout
- `POST /api/analysis` and `POST /api/scan` need exemption
- **Fix:** Exempt long-running endpoints from global timeout

**[IMPORTANT] Panic recovery loses stack trace**
- Only logs error value, no stack trace
- **Fix:** Use `runtime/debug.Stack()` in recovery

### Static Handler (`api/static.go`) [IMPORTANT]
- `fs.Sub(distFS, "dist")` error ignored — will panic if dist dir missing
- **Fix:** Check error, fail fast during init

### Helpers (`api/helpers/utils.go`) [IMPORTANT]
- JSON encoding error swallowed — client gets status 200 with corrupt body
- **Fix:** Buffer JSON to `bytes.Buffer` first, then write to ResponseWriter

### All Handlers [IMPORTANT]
- No request body size limit on POST/DELETE endpoints
- **Fix:** `http.MaxBytesReader(w, r.Body, 1<<20)` before decode
- Repository errors always return 500 — no 404 distinction
- **Fix:** Check for `sql.ErrNoRows` and return 404

---

## 5. Analyzer (`analyzer/analyzer.go`)

### Issues

**[CRITICAL] Global running flag blocks all analysis**
- Single boolean prevents ANY concurrent analysis, even for different snapshot pairs
- **Fix:** Use `map[string]bool` keyed by `"currentID:previousID"`

**[CRITICAL] Background context loses cancellation**
- `context.Background()` in goroutine — analysis continues after server shutdown
- May write to closed DB, cause panics
- **Fix:** Thread context through analyzer, add `Close()` method with cancel

**[IMPORTANT] No timeout for Gemini API calls**
- `SendMessage` has no timeout — if Gemini hangs, analysis hangs forever
- **Fix:** `context.WithTimeout(ctx, 2*time.Minute)` per API call

**[IMPORTANT] Max iterations = silent truncation**
- Loop exits at 20 iterations without marking analysis as incomplete
- **Fix:** Log warning, consider marking status as "partial"

**[IMPORTANT] Empty response marked as Completed**
- "No analysis generated." set as result, status = Completed
- **Fix:** Mark as Failed when no useful output

**[IMPORTANT] Chat session never closed**
- Potential resource leak if genai requires cleanup

### Tools (`analyzer/tools.go`) [SUGGESTION]
- `getInt64Arg` has unreachable `int64`/`int` cases (JSON always produces `float64`)
- No numeric range validation on snapshot IDs
- **Fix:** Simplify to float64-only path, add range checks