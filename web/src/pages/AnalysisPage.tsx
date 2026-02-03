import { useEffect, useState, useCallback } from 'react'
import Markdown from 'react-markdown'
import { api } from '../api'
import type { Scan, SnapshotAnalysis, AnalysisGlobalStatus, AnalysisStatusType } from '../api'
import { navigate } from '../lib/router'
import { formatDate } from '../lib/format'
import { Breadcrumb } from '../components/Breadcrumb'
import { Button } from '../components/Button'
import { Loading, EmptyState } from '../components/Loading'
import { Select } from '../components/Select'

const markdownComponents = {
  h1: ({ children, ...props }: React.HTMLAttributes<HTMLHeadingElement>) => (
    <h1 className="text-xl font-bold text-gray-900 dark:text-gray-100 mb-3 mt-4 first:mt-0" {...props}>{children}</h1>
  ),
  h2: ({ children, ...props }: React.HTMLAttributes<HTMLHeadingElement>) => (
    <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2 mt-4 first:mt-0" {...props}>{children}</h2>
  ),
  h3: ({ children, ...props }: React.HTMLAttributes<HTMLHeadingElement>) => (
    <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-2 mt-3" {...props}>{children}</h3>
  ),
  p: ({ children, ...props }: React.HTMLAttributes<HTMLParagraphElement>) => (
    <p className="text-sm text-gray-800 dark:text-gray-200 mb-2 leading-relaxed" {...props}>{children}</p>
  ),
  ul: ({ children, ...props }: React.HTMLAttributes<HTMLUListElement>) => (
    <ul className="list-disc list-outside ml-5 mb-3 space-y-1 text-sm text-gray-800 dark:text-gray-200" {...props}>{children}</ul>
  ),
  ol: ({ children, ...props }: React.HTMLAttributes<HTMLOListElement>) => (
    <ol className="list-decimal list-outside ml-5 mb-3 space-y-1 text-sm text-gray-800 dark:text-gray-200" {...props}>{children}</ol>
  ),
  li: ({ children, ...props }: React.HTMLAttributes<HTMLLIElement>) => (
    <li className="leading-relaxed" {...props}>{children}</li>
  ),
  strong: ({ children, ...props }: React.HTMLAttributes<HTMLElement>) => (
    <strong className="font-semibold text-gray-900 dark:text-gray-100" {...props}>{children}</strong>
  ),
  code: ({ children, ...props }: React.HTMLAttributes<HTMLElement>) => (
    <code className="font-mono text-xs bg-gray-200 dark:bg-gray-700 text-gray-800 dark:text-gray-200 px-1.5 py-0.5 rounded" {...props}>{children}</code>
  ),
  pre: ({ children, ...props }: React.HTMLAttributes<HTMLPreElement>) => (
    <pre className="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg p-3 mb-3 overflow-x-auto text-xs font-mono text-gray-800 dark:text-gray-200" {...props}>{children}</pre>
  ),
  hr: (props: React.HTMLAttributes<HTMLHRElement>) => (
    <hr className="border-gray-200 dark:border-gray-700 my-4" {...props} />
  ),
}

interface AnalysisPageProps {
  currentId?: number
  previousId?: number
}

export function AnalysisPage({ currentId: initialCurrentId, previousId: initialPreviousId }: AnalysisPageProps) {
  const [scans, setScans] = useState<Scan[]>([])
  const [currentId, setCurrentId] = useState<number | null>(initialCurrentId ?? null)
  const [previousId, setPreviousId] = useState<number | null>(initialPreviousId ?? null)
  const [analysis, setAnalysis] = useState<SnapshotAnalysis | null>(null)
  const [globalStatus, setGlobalStatus] = useState<AnalysisGlobalStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [expandedToolCalls, setExpandedToolCalls] = useState<Set<number>>(new Set())
  const [expandedToolResults, setExpandedToolResults] = useState<Set<number>>(new Set())

  // Load all scans and pre-select from props or first two scans
  useEffect(() => {
    async function loadScans() {
      setLoading(true)
      try {
        const scansData = await api.getScans()
        setScans(scansData || [])
        if (scansData && scansData.length > 0) {
          if (!initialCurrentId) {
            setCurrentId(scansData[0].id)
          }
          if (!initialPreviousId && scansData.length > 1) {
            setPreviousId(scansData[1].id)
          }
        }
      } catch (err) {
        console.error('Failed to load scans:', err)
      }
      setLoading(false)
    }
    loadScans()
  }, [initialCurrentId, initialPreviousId])

  // Load analysis when snapshot pair changes
  const loadAnalysis = useCallback(async () => {
    if (!currentId || !previousId) return
    try {
      const data = await api.getAnalysis(currentId, previousId)
      setAnalysis(data)
    } catch (err) {
      console.error('Failed to load analysis:', err)
    }
  }, [currentId, previousId])

  useEffect(() => {
    if (currentId && previousId) {
      loadAnalysis()
    }
  }, [currentId, previousId, loadAnalysis])

  // Load global status
  const loadGlobalStatus = useCallback(async () => {
    try {
      const data = await api.getAnalysisStatus()
      setGlobalStatus(data)
    } catch (err) {
      console.error('Failed to load analysis status:', err)
    }
  }, [])

  useEffect(() => {
    loadGlobalStatus()
  }, [loadGlobalStatus])

  // Poll analysis when it's pending or running
  useEffect(() => {
    if (!analysis || (analysis.status !== 'pending' && analysis.status !== 'running')) {
      return
    }

    const interval = setInterval(() => {
      loadAnalysis()
      loadGlobalStatus()
    }, 3000)

    return () => clearInterval(interval)
  }, [analysis, loadAnalysis, loadGlobalStatus])

  async function handleStartAnalysis() {
    if (!currentId || !previousId) return
    try {
      const result = await api.startAnalysis(currentId, previousId)
      setAnalysis(result)
      loadGlobalStatus()
    } catch (err) {
      console.error('Failed to start analysis:', err)
    }
  }

  async function handleRegenerate() {
    if (!currentId || !previousId) return
    try {
      await api.deleteAnalysis(currentId, previousId)
      await handleStartAnalysis()
    } catch (err) {
      console.error('Failed to regenerate analysis:', err)
    }
  }

  function toggleToolCalls() {
    if (expandedToolCalls.size > 0) {
      setExpandedToolCalls(new Set())
    } else {
      setExpandedToolCalls(new Set([0]))
    }
  }

  function toggleToolResult(index: number) {
    const newSet = new Set(expandedToolResults)
    if (newSet.has(index)) {
      newSet.delete(index)
    } else {
      newSet.add(index)
    }
    setExpandedToolResults(newSet)
  }

  if (loading) return <Loading />

  if (scans.length === 0) {
    return (
      <EmptyState
        title="No scans yet"
        description="Run at least two scans to compare snapshots"
      />
    )
  }

  const canAnalyze = currentId && previousId && currentId !== previousId && !globalStatus?.running
  const isCurrentPairRunning = globalStatus?.running &&
    globalStatus.current_snapshot_id === currentId &&
    globalStatus.previous_snapshot_id === previousId

  return (
    <div className="space-y-6">
      <Breadcrumb
        items={[
          { label: 'Scans', onClick: () => navigate({ page: 'scans' }) },
          { label: 'AI Analysis' }
        ]}
      />

      <div className="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg p-6 space-y-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Snapshot Selection</h2>

        <div className="flex items-end gap-4 flex-wrap">
          <div className="flex-1 min-w-64">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Current Snapshot
            </label>
            <Select
              value={currentId || ''}
              onChange={(e) => setCurrentId(Number(e.target.value))}
              aria-label="Select current snapshot"
            >
              {scans.map((scan) => (
                <option key={scan.id} value={scan.id}>
                  Snapshot #{scan.id} - {formatDate(scan.collected_at)}
                </option>
              ))}
            </Select>
          </div>

          <div className="flex-1 min-w-64">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Previous Snapshot
            </label>
            <Select
              value={previousId || ''}
              onChange={(e) => setPreviousId(Number(e.target.value))}
              aria-label="Select previous snapshot"
            >
              {scans.map((scan) => (
                <option key={scan.id} value={scan.id}>
                  Snapshot #{scan.id} - {formatDate(scan.collected_at)}
                </option>
              ))}
            </Select>
          </div>

          <Button
            onClick={handleStartAnalysis}
            disabled={!canAnalyze}
            aria-label="Start analysis"
          >
            Analyze
          </Button>
        </div>

        {globalStatus?.running && (
          <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
            <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" aria-hidden="true" />
            <span aria-live="polite">{globalStatus.progress || 'Running analysis...'}</span>
          </div>
        )}
      </div>

      {analysis && (
        <div className="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Analysis Result</h2>
            <StatusBadge status={analysis.status} progress={isCurrentPairRunning ? globalStatus?.progress : undefined} />
          </div>

          <div className="space-y-2 text-sm text-gray-500 dark:text-gray-400">
            <div>Created: {formatDate(analysis.created_at)}</div>
            {analysis.completed_at && <div>Completed: {formatDate(analysis.completed_at)}</div>}
          </div>

          {analysis.status === 'completed' && analysis.result && (
            <div className="bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-5">
              <Markdown components={markdownComponents}>{analysis.result}</Markdown>
            </div>
          )}

          {analysis.status === 'failed' && analysis.error && (
            <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
              <p className="text-sm text-red-800 dark:text-red-200 font-mono">{analysis.error}</p>
            </div>
          )}

          {analysis.tool_calls && analysis.tool_calls.length > 0 && (
            <div className="border-t border-gray-200 dark:border-gray-700 pt-4">
              <button
                onClick={toggleToolCalls}
                className="text-sm font-medium text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-gray-100 transition-colors focus:outline-none focus:ring-2 focus:ring-gray-300 dark:focus:ring-gray-600 rounded px-2 py-1"
              >
                {expandedToolCalls.size > 0 ? '▼' : '▶'} Tool Calls ({analysis.tool_calls.length})
              </button>

              {expandedToolCalls.size > 0 && (
                <div className="mt-4 space-y-3">
                  {analysis.tool_calls.map((call, i) => (
                    <div key={i} className="bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4 space-y-2">
                      <div className="font-semibold text-gray-900 dark:text-gray-100">{call.name}</div>
                      <div>
                        <div className="text-xs text-gray-500 dark:text-gray-400 mb-1">Arguments:</div>
                        <pre className="text-xs font-mono text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-900 p-2 rounded border border-gray-200 dark:border-gray-700 overflow-x-auto">
                          {JSON.stringify(call.args, null, 2)}
                        </pre>
                      </div>
                      {call.result !== undefined && (
                        <div>
                          <button
                            onClick={() => toggleToolResult(i)}
                            className="text-xs text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 transition-colors focus:outline-none"
                          >
                            {expandedToolResults.has(i) ? '▼' : '▶'} Result
                          </button>
                          {expandedToolResults.has(i) && (
                            <pre className="mt-1 text-xs font-mono text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-900 p-2 rounded border border-gray-200 dark:border-gray-700 overflow-x-auto">
                              {JSON.stringify(call.result, null, 2)}
                            </pre>
                          )}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {(analysis.status === 'completed' || analysis.status === 'failed') && (
            <div className="flex justify-end pt-2">
              <Button
                variant="ghost"
                onClick={handleRegenerate}
                disabled={globalStatus?.running}
                aria-label="Regenerate analysis"
              >
                Regenerate
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

interface StatusBadgeProps {
  status: AnalysisStatusType
  progress?: string
}

function StatusBadge({ status, progress }: StatusBadgeProps) {
  const styles: Record<AnalysisStatusType, string> = {
    pending: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
    running: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-200',
    completed: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-200',
    failed: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-200'
  }

  const labels: Record<AnalysisStatusType, string> = {
    pending: 'Pending',
    running: 'Running',
    completed: 'Completed',
    failed: 'Failed'
  }

  return (
    <div className={`inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-medium ${styles[status]}`}>
      {status === 'running' && <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" aria-hidden="true" />}
      <span>{labels[status]}</span>
      {status === 'running' && progress && <span className="text-xs opacity-75">- {progress}</span>}
    </div>
  )
}