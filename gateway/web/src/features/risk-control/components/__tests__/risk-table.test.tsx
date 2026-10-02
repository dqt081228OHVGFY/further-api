import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import type { RiskUser } from '../../types'
import { formatRiskTimestamp } from '../../lib/format'
import { RiskTable } from '../risk-table'

const row: RiskUser = {
  user_id: 7,
  username: 'preview-user',
  score: 83,
  level: 'high',
  status: 'unreviewed',
  signals: {
    regular_timing: 76,
    repeated_input: 91,
    multi_model: 20,
    post_402: 0,
    non_stream: 88,
    night_activity: 18,
    post_channel_unavailable: 0,
    blocked_liveness: 45,
    multi_ip: 12,
  },
  request_count: 42,
  ip_count: 2,
  last_request_at: 0,
}

beforeEach(async () => {
  await i18next.changeLanguage('en')
})

describe('risk table', () => {
  it('shows an empty state when no users match the filters', () => {
    render(<RiskTable rows={[]} loading={false} updatingUserId={null} onDetails={vi.fn()} onStatus={vi.fn()} />)
    expect(screen.getByText('No risk records yet')).toBeInTheDocument()
  })

  it('shows nine signal scores and lets an admin review a user', async () => {
    const onDetails = vi.fn()
    const onStatus = vi.fn()
    render(<RiskTable rows={[row]} loading={false} updatingUserId={null} onDetails={onDetails} onStatus={onStatus} />)

    expect(screen.getByText('preview-user')).toBeInTheDocument()
    expect(screen.getByLabelText('Risk score: 83')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'View risk details' }))
    await userEvent.click(screen.getByRole('button', { name: 'Watch account' }))
    expect(onDetails).toHaveBeenCalledWith(7)
    expect(onStatus).toHaveBeenCalledWith(7, 'watching')
  })

  it('uses UTC+8 for a valid timestamp and handles missing timestamps', () => {
    expect(formatRiskTimestamp(0, 'zhCN')).toBe('—')
    expect(formatRiskTimestamp(1_725_000_000, 'en')).toContain('2024')
  })
})
