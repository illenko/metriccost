import { useEffect, useState, useCallback } from 'react'
import {api} from '../api'
import type { Scan, Service, ScanStatus, ScanProgress } from '../api'
import { navigate } from '../lib/router'
import { formatNumber, formatDate } from '../lib/format'
import { useDebounce } from '../hooks/useDebounce'
import { DataTable, type Column } from '../components/DataTable'
import { Loading, EmptyState } from '../components/Loading'
import { Input } from '../components/Input'
import { Select } from '../components/Select'
import { Button } from '../components/Button'

interface ScansPageProps {
  scanStatus: ScanStatus | null
  onScan: () => void
}

export function ScansPage({ scanStatus, onScan }: ScansPageProps) {
  const [scans, setScans] = useState<Scan[]>([])
  const [selectedScanId, setSelectedScanId] = useState<number | null>(null)
  const [services, setServices] = useState<Service[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 300)

  // Load all scans and select the latest one
  useEffect(() => {
    async function loadInitial() {
      setLoading(true)
      try {
        const scansData = await api.getScans()
        setScans(scansData || [])
        if (scansData && scansData.length > 0) {
          setSelectedScanId(scansData[0].id) // Latest scan is first
        }
      } catch (err) {
        console.error('Failed to load scans:', err)
      }
      setLoading(false)
    }
    loadInitial()
  }, [])

  // Load services when scan or debounced search changes
  const loadServices = useCallback(async () => {
    if (!selectedScanId) return
    try {
      const data = await api.getServices(selectedScanId, { search: debouncedSearch || undefined })
      setServices(data || [])
    } catch (err) {
      console.error('Failed to load services:', err)
    }
  }, [selectedScanId, debouncedSearch])

  useEffect(() => {
    if (selectedScanId) {
      loadServices()
    }
  }, [selectedScanId, loadServices])

  if (loading) return <Loading />

  if (scans.length === 0) {
    return (
      <EmptyState
        title="No scans yet"
        description="Run a scan to discover services and metrics"
      />
    )
  }

  const columns: Column<Service>[] = [
    {
      key: 'name',
      header: 'Service',
      render: (svc) => <span className="font-mono text-gray-900 dark:text-gray-100">{svc.name}</span>,
    },
    {
      key: 'series',
      header: 'Series',
      align: 'right',
      render: (svc) => <span className="text-gray-600 dark:text-gray-400">{formatNumber(svc.total_series)}</span>,
    },
    {
      key: 'metrics',
      header: 'Metrics',
      align: 'right',
      render: (svc) => <span className="text-gray-500 dark:text-gray-500">{svc.metric_count}</span>,
    },
  ]

  function handleRowClick(svc: Service) {
    if (selectedScanId) {
      navigate({ page: 'metrics', scanId: selectedScanId, serviceName: svc.name })
    }
  }

  return (
    <div className="space-y-6">
      <ScanStatusBanner status={scanStatus} onScan={onScan} />

      <div className="flex items-center justify-between flex-wrap gap-4">
        <div className="flex items-center gap-4 flex-wrap">
          <Select
            value={selectedScanId || ''}
            onChange={(e) => setSelectedScanId(Number(e.target.value))}
            aria-label="Select snapshot"
          >
            {scans.map((scan) => (
              <option key={scan.id} value={scan.id}>
                Snapshot #{scan.id} — {formatDate(scan.collected_at)}
              </option>
            ))}
          </Select>
          <span className="text-sm text-gray-500 dark:text-gray-400">
            {formatNumber(services.length)} services · {formatNumber(services.reduce((sum, s) => sum + s.total_series, 0))} total series
          </span>
        </div>
        <Input
          type="text"
          placeholder="Search services..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-64"
          aria-label="Search services"
        />
      </div>

      <DataTable
        columns={columns}
        data={services}
        keyExtractor={(svc) => svc.id}
        onRowClick={handleRowClick}
      />
    </div>
  )
}

function formatScanProgress(progress: ScanProgress | null): string {
  if (!progress || !progress.phase) {
    return 'Initializing scan...'
  }

  switch (progress.phase) {
    case 'discovering':
      return 'Discovering services...'
    case 'processing_service':
      return `Scanning: ${progress.detail} (${progress.current} of ${progress.total} done)`
    case 'collecting_labels':
      return `[${progress.detail}] Collecting labels for ${progress.current} metrics...`
    case 'service_complete':
      if (progress.current === progress.total) {
        return `Scan complete! Finalizing...`
      }
      return `Completed: ${progress.detail} (${progress.current} of ${progress.total} done)`
    default:
      const phase = progress.phase.replace(/_/g, ' ')
      return `${phase.charAt(0).toUpperCase() + phase.slice(1)}...`
  }
}

function ScanStatusBanner({ status, onScan }: { status: ScanStatus | null; onScan: () => void }) {
  if (!status) return null

  return (
    <div className="border border-gray-200 dark:border-gray-700 rounded-lg p-4 bg-gray-50 dark:bg-gray-800/50">
      {status.running ? (
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="w-2.5 h-2.5 bg-green-500 rounded-full animate-pulse" aria-hidden="true" />
            <div>
              <div className="text-sm font-medium text-gray-900 dark:text-gray-100">Scan in progress</div>
              <div className="text-xs text-gray-500 dark:text-gray-400" aria-live="polite">
                {formatScanProgress(status.progress)}
              </div>
            </div>
          </div>
        </div>
      ) : (
        <div className="flex items-center justify-between flex-wrap gap-4">
          <div className="flex items-center gap-6 flex-wrap">
            <StatusItem label="Last scan" value={status.last_scan_at ? formatDate(status.last_scan_at) : 'Never'} />
            {status.last_duration && <StatusItem label="Duration" value={status.last_duration} />}
            {status.next_scan_at && <StatusItem label="Next scan" value={formatDate(status.next_scan_at)} />}
            {status.total_services > 0 && <StatusItem label="Services" value={formatNumber(status.total_services)} />}
            {status.total_series > 0 && <StatusItem label="Series" value={formatNumber(status.total_series)} />}
          </div>
          <Button onClick={onScan} size="sm" aria-label="Trigger a new scan">Run Scan</Button>
        </div>
      )}
      {!status.running && status.last_error && (
        <div className="mt-2 text-xs text-red-600 dark:text-red-400">Last error: {status.last_error}</div>
      )}
    </div>
  )
}

function StatusItem({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs text-gray-500 dark:text-gray-400">{label}</div>
      <div className="text-sm font-medium text-gray-900 dark:text-gray-100">{value}</div>
    </div>
  )
}
