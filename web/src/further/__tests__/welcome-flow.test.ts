import { describe, expect, it } from 'vitest'
import type { AuthUser } from '@/stores/auth-store'
import { authenticatedDestination, isNewWorkspaceUser, postLoginDestination, welcomeDestination } from '../welcome-flow'
import { managementDestination } from '../workspace-target'

const user: AuthUser = { id: 24, username: 'new-member', role: 1, request_count: 0, used_quota: 0, welcome_completed: false }

describe('new-user welcome navigation', () => {
  it('preserves the first requested console destination through the welcome page', () => {
    expect(postLoginDestination(user, '/console/#tokens')).toBe('/welcome?next=%2Fconsole%2F%23tokens')
    history.replaceState(null, '', '/welcome?next=%2Fconsole%2F%23tokens')
    expect(welcomeDestination()).toBe('/console/#tokens')
  })
  it('keeps completed and legacy accounts on their requested page', () => {
    for (const state of [{ ...user, welcome_completed: true }, { ...user, welcome_completed: undefined }]) {
      expect(isNewWorkspaceUser(state)).toBe(false)
      expect(postLoginDestination(state, '/console/#docs')).toBe('/console/#docs')
    }
    expect(postLoginDestination(user, '/pricing')).toBe('/pricing')
  })
  it('welcomes administrators once and preserves their management destination', () => {
    const administrator = { ...user, role: 100, request_count: 30, used_quota: 80 }
    expect(postLoginDestination(administrator, '/admin/#risk')).toBe('/welcome?next=%2Fadmin%2F%23risk')
    expect(postLoginDestination({ ...administrator, welcome_completed: true }, '/admin/#risk')).toBe('/admin/#risk')
  })
  it('rejects remote and recursive completion destinations', () => {
    for (const next of ['https://outside.invalid/', '//outside.invalid/', '/welcome?next=/welcome', '/welcome/']) {
      history.replaceState(null, '', `/welcome?next=${encodeURIComponent(next)}`)
      expect(welcomeDestination()).toBe('/console/#overview')
    }
    expect(postLoginDestination(user, '//outside.invalid/')).toBe('/welcome?next=%2Fconsole%2F')
  })
  it('uses one role-aware entry for native login, OAuth and welcome completion', () => {
    for (const role of [1, 10, 50, 100]) {
      const account = { ...user, role, welcome_completed: true }
      const expected = role >= 10 ? '/admin/' : '/console/'
      expect(managementDestination(role)).toBe(expected)
      expect(postLoginDestination(account)).toBe(expected)
      history.replaceState(null, '', '/welcome')
      expect(welcomeDestination(account)).toBe(`${expected}#overview`)
    }
  })
  it('canonicalizes legacy feature redirects and rejects privileged and backend destinations', () => {
    const completed = { ...user, welcome_completed: true }
    expect(authenticatedDestination(completed, '/keys')).toBe('/console/#tokens')
    expect(authenticatedDestination(completed, '/security')).toBe('/console/#profile')
    for (const path of ['/admin/#channels', '/channels', '/system-settings/security/ssrf']) {
      expect(authenticatedDestination(completed, path)).toBe('/console/#overview')
    }
    for (const path of ['/api/user/self', '/v1/models', '/sign-in?redirect=/console/', '/auth/', '/welcome', '/oauth/linuxdo']) {
      expect(authenticatedDestination(completed, path)).toBe('/console/')
    }
    const root = { ...completed, role: 100 }
    expect(authenticatedDestination(root, '/subscriptions')).toBe('/admin/#subscriptions/paid')
    expect(authenticatedDestination(root, '/usage-logs/common')).toBe('/admin/#logs')
    expect(authenticatedDestination(root, '/usage-logs/common?from=saved')).toBe('/admin/#logs')
    expect(authenticatedDestination(root, '/dashboard/models?from=saved')).toBe('/admin/#overview/analysis')
    expect(authenticatedDestination(root, '/system-settings/site/notice?from=saved')).toBe('/admin/#settings/announcements')
  })
})
