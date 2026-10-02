/*
Copyright (C) 2023-2026 QuantumNous
This file is part of a modified New API distribution under AGPLv3.
*/
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  RiskStatus,
  RiskUserDetail,
  RiskUserFilters,
  RiskUserList,
  RiskWindow,
} from './types'

interface RiskResponse<T> {
  success: boolean
  data: T
  message?: string
}

export async function listRiskUsers(filters: RiskUserFilters): Promise<RiskUserList> {
  const response = await api.get<RiskResponse<RiskUserList>>('/api/risk/users', {
    params: filters,
  })
  return requireServerSuccess(response.data).data
}

export async function getRiskUserDetail(
  userId: number,
  window: RiskWindow
): Promise<RiskUserDetail> {
  const response = await api.get<RiskResponse<RiskUserDetail>>(
    `/api/risk/users/${userId}`,
    { params: { window } }
  )
  return requireServerSuccess(response.data).data
}

export async function updateRiskUserStatus(
  userId: number,
  status: RiskStatus
): Promise<void> {
  const response = await api.patch<RiskResponse<{ user_id: number; status: RiskStatus }>>(
    `/api/risk/users/${userId}/status`,
    { status }
  )
  requireServerSuccess(response.data)
}
