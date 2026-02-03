import type { Route } from '../lib/router'
import type { ScanStatus } from '../api'
import { ScansPage } from '../pages/ScansPage'
import { ServicesPage } from '../pages/ServicesPage'
import { MetricsPage } from '../pages/MetricsPage'
import { LabelsPage } from '../pages/LabelsPage'
import { AnalysisPage } from '../pages/AnalysisPage'

interface RouterProps {
  route: Route
  scanStatus: ScanStatus | null
  onScan: () => void
}

export function Router({ route, scanStatus, onScan }: RouterProps) {
  switch (route.page) {
    case 'scans':
      return <ScansPage scanStatus={scanStatus} onScan={onScan} />
    case 'services':
      return <ServicesPage scanId={route.scanId} />
    case 'metrics':
      return <MetricsPage scanId={route.scanId} serviceName={route.serviceName} />
    case 'labels':
      return <LabelsPage scanId={route.scanId} serviceName={route.serviceName} metricName={route.metricName} />
    case 'analysis':
      return <AnalysisPage currentId={route.currentId} previousId={route.previousId} />
    default:
      // TypeScript exhaustiveness check
      const _exhaustiveCheck: never = route
      return _exhaustiveCheck
  }
}
