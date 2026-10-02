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

import { normalizeNativeFeaturePath } from './native-feature-paths'

const consoleRoots = new Set([
  'chat',
  'chat2link',
  'keys',
  'playground',
  'profile',
  'security',
  'wallet',
])

const adminRoots = new Set([
  'channels',
  'models',
  'redemption-codes',
  'risk-control',
  'subscriptions',
  'system-info',
  'system-settings',
  'task-plugins',
  'users',
])

const originalPanels: Record<string, string> = {
  '/keys': '/console/#tokens',
  '/profile': '/console/#profile',
  '/security': '/console/#profile',
  '/channels': '/admin/#channels',
  '/users': '/admin/#users',
  '/redemption-codes': '/admin/#redemptions',
  '/models': '/admin/#models',
  '/subscriptions': '/admin/#subscriptions',
  '/risk-control': '/admin/#risk',
  '/system-settings': '/admin/#settings',
}

export function activityShellTarget(path: string): string | null {
  const plainPath = path.split('?')[0].replace(/\/+$/, '')
  if (
    plainPath === '/activities' ||
    plainPath === '/activity' ||
    plainPath === '/benefits' ||
    plainPath === '/checkin' ||
    plainPath === '/profile/checkin'
  ) {
    return '/console/#activities'
  }
  if (plainPath === '/system-settings/billing/checkin') {
    return '/admin/#activities'
  }
  const festival = plainPath.match(
    /^\/activities\/(duanwu|qixi|mid_autumn|chongyang|new_year_eve|national_day)$/
  )
  return festival ? `/console/#activities/${festival[1]}` : null
}

export function originalShellTarget(path: string, role: number): string {
  const fallback = role >= 10 ? '/admin/#overview' : '/console/#overview'
  const activityTarget = activityShellTarget(path)
  if (activityTarget) return activityTarget
  let canonical: string
  try {
    canonical = normalizeNativeFeaturePath(path)
  } catch {
    return fallback
  }

  const pathname = canonical.split('?')[0]
  if (pathname === '/system-settings/billing/payment') {
    return '/admin/#settings/payment'
  }
  if (pathname === '/wallet' && !canonical.includes('?')) {
    return '/console/#vip/payment'
  }
  if (
    (pathname === '/dashboard' || pathname === '/dashboard/overview') &&
    !canonical.includes('?')
  ) {
    return fallback
  }
  if (!canonical.includes('?') && originalPanels[pathname]) {
    return originalPanels[pathname]
  }

  const root = pathname.split('/')[1]
  let base = fallback.slice(0, fallback.indexOf('#'))
  if (consoleRoots.has(root)) base = '/console/'
  if (adminRoots.has(root)) base = '/admin/'
  if (base === '/console/') {
    if (root === 'keys') return '/console/#tokens'
    if (root === 'profile' || root === 'security') return '/console/#profile'
    if (root === 'wallet') return '/console/#vip/payment'
    if (root === 'usage-logs') return '/console/#usage'
    if (pathname === '/dashboard/models' || pathname === '/dashboard/flow') {
      return '/console/#usage'
    }
    return '/console/#overview'
  }
  if (pathname === '/users') return '/admin/#users'
  return `${base}#native/${encodeURIComponent(canonical)}`
}
