/*
Copyright (C) 2023-2026 QuantumNous
This file is part of a modified New API distribution under AGPLv3.
*/

export const RISK_SIGNAL_KEYS = [
  'regular_timing',
  'repeated_input',
  'multi_model',
  'post_402',
  'non_stream',
  'night_activity',
  'post_channel_unavailable',
  'blocked_liveness',
  'multi_ip',
] as const

export type RiskSignalKey = (typeof RISK_SIGNAL_KEYS)[number]
export const RISK_SIGNAL_LABELS: Record<RiskSignalKey, string> = {
  regular_timing: 'Regular timing',
  repeated_input: 'Repeated input',
  multi_model: 'Multiple models',
  post_402: 'Requests after 402',
  non_stream: 'Non-stream requests',
  night_activity: 'Night activity',
  post_channel_unavailable: 'Requests after channel unavailable',
  blocked_liveness: 'Blocked liveness probes',
  multi_ip: 'Multiple IPs',
}
export type RiskLevel = 'low' | 'medium' | 'high' | 'critical'
export type RiskStatus = 'unreviewed' | 'watching' | 'confirmed' | 'dismissed'
export type RiskWindow = '3d' | '7d'

export interface RiskUser {
  user_id: number
  username: string
  score: number
  level: RiskLevel
  status: RiskStatus
  signals: Record<RiskSignalKey, number>
  request_count: number
  ip_count: number
  last_request_at: number | string | null
}

export interface RiskUserDetail extends RiskUser {
  evidence: Record<string, number | string | boolean | null>
}

export interface RiskUserList {
  items: RiskUser[]
  total: number
  page: number
  page_size: number
  window: RiskWindow
}

export interface RiskUserFilters {
  window: RiskWindow
  page: number
  page_size: number
  username: string
  level: string
  status: string
}
