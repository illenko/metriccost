-- metric_snapshots: daily snapshot per metric (aggregated)
CREATE TABLE IF NOT EXISTS metric_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collected_at TIMESTAMP NOT NULL,
    metric_name TEXT NOT NULL,
    cardinality INTEGER NOT NULL,
    estimated_size_bytes INTEGER NOT NULL,
    sample_count INTEGER,
    team TEXT,
    labels_json TEXT,  -- JSON: {"label": unique_values_count}
    UNIQUE(metric_name, collected_at)
);

CREATE INDEX IF NOT EXISTS idx_metric_snapshots_date ON metric_snapshots(collected_at);
CREATE INDEX IF NOT EXISTS idx_metric_snapshots_metric ON metric_snapshots(metric_name);
CREATE INDEX IF NOT EXISTS idx_metric_snapshots_team ON metric_snapshots(team);

-- recommendations: generated optimization recommendations
CREATE TABLE IF NOT EXISTS recommendations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TIMESTAMP NOT NULL,
    metric_name TEXT NOT NULL,
    type TEXT NOT NULL,       -- 'high_cardinality', 'unused', 'redundant_labels'
    priority TEXT NOT NULL,   -- 'high', 'medium', 'low'
    current_cardinality INTEGER,
    current_size_bytes INTEGER,
    potential_reduction_bytes INTEGER,
    description TEXT,
    suggested_action TEXT
);

CREATE INDEX IF NOT EXISTS idx_recommendations_priority ON recommendations(priority);
CREATE INDEX IF NOT EXISTS idx_recommendations_type ON recommendations(type);

-- dashboard_stats: Grafana dashboard usage tracking
CREATE TABLE IF NOT EXISTS dashboard_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collected_at TIMESTAMP NOT NULL,
    dashboard_uid TEXT NOT NULL,
    dashboard_name TEXT NOT NULL,
    folder_name TEXT,
    last_viewed_at TIMESTAMP,
    query_count INTEGER,
    metrics_used TEXT,  -- JSON array of metric names
    UNIQUE(dashboard_uid, collected_at)
);

CREATE INDEX IF NOT EXISTS idx_dashboard_stats_uid ON dashboard_stats(dashboard_uid);
CREATE INDEX IF NOT EXISTS idx_dashboard_stats_date ON dashboard_stats(collected_at);

-- snapshots: overall system snapshots (totals history)
CREATE TABLE IF NOT EXISTS snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collected_at TIMESTAMP NOT NULL UNIQUE,
    total_metrics INTEGER NOT NULL,
    total_cardinality INTEGER NOT NULL,
    total_size_bytes INTEGER NOT NULL,
    team_breakdown TEXT  -- JSON: {"team_name": {"cardinality": 1000, "size_bytes": 123456}}
);

CREATE INDEX IF NOT EXISTS idx_snapshots_date ON snapshots(collected_at);
