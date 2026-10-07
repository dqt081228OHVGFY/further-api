import { sanitizeAuthRedirect } from '@/features/auth/lib/auth-redirect'
import type { AuthUser } from '@/stores/auth-store'
import { managementDestination, workspaceShellTarget } from './workspace-target'

export function isNewWorkspaceUser(user: AuthUser): boolean {
  return user.welcome_completed === false
}

export function postLoginDestination(user: AuthUser, requested?: string): string {
  const next = authenticatedDestination(user, requested)
  if (isNewWorkspaceUser(user) && /^\/(?:console|admin)(?:\/|$)/.test(next)) {
    return `/welcome?next=${encodeURIComponent(next)}`
  }
  return next
}

export function authenticatedDestination(user: Pick<AuthUser, 'role'>, requested?: string | null): string {
  const fallback = managementDestination(user.role)
  const safe = sanitizeAuthRedirect(requested, location.origin)
  if (!safe || /^\/(?:auth|sign-in|sign-up|welcome|setup|oauth|otp|api|v1|v1beta|mj|pg)(?:\/|\?|#|$)/.test(safe)) return fallback
  if (/^\/admin(?:\/|\?|#|$)/.test(safe) && Number(user.role) < 10) return `${fallback}#overview`
  if (/^\/(?:console|admin)(?:\/|\?|#|$)/.test(safe)) return safe
  const canonical = workspaceShellTarget(safe, Number(user.role))
  // Public pages remain valid redirect destinations; recognized feature roots
  // use their single Further editor instead of a legacy page.
  return /^\/(?:channels|chat|chat2link|dashboard|keys|models|playground|profile|redemption-codes|risk-control|security|subscriptions|system-info|system-settings|task-plugins|usage-logs|users|wallet|activities|activity|benefits|checkin)(?:\/|\?|#|$)/.test(safe) ? canonical : safe
}

export function welcomeDestination(user: Pick<AuthUser, 'role'> = { role: 1 }): string {
  const next = sanitizeAuthRedirect(new URLSearchParams(location.search).get('next'), location.origin)
  return next && !/^\/welcome(?:\/|\?|$)/.test(next) ? authenticatedDestination(user, next) : `${managementDestination(user.role)}#overview`
}
