import { useEffect, useState } from 'react'
import { api } from './api'
import type { Overview, Metric, Recommendation, ScanStatus } from './api'

function formatBytes(bytes: number): string {
  if (bytes >= 1e9) return (bytes / 1e9).toFixed(2) + ' GB'
  if (bytes >= 1e6) return (bytes / 1e6).toFixed(2) + ' MB'
  if (bytes >= 1e3) return (bytes / 1e3).toFixed(2) + ' KB'
  return bytes + ' B'
}

function formatNumber(n: number): string {
  return n.toLocaleString()
}

type Tab = 'overview' | 'metrics' | 'recommendations'

function getTabFromHash(): Tab {
  const hash = window.location.hash.slice(1)
  if (hash === 'metrics' || hash === 'recommendations') return hash
  return 'overview'
}

function App() {
  const [tab, setTab] = useState<Tab>(getTabFromHash)
  const [overview, setOverview] = useState<Overview | null>(null)
  const [metrics, setMetrics] = useState<Metric[]>([])
  const [recommendations, setRecommendations] = useState<Recommendation[]>([])
  const [scanStatus, setScanStatus] = useState<ScanStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')

  useEffect(() => {
    loadData()
    const interval = setInterval(loadScanStatus, 5000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    window.location.hash = tab
  }, [tab])

  useEffect(() => {
    const onHashChange = () => setTab(getTabFromHash())
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  useEffect(() => {
    if (tab === 'metrics') loadMetrics()
    if (tab === 'recommendations') loadRecommendations()
  }, [tab, search])

  async function loadData() {
    setLoading(true)
    try {
      const [ov, status] = await Promise.all([api.getOverview(), api.getScanStatus()])
      setOverview(ov)
      setScanStatus(status)
    } catch (e) {
      console.error(e)
    }
    setLoading(false)
  }

  async function loadScanStatus() {
    try {
      setScanStatus(await api.getScanStatus())
    } catch (e) {
      console.error(e)
    }
  }

  async function loadMetrics() {
    try {
      setMetrics(await api.getMetrics({ limit: 50, sort: 'size', search: search || undefined }))
    } catch (e) {
      console.error(e)
    }
  }

  async function loadRecommendations() {
    try {
      setRecommendations(await api.getRecommendations())
    } catch (e) {
      console.error(e)
    }
  }

  async function handleScan() {
    await api.triggerScan()
    loadScanStatus()
  }

  return (
    <div className="min-h-screen bg-white">
      {/* Header */}
      <header className="border-b border-gray-200">
        <div className="max-w-6xl mx-auto px-6 py-4 flex items-center justify-between">
          <h1 className="text-xl font-semibold text-gray-900">Metric Cost</h1>
          <div className="flex items-center gap-4">
            {scanStatus?.running && (
              <span className="text-sm text-gray-500 flex items-center gap-2">
                <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
                Scanning...
              </span>
            )}
            <button
              onClick={handleScan}
              disabled={scanStatus?.running}
              className="px-4 py-2 text-sm bg-gray-900 text-white rounded-lg hover:bg-gray-800 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Run Scan
            </button>
          </div>
        </div>
      </header>

      {/* Navigation */}
      <nav className="border-b border-gray-200">
        <div className="max-w-6xl mx-auto px-6">
          <div className="flex gap-8">
            {(['overview', 'metrics', 'recommendations'] as const).map((t) => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={`py-3 text-sm font-medium border-b-2 -mb-px capitalize ${
                  tab === t
                    ? 'border-gray-900 text-gray-900'
                    : 'border-transparent text-gray-500 hover:text-gray-700'
                }`}
              >
                {t}
              </button>
            ))}
          </div>
        </div>
      </nav>

      {/* Content */}
      <main className="max-w-6xl mx-auto px-6 py-8">
        {loading ? (
          <div className="text-gray-500">Loading...</div>
        ) : tab === 'overview' ? (
          <OverviewTab overview={overview} scanStatus={scanStatus} />
        ) : tab === 'metrics' ? (
          <MetricsTab metrics={metrics} search={search} onSearchChange={setSearch} />
        ) : (
          <RecommendationsTab recommendations={recommendations} />
        )}
      </main>
    </div>
  )
}

function OverviewTab({ overview, scanStatus }: { overview: Overview | null; scanStatus: ScanStatus | null }) {
  if (!overview) return <div className="text-gray-500">No data yet. Run a scan to get started.</div>

  return (
    <div className="space-y-8">
      {/* Stats */}
      <div className="grid grid-cols-4 gap-6">
        <StatCard label="Total Metrics" value={formatNumber(overview.total_metrics)} />
        <StatCard label="Total Cardinality" value={formatNumber(overview.total_cardinality)} />
        <StatCard label="Estimated Size" value={formatBytes(overview.total_size_bytes)} />
        <StatCard
          label="Trend"
          value={`${overview.trend_percentage >= 0 ? '+' : ''}${overview.trend_percentage.toFixed(1)}%`}
          trend={overview.trend_percentage}
        />
      </div>

      {/* Last Scan Info */}
      {scanStatus && scanStatus.last_scan_at && (
        <div className="text-sm text-gray-500">
          Last scan: {new Date(scanStatus.last_scan_at).toLocaleString()} ({scanStatus.last_duration})
        </div>
      )}

      {/* Team Breakdown */}
      {Object.keys(overview.team_breakdown || {}).length > 0 && (
        <div>
          <h2 className="text-lg font-medium text-gray-900 mb-4">By Team</h2>
          <div className="border border-gray-200 rounded-lg overflow-hidden">
            <table className="w-full">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Team</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Metrics</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Cardinality</th>
                  <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Size</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {Object.entries(overview.team_breakdown).map(([team, data]) => (
                  <tr key={team}>
                    <td className="px-4 py-3 text-sm text-gray-900">{team}</td>
                    <td className="px-4 py-3 text-sm text-gray-600 text-right">{formatNumber(data.metric_count)}</td>
                    <td className="px-4 py-3 text-sm text-gray-600 text-right">{formatNumber(data.cardinality)}</td>
                    <td className="px-4 py-3 text-sm text-gray-600 text-right">{formatBytes(data.size_bytes)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}

function MetricsTab({
  metrics,
  search,
  onSearchChange,
}: {
  metrics: Metric[]
  search: string
  onSearchChange: (s: string) => void
}) {
  return (
    <div className="space-y-4">
      <input
        type="text"
        placeholder="Search metrics..."
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
        className="w-full max-w-md px-4 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-gray-200"
      />

      <div className="border border-gray-200 rounded-lg overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Metric</th>
              <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Cardinality</th>
              <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Size</th>
              <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Team</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200">
            {metrics.map((m) => (
              <tr key={m.name}>
                <td className="px-4 py-3 text-sm font-mono text-gray-900">{m.name}</td>
                <td className="px-4 py-3 text-sm text-gray-600 text-right">{formatNumber(m.cardinality)}</td>
                <td className="px-4 py-3 text-sm text-gray-600 text-right">{formatBytes(m.estimated_size_bytes)}</td>
                <td className="px-4 py-3 text-sm text-gray-500 text-right">{m.team || '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function RecommendationsTab({ recommendations }: { recommendations: Recommendation[] }) {
  if (recommendations.length === 0) {
    return <div className="text-gray-500">No recommendations yet.</div>
  }

  const priorityColor = {
    high: 'bg-red-100 text-red-800',
    medium: 'bg-yellow-100 text-yellow-800',
    low: 'bg-gray-100 text-gray-800',
  }

  return (
    <div className="space-y-4">
      {recommendations.map((r) => (
        <div key={r.id} className="border border-gray-200 rounded-lg p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1">
              <div className="flex items-center gap-2 mb-1">
                <span className={`px-2 py-0.5 text-xs font-medium rounded ${priorityColor[r.priority as keyof typeof priorityColor] || priorityColor.low}`}>
                  {r.priority}
                </span>
                <span className="text-xs text-gray-500">{r.type.replace('_', ' ')}</span>
              </div>
              <div className="font-mono text-sm text-gray-900 mb-2">{r.metric_name}</div>
              <p className="text-sm text-gray-600">{r.description}</p>
              {r.suggested_action && (
                <p className="text-sm text-gray-500 mt-2 italic">{r.suggested_action}</p>
              )}
            </div>
            <div className="text-right text-sm">
              <div className="text-gray-500">Potential savings</div>
              <div className="font-medium text-green-600">{formatBytes(r.potential_reduction_bytes)}</div>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function StatCard({ label, value, trend }: { label: string; value: string; trend?: number }) {
  return (
    <div className="border border-gray-200 rounded-lg p-4">
      <div className="text-sm text-gray-500 mb-1">{label}</div>
      <div className={`text-2xl font-semibold ${trend !== undefined ? (trend >= 0 ? 'text-red-600' : 'text-green-600') : 'text-gray-900'}`}>
        {value}
      </div>
    </div>
  )
}

export default App
