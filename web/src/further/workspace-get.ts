import type { AxiosResponse } from 'axios'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

const pending = new Map<string, Promise<AxiosResponse>>()

/** Share concurrent reads, then discard them so the next refresh stays fresh. */
export function workspaceGet<T = unknown>(path: string, params?: Record<string, unknown>): Promise<AxiosResponse<T>> {
  const url = new URL(path, 'https://further.invalid')
  url.searchParams.sort()
  const parameters = params ? Object.entries(params).sort(([a], [b]) => a.localeCompare(b)) : []
  const key = JSON.stringify([useAuthStore.getState().auth.accessToken || '', `${url.pathname}${url.search}`, parameters])
  const existing = pending.get(key)
  if (existing) return existing as Promise<AxiosResponse<T>>
  const request = Promise.resolve().then(() => params ? api.get<T>(path, { params }) : api.get<T>(path)).finally(() => {
    if (pending.get(key) === request) pending.delete(key)
  })
  pending.set(key, request)
  return request
}
