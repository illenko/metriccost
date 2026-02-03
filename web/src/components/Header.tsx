import { navigate } from '../lib/router'

export function Header() {
  return (
    <header className="border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900">
      <div className="max-w-6xl mx-auto px-6 py-4 flex items-center justify-between">
        <button
          onClick={() => navigate({ page: 'scans' })}
          className="text-xl font-semibold text-gray-900 dark:text-gray-100 hover:text-gray-700 dark:hover:text-gray-300 transition-colors"
          aria-label="Go to home page"
        >
          🤔 WhoDidThis?
        </button>
        <div className="flex items-center gap-2">
          <button
            onClick={() => navigate({ page: 'scans' })}
            className="px-3 py-1.5 rounded-lg text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
            aria-label="Scans"
          >
            📡 Scans
          </button>
          <button
            onClick={() => navigate({ page: 'analysis' })}
            className="px-3 py-1.5 rounded-lg text-sm font-medium bg-gradient-to-r from-violet-500 to-fuchsia-500 text-white hover:from-violet-600 hover:to-fuchsia-600 shadow-sm shadow-violet-500/25 transition-all"
            aria-label="AI Analysis"
          >
            ✨ AI Analysis
          </button>
        </div>
      </div>
    </header>
  )
}
