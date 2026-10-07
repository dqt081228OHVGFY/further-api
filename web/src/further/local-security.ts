import { checkVerificationMethods, verify } from '@/features/auth/secure-verification/api'
import type { SecurityProofScope, VerificationInput, VerificationOperation } from '@/features/auth/secure-verification/types'
import { beginPasskeyRegistration, finishPasskeyRegistration, deletePasskey } from '@/features/auth/passkey/api'
import { buildRegistrationResult, createCredential, prepareCredentialCreationOptions } from '@/lib/passkey'
import { setup2FA, enable2FA, createAccessToken, revokeAccessToken } from '@/features/security/api'
import { disable2FA, regenerate2FABackupCodes } from '@/lib/api'

// Business-only bridge. Further owns every form, dialog and page around it.
const security = {
  methods: (scope: SecurityProofScope) => checkVerificationMethods(scope),
  verify: (input: VerificationInput, scope: SecurityProofScope, encrypted: boolean) =>
    verify(input, { scope } as VerificationOperation, encrypted, new AbortController().signal),
  async run(action: string, proof: string) {
    const signal = new AbortController().signal
    switch (action) {
      case '2fa.setup': return setup2FA(proof, signal)
      case '2fa.disable': return disable2FA(proof, signal)
      case '2fa.backup_codes.regenerate': return regenerate2FABackupCodes(proof, signal)
      case 'access_token.generate': return createAccessToken(proof, signal)
      case 'access_token.revoke': return revokeAccessToken(proof, signal)
      case 'passkey.delete': return deletePasskey(proof, signal)
      case 'passkey.register': {
        const begin = await beginPasskeyRegistration(proof, signal)
        if (!begin.flow_token) throw new Error('安全验证已过期，请重新验证。')
        const credential = await createCredential(prepareCredentialCreationOptions(begin.options ?? begin), signal)
        const result = buildRegistrationResult(credential as PublicKeyCredential | null)
        if (!result) throw new Error('已取消通行密钥注册。')
        return finishPasskeyRegistration(begin.flow_token, result, signal)
      }
      default: throw new Error('不支持的安全操作。')
    }
  },
  enable: (code: string, flow: string) => enable2FA(code, flow, new AbortController().signal),
}

declare global { interface Window { FurtherSecurity: typeof security } }
window.FurtherSecurity = security
