/*
Copyright (C) 2023-2026 QuantumNous
This file is part of a modified New API distribution under AGPLv3.
*/
import { toIntlLocale } from '@/i18n/languages'

import type { RiskUser } from '../types'

export function formatRiskTimestamp(
  value: RiskUser['last_request_at'],
  locale: string
): string {
  if (!value) return '—'
  const timestamp = typeof value === 'number' ? value * 1000 : Date.parse(value)
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '—'
  return new Intl.DateTimeFormat(toIntlLocale(locale), {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(timestamp)
}
