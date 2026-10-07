import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { workspaceGet } from '../workspace-get'

export type Row = Record<string, unknown>

export function asRow(value: unknown): Row {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Row
    : {}
}

export function asRows(value: unknown): Row[] {
  return Array.isArray(value) ? value.map(asRow) : []
}

export function label(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

export async function read(path: string, params?: Record<string, unknown>): Promise<unknown> {
  const response = await workspaceGet<{ data: unknown }>(path, params)
  return requireServerSuccess(response.data).data
}

export async function write(path: string, method: 'POST' | 'PUT' | 'PATCH' | 'DELETE', data?: unknown): Promise<unknown> {
  const response = await api.request({ url: path, method, data })
  return requireServerSuccess(response.data).data
}

export type PageData = { items: Row[]; total: number }

export function pageData(value: unknown): PageData {
  if (Array.isArray(value)) return { items: asRows(value), total: value.length }
  const record = asRow(value)
  const items = asRows(record.items ?? record.data ?? record.records)
  const total = Number(record.total)
  return { items, total: Number.isFinite(total) && total >= 0 ? total : items.length }
}
