import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export type WorkspaceBootstrap = {
  schema_version: number
  setup: { status: boolean; root_init: boolean; database_type: string }
  status: Record<string, unknown>
  authenticated: boolean
  user: Record<string, unknown> | null
  groups: Record<string, unknown>
}

let pending: Promise<WorkspaceBootstrap> | undefined

export function loadWorkspaceBootstrap(): Promise<WorkspaceBootstrap> {
  pending ??= api.get('/api/further/workspace/bootstrap').then((response) => {
    const data = requireServerSuccess(response.data).data as WorkspaceBootstrap
    if (data.schema_version !== 1 || !data.setup || !data.status || !data.groups) {
      throw new Error('工作区初始化数据无效，请重新载入。')
    }
    return data
  }).catch((cause: unknown) => { pending = undefined; throw cause })
  return pending
}
