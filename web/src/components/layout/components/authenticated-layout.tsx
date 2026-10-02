/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Outlet, useRouterState } from '@tanstack/react-router'
import { motion } from 'motion/react'
import { useEffect } from 'react'

import { SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { useRelayForMotionEnabled } from '@/hooks/use-relayfor-motion'
import { useNativeFeatureIsland } from '@/relayfor/native-feature-context'
import { originalShellTarget } from '@/relayfor/original-shell-target'
import { useAuthStore } from '@/stores/auth-store'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

function RelayforOutlet() {
  const motionEnabled = useRelayForMotionEnabled()
  const routeKey = useRouterState({
    select: (state) => state.location.pathname,
  })

  return (
    <motion.div
      key={routeKey}
      initial={motionEnabled ? { opacity: 0 } : false}
      animate={{ opacity: 1 }}
      transition={{ duration: motionEnabled ? 0.3 : 0, ease: [0, 0, 0, 1] }}
      className='flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'
    >
      <Outlet />
    </motion.div>
  )
}

export function AuthenticatedLayout(props: AuthenticatedLayoutProps) {
  const user = useAuthStore((state) => state.auth.user)
  const nativeFeature = useNativeFeatureIsland()

  useEffect(() => {
    if (!nativeFeature)
      location.replace(
        originalShellTarget(
          location.pathname + location.search,
          Number(user?.role || 0)
        )
      )
  }, [nativeFeature, user?.role])

  return (
    <LayoutProvider>
      <SearchProvider globalShortcut={!nativeFeature}>
        <SidebarProvider className='relayfor-native-content'>
          {nativeFeature ? (props.children ?? <RelayforOutlet />) : null}
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
  )
}
