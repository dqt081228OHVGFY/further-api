import { describe, expect, it } from 'vitest'

import { isValidSiteLogo } from '../site-logo'

describe('site logo setting', () => {
  it('accepts the built-in gateway icon and a remote image URL', () => {
    expect(isValidSiteLogo('/relayfor-logo.svg')).toBe(true)
    expect(isValidSiteLogo('https://example.com/logo.svg')).toBe(true)
  })

  it('accepts an empty value and rejects protocol-relative or malformed paths', () => {
    expect(isValidSiteLogo('')).toBe(true)
    expect(isValidSiteLogo('//example.com/logo.svg')).toBe(false)
    expect(isValidSiteLogo('/bad\\path.svg')).toBe(false)
    expect(isValidSiteLogo('/bad path.svg')).toBe(false)
  })
})
