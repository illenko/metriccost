import { useState, useMemo, type ReactNode } from 'react'

const PAGE_SIZE = 25

export interface Column<T> {
  key: string
  header: string
  align?: 'left' | 'right'
  render: (item: T) => ReactNode
}

interface DataTableProps<T> {
  columns: Column<T>[]
  data: T[]
  keyExtractor: (item: T) => string | number
  onRowClick?: (item: T) => void
}

export function DataTable<T>({ columns, data, keyExtractor, onRowClick }: DataTableProps<T>) {
  const [page, setPage] = useState(0)

  const totalPages = Math.max(1, Math.ceil(data.length / PAGE_SIZE))

  // Reset to first page when data changes
  const safeePage = page >= totalPages ? 0 : page
  if (safeePage !== page) setPage(safeePage)

  const pageData = useMemo(
    () => data.slice(safeePage * PAGE_SIZE, (safeePage + 1) * PAGE_SIZE),
    [data, safeePage]
  )

  const showPagination = data.length > PAGE_SIZE

  return (
    <div className="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
      <table className="w-full">
        <thead className="bg-gray-50 dark:bg-gray-800">
          <tr>
            {columns.map((col) => (
              <th
                key={col.key}
                scope="col"
                className={`px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase ${
                  col.align === 'right' ? 'text-right' : 'text-left'
                }`}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-900">
          {pageData.map((item) => (
            <tr
              key={keyExtractor(item)}
              onClick={onRowClick ? () => onRowClick(item) : undefined}
              className={onRowClick ? 'hover:bg-gray-50 dark:hover:bg-gray-800 cursor-pointer transition-colors' : ''}
              role={onRowClick ? 'button' : undefined}
              tabIndex={onRowClick ? 0 : undefined}
              onKeyDown={onRowClick ? (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  onRowClick(item)
                }
              } : undefined}
            >
              {columns.map((col) => (
                <td
                  key={col.key}
                  className={`px-4 py-3 text-sm ${
                    col.align === 'right' ? 'text-right' : 'text-left'
                  }`}
                >
                  {col.render(item)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {showPagination && (
        <div className="flex items-center justify-between px-4 py-3 bg-gray-50 dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700">
          <span className="text-xs text-gray-500 dark:text-gray-400">
            {safeePage * PAGE_SIZE + 1}–{Math.min((safeePage + 1) * PAGE_SIZE, data.length)} of {data.length}
          </span>
          <div className="flex items-center gap-1">
            <button
              onClick={() => setPage(0)}
              disabled={safeePage === 0}
              className="px-2 py-1 text-xs text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-700 rounded disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              aria-label="First page"
            >
              &laquo;
            </button>
            <button
              onClick={() => setPage(safeePage - 1)}
              disabled={safeePage === 0}
              className="px-2 py-1 text-xs text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-700 rounded disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              aria-label="Previous page"
            >
              &lsaquo;
            </button>
            <span className="px-2 py-1 text-xs font-medium text-gray-700 dark:text-gray-300">
              {safeePage + 1} / {totalPages}
            </span>
            <button
              onClick={() => setPage(safeePage + 1)}
              disabled={safeePage >= totalPages - 1}
              className="px-2 py-1 text-xs text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-700 rounded disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              aria-label="Next page"
            >
              &rsaquo;
            </button>
            <button
              onClick={() => setPage(totalPages - 1)}
              disabled={safeePage >= totalPages - 1}
              className="px-2 py-1 text-xs text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-700 rounded disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              aria-label="Last page"
            >
              &raquo;
            </button>
          </div>
        </div>
      )}
    </div>
  )
}