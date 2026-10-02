import { Check, Palette } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import type { RelayForPalette } from '@/lib/theme-customization'
import { cn } from '@/lib/utils'

const palettes: {
  value: RelayForPalette
  label: string
  swatch: string
}[] = [
  { value: 'white', label: 'White', swatch: '#ffffff' },
  { value: 'blue', label: 'Blue', swatch: '#005ac1' },
  { value: 'yellow', label: 'Yellow', swatch: '#e9bd35' },
]

/** Color choice is independent of the existing light/dark mode and presets. */
export function RelayForPaletteSwitch({ className }: { className?: string }) {
  const { t } = useTranslation()
  const {
    customization: { palette },
    setPalette,
  } = useThemeCustomization()

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            className={cn('h-9 w-9', className)}
            aria-label={t('Color palette')}
          />
        }
      >
        <Palette className='size-[1.2rem]' aria-hidden='true' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='min-w-32'>
        {palettes.map((option) => (
          <DropdownMenuItem
            key={option.value}
            onClick={() => setPalette(option.value)}
          >
            <span
              aria-hidden='true'
              className='size-4 rounded-full border border-black/20 shadow-sm'
              style={{ backgroundColor: option.swatch }}
            />
            {t(option.label)}
            <Check
              aria-hidden='true'
              size={14}
              className={cn('ms-auto', palette !== option.value && 'invisible')}
            />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
