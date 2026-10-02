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
import { AnimatePresence, motion } from 'motion/react'
import { useTranslation } from 'react-i18next'

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
  SidebarTrigger,
} from '@/components/ui/sidebar'
import { useLayout } from '@/context/layout-provider'
import { useRelayForMotionEnabled } from '@/hooks/use-relayfor-motion'
import { useSidebarView } from '@/hooks/use-sidebar-view'

import { NavGroup } from './nav-group'
import { SidebarViewHeader } from './sidebar-view-header'
import { SystemBrand } from './system-brand'

/**
 * Application sidebar.
 *
 * Adopts the Vercel / Cloudflare "drill-in" pattern: the URL drives
 * which sidebar *view* is rendered. Clicking a top-level entry like
 * `System Settings` swaps the sidebar to a contextual workspace —
 * with a `← Back to Dashboard` affordance — instead of stacking the
 * sub-navigation inside the root tree.
 *
 * Architecture:
 *   - View resolution + filtering: {@link useSidebarView}
 *   - View registry: `layout/lib/sidebar-view-registry.ts`
 *   - Per-view header: {@link SidebarViewHeader}
 *
 * Adding a new nested view only requires registering a {@link SidebarView}
 * in the registry; this component requires no changes.
 */
export function AppSidebar() {
  const { t } = useTranslation()
  const { collapsible, variant } = useLayout()
  const { key, view, navGroups } = useSidebarView()
  const motionEnabled = useRelayForMotionEnabled()

  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      <SidebarHeader className='relayfor-console-brand'>
        <SystemBrand variant='sidebar' />
      </SidebarHeader>
      {view && <SidebarViewHeader view={view} />}

      <SidebarContent className='relayfor-console-nav'>
        <AnimatePresence mode='wait' initial={false}>
          <motion.div
            key={key}
            initial={motionEnabled ? { opacity: 0 } : false}
            animate={{ opacity: 1 }}
            exit={motionEnabled ? { opacity: 0 } : undefined}
            transition={{
              duration: motionEnabled ? 0.3 : 0,
              ease: [0, 0, 0, 1],
            }}
            className='flex flex-col'
          >
            {navGroups.map((props) => (
              <NavGroup key={props.id || props.title} {...props} />
            ))}
          </motion.div>
        </AnimatePresence>
      </SidebarContent>

      <SidebarFooter className='relayfor-console-footer'>
        <SidebarTrigger />
        <span>{t('Toggle Sidebar')}</span>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
