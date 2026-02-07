# Code Review — Pass 1: Foundation Layers

> Reviewed: 2026-02-07
> Scope: Project structure, build toolchain, config, main.go, models, storage

---

## 1. Project Structure & Build Toolchain

### go.mod
- Go version `1.25.5` should be `1.25` (minor only, per Go convention)
- Heavy `google.golang.org/genai` dependency — consider documenting why it's required

### Makefile [CRITICAL]
- `MAIN_PATH=./cmd/whodidthis` — **directory doesn't exist**. Main is at root. Breaks all build targets.
- `-ldflags "-X main.Version=$(VERSION)"` — `main.go` has no `Version` variable. Silently fails.
- Missing `.PHONY` for `deps` and `build-run`
- No `docker-build` target
- `air` assumed installed, no install instructions
- No `install` target

### Dockerfile [CRITICAL]
- Single-stage build — `COPY ./whodidthis /app/whodidthis` expects pre-built binary
- Runs as root (no `USER` directive)
- No `HEALTHCHECK`
- Alpine 3.19 is outdated (3.21 is current)
- `chmod +x` happens at runtime instead of build time

### docker-compose.yml [IMPORTANT]
- Hardcoded `WDT_GEMINI_API_KEY=123` — secret leak risk
- No resource limits (memory/CPU)
- No healthcheck
- `host.docker.internal` only works on Docker Desktop, not Linux

### .gitignore [CRITICAL]
- Ignores `docker-compose.yml`, `Dockerfile`, and `data/` — should be tracked
- Missing `.env` entry

---

## 2. Configuration (`config/config.go`)

### Issues

**[CRITICAL] Two completely separate code paths (lines 62-96):**
- **YAML found (line 84-95):** Uses `v.Unmarshal(&cfg)`. But Viper's `AutomaticEnv()` does NOT work with `Unmarshal()` — it only works with `v.Get*()` calls. **Env vars are silently ignored when config.yaml exists.** You cannot override e.g. `WDT_GEMINI_API_KEY` if a config.yaml is present.
- **YAML not found (line 76-81):** Falls to `envConfig()` (lines 98-131) which manually maps every field via `v.Get*()`. This duplicate mapping must be kept in sync. `Validate()` is never called on this path.
- **Fix:** Delete `envConfig()`, use Viper's `BindEnv`/`SetEnvKeyReplacer` for proper YAML + env var layering in a single path.

**Other issues:**
- `applyDefaults()` runs AFTER `Validate()` — should be before
- Only validates 3 fields. Missing: `Scan.Interval`, `Scan.SampleValuesLimit`, `Storage.Path`, `Log.Level`
- `LogLevel()` returns `slog.LevelInfo` for unrecognized values with no warning
- `fmt.Println` used for error output (lines 77-78) instead of structured logging
- No doc comments on exported types
- No timeout configuration for HTTP clients

### config.example.yaml
- Missing entire `gemini` section
- Some default values inconsistent with code

---

## 3. Main Entry Point (`main.go`)

### Issues [CRITICAL]
- Multiple `os.Exit(1)` calls — defers never run, resources leak (db.Close, etc.)
- `defer db.Close()` doesn't check returned error
- Prometheus client never closed
- TextHandler for logging — should be JSONHandler for production
- `DEBUG=true` env var bypasses config log level (inconsistent)
- No version info printed at startup
- Hardcoded 30s shutdown timeout
- Second SIGINT blocks (channel buffer size 1)

### Fix
- Extract to `run() error` function, single `os.Exit` in `main()`
- Check `db.Close()` error in deferred func
- Log version/commit/config on startup

---

## 4. Models (`models/models.go`)

### Issues [MINOR]
- `AnalysisStatus` string type with no validation — add `IsValid()` method
- `ToolCall.Args` is `map[string]any`, `Result` is `any` — consider `json.RawMessage`
- No doc comments on exported types

---

## 5. Storage Layer

### sqlite.go [CRITICAL]
- No migration tracking — re-runs ALL migrations every start
- Migrations not wrapped in transaction — partial failure = inconsistent DB
- `ReadDir` order is OS-dependent, not explicitly sorted
- `MaxOpenConns(1)` is correct for SQLite but no query timeouts set
- `VACUUM` after cleanup blocks all operations

### All *_repo.go [IMPORTANT]
- No context timeout enforcement on any DB operation
- `GetByName` returns `nil, nil` when not found — callers must nil-check instead of `errors.Is(err, sql.ErrNoRows)`
- Inconsistent error wrapping (some bare, some wrapped)
- `LastInsertId()` errors ignored in some batch ops
- `defer tx.Rollback()` logs error after successful commit (`sql.ErrTxDone`)
- No prepared statement caching for batch ops

### services_repo.go [IMPORTANT]
- ORDER BY built with string concat — `Sort` validated but `Order` not validated before concat (potential SQL injection)

### Migrations
- `001`: `collected_at UNIQUE` blocks same-second scans (manual triggers)
- `002`: `UNIQUE(current_snapshot_id, previous_snapshot_id)` prevents retry without delete
- No migration versioning table