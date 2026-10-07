import { constants, createDecipheriv, generateKeyPairSync, privateDecrypt, webcrypto } from 'node:crypto'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { clearPasswordEncryptionCache, encryptPassword } from '../password-encryption'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get } }))
vi.mock('i18next', () => ({ t: (value: string) => value }))

const { privateKey, publicKey } = generateKeyPairSync('rsa', { modulusLength: 2048 })
const kid = 'local-password-test'
const pem = publicKey.export({ type: 'spki', format: 'pem' }).toString()

describe('native password encryption', () => {
  beforeEach(() => {
    clearPasswordEncryptionCache()
    vi.stubGlobal('crypto', webcrypto)
    get.mockReset()
    get.mockResolvedValue({ data: { success: true, data: { kid, public_key: pem } } })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('uses the empty OAEP label expected by the gateway for a short UTF-8 password', async () => {
    const password = '本地安全验证-123'
    const result = await encryptPassword(password)
    const decoded = privateDecrypt({ key: privateKey, padding: constants.RSA_PKCS1_OAEP_PADDING, oaepHash: 'sha256' }, Buffer.from(result.password_encrypted, 'base64'))
    expect(decoded.toString('utf8')).toBe(password)
    expect(result.encryption_key_id).toBe(kid)
    expect(result).not.toHaveProperty('password')
  })

  it('binds long password AES-GCM ciphertext to the gateway key id', async () => {
    const password = '本地长密码🔒'.repeat(40)
    const result = await encryptPassword(password)
    const [version, wrapped, nonce, ciphertext] = result.password_encrypted.split('.')
    expect(version).toBe('v2')
    const secret = privateDecrypt({ key: privateKey, padding: constants.RSA_PKCS1_OAEP_PADDING, oaepHash: 'sha256', oaepLabel: Buffer.from('password-v2') }, Buffer.from(wrapped, 'base64'))
    const body = Buffer.from(ciphertext, 'base64')
    function decrypt(keyId: string) {
      const decipher = createDecipheriv('aes-256-gcm', secret, Buffer.from(nonce, 'base64'))
      decipher.setAAD(Buffer.from(`password-v2:${keyId}`))
      decipher.setAuthTag(body.subarray(-16))
      return Buffer.concat([decipher.update(body.subarray(0, -16)), decipher.final()]).toString('utf8')
    }
    expect(decrypt(kid)).toBe(password)
    expect(() => decrypt('another-key-id')).toThrow()
  })

  it('uses fresh cryptographic randomness for repeated submissions', async () => {
    const first = await encryptPassword('same-local-password')
    const second = await encryptPassword('same-local-password')
    expect(first.password_encrypted).not.toBe(second.password_encrypted)
    expect(get).toHaveBeenCalledTimes(1)
  })

  it('rejects an insecure environment without sending a request or a plaintext fallback', async () => {
    vi.stubGlobal('crypto', {})
    await expect(encryptPassword('not-transmitted')).rejects.toThrow('HTTPS')
    expect(get).not.toHaveBeenCalled()
  })
})
