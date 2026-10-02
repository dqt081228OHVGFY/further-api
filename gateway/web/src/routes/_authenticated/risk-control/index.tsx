/*
Copyright (C) 2023-2026 QuantumNous
This file is part of a modified New API distribution under AGPLv3.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { RiskControlPage } from '@/features/risk-control'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/risk-control/')({
  beforeLoad: () => {
    const user = useAuthStore.getState().auth.user
    if (!user || user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: RiskControlPage,
})
