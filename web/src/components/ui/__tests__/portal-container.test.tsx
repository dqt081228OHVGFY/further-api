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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogTitle,
} from '../alert-dialog'
import { Combobox } from '../combobox'
import { ComboboxInput } from '../combobox-input'
import { Dialog, DialogContent, DialogTitle } from '../dialog'
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerTitle,
} from '../drawer'
import { PortalContainerContext } from '../portal-container'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../select'
import { Sheet, SheetContent, SheetTitle } from '../sheet'

const options = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Google' },
]

function ProviderCombobox(props: {
  onValueChange: (value: string | null) => void
}) {
  return (
    <Combobox
      options={options}
      value='openai'
      onValueChange={props.onValueChange}
      aria-label='Provider'
    />
  )
}

function ProviderSelect(props: {
  onValueChange: (value: string | null) => void
}) {
  return (
    <Select value='openai' onValueChange={props.onValueChange}>
      <SelectTrigger aria-label='Provider'>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function FilterDrawer(props: { children: ReactNode }) {
  return (
    <Drawer open>
      <DrawerContent>
        <DrawerTitle>Filters</DrawerTitle>
        <DrawerDescription>Adjust the result filters</DrawerDescription>
        {props.children}
      </DrawerContent>
    </Drawer>
  )
}

// JSDOM does not implement the pointer-capture API used by the drawer.
const pointerCapture = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'setPointerCapture'
)
const shadowHosts: HTMLElement[] = []

function renderInShadow(children: ReactNode) {
  const host = document.createElement('div')
  document.body.append(host)
  shadowHosts.push(host)
  const shadow = host.attachShadow({ mode: 'open' })
  const mount = document.createElement('div')
  const portal = document.createElement('div')
  shadow.append(mount, portal)
  render(
    <PortalContainerContext.Provider value={{ current: portal }}>
      {children}
    </PortalContainerContext.Provider>,
    { container: mount }
  )
  return { shadow, portal }
}

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'setPointerCapture', {
    configurable: true,
    value: () => undefined,
  })
})

afterEach(() => {
  shadowHosts.splice(0).forEach((host) => host.remove())
  if (pointerCapture) {
    Object.defineProperty(
      HTMLElement.prototype,
      'setPointerCapture',
      pointerCapture
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'setPointerCapture')
  }
})

describe('portals inside a shadow feature island', () => {
  it.each([
    [
      'dialog',
      <Dialog open>
        <DialogContent>
          <DialogTitle>Dialog title</DialogTitle>
        </DialogContent>
      </Dialog>,
      'dialog-content',
    ],
    [
      'alert dialog',
      <AlertDialog open>
        <AlertDialogContent>
          <AlertDialogTitle>Alert title</AlertDialogTitle>
        </AlertDialogContent>
      </AlertDialog>,
      'alert-dialog-content',
    ],
    [
      'sheet',
      <Sheet open>
        <SheetContent>
          <SheetTitle>Sheet title</SheetTitle>
        </SheetContent>
      </Sheet>,
      'sheet-content',
    ],
    [
      'drawer',
      <Drawer open>
        <DrawerContent>
          <DrawerTitle>Drawer title</DrawerTitle>
        </DrawerContent>
      </Drawer>,
      'drawer-content',
    ],
  ])('renders an open %s in the provided portal', async (_, content, slot) => {
    const { portal } = renderInShadow(content)
    await waitFor(() => {
      expect(portal.querySelector(`[data-slot="${slot}"]`)).not.toBeNull()
    })
  })

  it('places ComboboxInput options in the provided portal', async () => {
    const change = vi.fn()
    const { shadow, portal } = renderInShadow(
      <ComboboxInput
        options={options}
        value='openai'
        onValueChange={change}
        aria-label='Provider'
      />
    )
    const input = shadow.querySelector<HTMLInputElement>(
      'input[role="combobox"]'
    )
    expect(input).not.toBeNull()
    await userEvent.setup().click(input!)
    await waitFor(() => {
      expect(portal.querySelector('[role="listbox"]')).not.toBeNull()
    })
  })
})

describe('popups inside a drawer', () => {
  it('renders combobox options inside the drawer dialog and clicking one applies it without closing the drawer', async () => {
    const change = vi.fn()
    render(
      <FilterDrawer>
        <ProviderCombobox onValueChange={change} />
      </FilterDrawer>
    )
    const user = userEvent.setup()
    const dialog = screen.getByRole('dialog', { name: 'Filters' })

    await user.click(within(dialog).getByRole('combobox', { name: 'Provider' }))
    await user.click(
      await within(dialog).findByRole('option', { name: 'Google' })
    )

    expect(change).toHaveBeenCalledWith('gemini')
    expect(screen.getByRole('dialog', { name: 'Filters' })).toBeInTheDocument()
  })

  it('renders select options inside the drawer dialog and clicking one applies it without closing the drawer', async () => {
    const change = vi.fn()
    render(
      <FilterDrawer>
        <ProviderSelect onValueChange={change} />
      </FilterDrawer>
    )
    const user = userEvent.setup()
    const dialog = screen.getByRole('dialog', { name: 'Filters' })

    await user.click(within(dialog).getByRole('combobox', { name: 'Provider' }))
    await user.click(
      await within(dialog).findByRole('option', { name: 'Google' })
    )

    expect(change).toHaveBeenCalledWith('gemini', expect.anything())
    expect(screen.getByRole('dialog', { name: 'Filters' })).toBeInTheDocument()
  })
})

describe('popups outside a drawer', () => {
  it('portals combobox options to document.body outside the component subtree', async () => {
    const view = render(<ProviderCombobox onValueChange={vi.fn()} />)
    const user = userEvent.setup()

    await user.click(screen.getByRole('combobox', { name: 'Provider' }))
    const listbox = await screen.findByRole('listbox')

    expect(document.body).toContainElement(listbox)
    expect(view.container).not.toContainElement(listbox)
  })

  it('portals select options to document.body outside the component subtree', async () => {
    const view = render(<ProviderSelect onValueChange={vi.fn()} />)
    const user = userEvent.setup()

    await user.click(screen.getByRole('combobox', { name: 'Provider' }))
    const listbox = await screen.findByRole('listbox')

    expect(document.body).toContainElement(listbox)
    expect(view.container).not.toContainElement(listbox)
  })
})
