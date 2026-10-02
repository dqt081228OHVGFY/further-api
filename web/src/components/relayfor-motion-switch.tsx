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
import { Waves } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  setRelayForMotionEnabled,
  useRelayForMotionEnabled,
} from '@/hooks/use-relayfor-motion'

export function RelayForMotionSwitch() {
  const { t } = useTranslation()
  const enabled = useRelayForMotionEnabled()
  const label = enabled ? t('Disable animations') : t('Enable animations')

  return (
    <Button
      variant='ghost'
      size='icon'
      className='h-9 w-9'
      aria-label={label}
      aria-pressed={enabled}
      title={label}
      onClick={() => setRelayForMotionEnabled(!enabled)}
    >
      <Waves aria-hidden='true' className='size-[1.2rem]' />
    </Button>
  )
}
