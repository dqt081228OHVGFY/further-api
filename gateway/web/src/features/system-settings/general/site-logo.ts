import * as z from 'zod'

/** Site logos may be remote URLs or assets served from this gateway. */
export function isValidSiteLogo(value: string): boolean {
  if (value === '') return true
  if (value.startsWith('/')) {
    return !value.startsWith('//') && !/[\\\s]/.test(value)
  }
  return z.url().safeParse(value).success
}
