import { useEffect, useState } from 'react'
import { type Route, parseRoute } from './lib/router'
import { useScanStatus } from './hooks/useScanStatus'
import { Header } from './components/Header'
import { Router } from './components/Router'

function App() {
  const [route, setRoute] = useState<Route>(parseRoute)
  const { scanStatus, triggerScan } = useScanStatus()

  useEffect(() => {
    const onHashChange = () => setRoute(parseRoute())
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  return (
    <div className="min-h-screen bg-white dark:bg-gray-900 transition-colors">
      <Header />
      <main className="max-w-6xl mx-auto px-6 py-8">
        <Router route={route} scanStatus={scanStatus} onScan={triggerScan} />
      </main>
    </div>
  )
}

export default App
