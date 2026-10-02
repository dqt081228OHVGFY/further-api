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

import '@/styles/relayfor-console.css'

import '@/styles/relayfor-extension.css'

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
  useEffect(() => {
    document.body.classList.add('relayfor-console-active')
    return () => {
      document.body.classList.remove('relayfor-console-active')
    }
  }, [])

  return (
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider className='relayfor-extension'>
          {props.children ?? <RelayforOutlet />}
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
  )
}
