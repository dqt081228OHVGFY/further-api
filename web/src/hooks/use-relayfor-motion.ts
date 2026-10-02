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
import { useLayoutEffect, useSyncExternalStore } from 'react'

const STORAGE_KEY = 'relayfor-motion'
const CHANGE_EVENT = 'relayfor:motion-change'
type MotionPreference = 'on' | 'off'

function storedPreference(): MotionPreference | null {
  try {
    const value = window.localStorage.getItem(STORAGE_KEY)
    return value === 'on' || value === 'off' ? value : null
  } catch {
    return null
  }
}

function syncDocumentPreference() {
  const preference = storedPreference()
  if (preference) {
    document.documentElement.dataset.relayforMotion = preference
  } else {
    delete document.documentElement.dataset.relayforMotion
  }
}

export function relayForMotionEnabled(): boolean {
  if (typeof window === 'undefined') return true
  const documentPreference = document.documentElement.dataset.relayforMotion
  const preference =
    documentPreference === 'on' || documentPreference === 'off'
      ? documentPreference
      : storedPreference()
  if (preference) return preference === 'on'
  return !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
}

export function setRelayForMotionEnabled(enabled: boolean) {
  const preference: MotionPreference = enabled ? 'on' : 'off'
  try {
    window.localStorage.setItem(STORAGE_KEY, preference)
  } catch {
    // The current page can still honor the preference without storage.
  }
  document.documentElement.dataset.relayforMotion = preference
  window.dispatchEvent(new Event(CHANGE_EVENT))
}

function subscribe(listener: () => void) {
  window.addEventListener(CHANGE_EVENT, listener)
  const media = window.matchMedia?.('(prefers-reduced-motion: reduce)')
  media?.addEventListener?.('change', listener)
  return () => {
    window.removeEventListener(CHANGE_EVENT, listener)
    media?.removeEventListener?.('change', listener)
  }
}

export function useRelayForMotionEnabled() {
  const enabled = useSyncExternalStore(
    subscribe,
    relayForMotionEnabled,
    () => true
  )
  useLayoutEffect(syncDocumentPreference, [])
  return enabled
}
