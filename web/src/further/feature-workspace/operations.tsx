import { MonitorOperations } from './monitor-operations'
import { PluginOperations } from './plugin-operations'

export function OperationsScreen({ pathname }: { pathname: string }) {
  if (pathname === '/task-plugins') return <PluginOperations />
  return <MonitorOperations />
}
