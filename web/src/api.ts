const BASE_URL = '/api'

export interface Overview {
  total_metrics: number
  total_cardinality: number
  total_size_bytes: number
  trend_percentage: number
  last_scan: string
  team_breakdown: Record<string, TeamMetrics>
}

export interface TeamMetrics {
  cardinality: number
  size_bytes: number
  metric_count: number
}

export interface Metric {
  name: string
  cardinality: number
  estimated_size_bytes: number
  team: string
  trend_percentage: number
}

export interface Recommendation {
  id: number
  metric_name: string
  type: string
  priority: string
  current_cardinality: number
  current_size_bytes: number
  potential_reduction_bytes: number
  description: string
  suggested_action: string
}

export interface ScanStatus {
  running: boolean
  last_scan_at: string
  last_duration: string
  last_error: string
  next_scan_at: string
  prometheus_metrics: number
  grafana_dashboards: number
  recommendations: number
  total_size_bytes: number
}

export interface TrendPoint {
  date: string
  total_metrics: number
  cardinality: number
  size_bytes: number
}

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

export const api = {
  getOverview: () => fetchJSON<Overview>(`${BASE_URL}/overview`),
  getMetrics: (params?: { limit?: number; sort?: string; search?: string }) => {
    const query = new URLSearchParams()
    if (params?.limit) query.set('limit', String(params.limit))
    if (params?.sort) query.set('sort', params.sort)
    if (params?.search) query.set('search', params.search)
    return fetchJSON<Metric[]>(`${BASE_URL}/metrics?${query}`)
  },
  getRecommendations: (priority?: string) => {
    const query = priority ? `?priority=${priority}` : ''
    return fetchJSON<Recommendation[]>(`${BASE_URL}/recommendations${query}`)
  },
  getTrends: (period = '30d') => fetchJSON<TrendPoint[]>(`${BASE_URL}/trends?period=${period}`),
  getScanStatus: () => fetchJSON<ScanStatus>(`${BASE_URL}/scan/status`),
  triggerScan: () => fetch(`${BASE_URL}/scan`, { method: 'POST' }),
}
