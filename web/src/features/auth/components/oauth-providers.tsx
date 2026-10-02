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
import { useTranslation } from 'react-i18next'

import { IconLinuxDo } from '@/assets/brand-icons'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { useOAuthLogin } from '../hooks/use-oauth-login'
import type { SystemStatus } from '../types'

type OAuthProvidersProps = {
  status: SystemStatus | null
  disabled?: boolean
  className?: string
  onWeChatLogin?: () => void
  isWeChatLoading?: boolean
  redirectTo?: string
  registrationCode?: string
}

export function OAuthProviders({
  status,
  disabled = false,
  className,
  redirectTo,
  registrationCode,
}: OAuthProvidersProps) {
  const { t } = useTranslation()
  const { isLoading, handleLinuxDOLogin } = useOAuthLogin(
    status,
    redirectTo,
    registrationCode
  )

  if (!status?.linuxdo_oauth) return null

  return (
    <div className={cn('space-y-3', className)}>
      <div className='relative'>
        <div className='absolute inset-0 flex items-center'>
          <span className='w-full border-t' />
        </div>
        <div className='relative flex justify-center text-xs uppercase'>
          <span className='bg-background text-muted-foreground px-2'>
            {t('Or continue with')}
          </span>
        </div>
      </div>
      <Button
        variant='outline'
        type='button'
        disabled={disabled || isLoading}
        onClick={handleLinuxDOLogin}
        className='h-11 w-full justify-center gap-2 rounded-lg'
      >
        <IconLinuxDo className='h-6 w-6' alt='' aria-hidden='true' />
        {t('Continue with LinuxDO')}
      </Button>
    </div>
  )
}
