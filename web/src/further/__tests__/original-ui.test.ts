import { readFileSync } from 'node:fs'
import path from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(path.resolve(process.cwd(), '../further-ui/assets/app.js'), 'utf8')

function originalDocument(storedPreferences?: Record<string, unknown>, systemReducedMotion = false) {
  document.body.dataset.page = 'admin'
  document.body.innerHTML = `
    <style>.view { display: none; } .view.active { display: block; } [hidden], .hidden { display: none !important; } .main-content { position: static; }</style>
    <nav class="nav-rail" data-shell><button class="nav-item active" data-nav="overview">概览</button><button class="nav-item" data-nav="users">用户</button></nav>
    <main class="main-content"><header class="top-app-bar"><div data-page-title>概览</div><div data-page-subtitle></div></header>
      <section class="view active" data-panel="overview" data-title="概览"></section>
      <section class="view" data-panel="users" data-title="用户"></section>
      <button id="action" class="md-btn">保存</button>
      <div id="sample-dialog" class="dialog-backdrop"><div class="md-dialog" tabindex="-1"><button>完成</button></div></div>
    </main>`
  document.documentElement.removeAttribute('data-further-motion')
  localStorage.clear()
  if (storedPreferences) {
    localStorage.setItem('further_user_preferences', JSON.stringify(storedPreferences))
    localStorage.setItem('further-motion', 'off')
  }
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: (query: string) => ({ matches: systemReducedMotion && query === '(prefers-reduced-motion: reduce)', addEventListener() {}, removeEventListener() {} }) })
  Object.defineProperty(window, 'CSS', { configurable: true, value: { escape: (value: string) => value } })
  window.eval(runtime)
  document.dispatchEvent(new Event('further:ready'))
  return { window }
}

type OriginalWindow = Window & { FurtherUI: {
  mountShell: () => void
  openDialog: (id: string) => void
  closeDialog: (id: string) => void
  transitionDropdown: (element: HTMLElement, visible: boolean, applyVisibility: (visible: boolean) => void, options?: { collapse?: boolean; immediate?: boolean }) => void
  transitionVisibility: (element: HTMLElement, visible: boolean, applyVisibility: (visible: boolean) => void) => void
  preferences: () => Record<string, unknown>
  savePreferences: (value: Record<string, unknown>, options?: { source: string }) => Record<string, unknown>
} }

function element(selector: string): HTMLElement {
  const result = document.querySelector<HTMLElement>(selector)
  if (!result) throw new Error(`Missing original UI element: ${selector}`)
  return result
}

describe('original Further interactions', () => {
  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })
  it('opens the admin overview without a synthetic click ripple', () => {
    const dom = originalDocument()
    dom.window.history.replaceState(null, '', '#overview')
    const overview = element('[data-nav="overview"]')
    overview.getBoundingClientRect = () => ({ x: 0, y: 0, left: 0, top: 0, width: 120, height: 40, right: 120, bottom: 40, toJSON() {} })
    const runtimeWindow = dom.window as unknown as OriginalWindow
    runtimeWindow.FurtherUI.mountShell()
    expect(element('[data-panel="overview"]').classList.contains('active')).toBe(true)
    expect(overview.querySelector('.md-ripple')).toBeNull()
  })

  it('retains hash panel switching with the original entry and title animation classes', () => {
    const dom = originalDocument()
    const runtimeWindow = dom.window as unknown as OriginalWindow
    runtimeWindow.FurtherUI.mountShell()
    element('[data-nav="users"]').click()
    expect(dom.window.location.hash).toBe('#users')
    expect(element('[data-panel="users"]').classList.contains('entering')).toBe(true)
    expect(element('[data-page-title]').textContent).toBe('用户')
    expect(element('[data-page-title]').classList.contains('updating')).toBe(true)
  })

  it('activates an SVG navigation target once after repeated shell mounts', () => {
    const dom = originalDocument()
    dom.window.history.replaceState(null, '', '#overview')
    const runtimeWindow = dom.window as unknown as OriginalWindow
    runtimeWindow.FurtherUI.mountShell()
    const nav = document.createElement('button')
    nav.type = 'button'
    nav.className = 'nav-item'
    nav.dataset.nav = 'activities'
    nav.innerHTML = '<span class="material-symbol"><svg viewBox="0 0 24 24"><path d="M3 12h18"/></svg></span>活动与签到'
    element('.nav-rail').append(nav)
    const panel = document.createElement('section')
    panel.dataset.panel = 'activities'
    panel.dataset.title = '活动与签到'
    element('.main-content').append(panel)
    runtimeWindow.FurtherUI.mountShell()
    runtimeWindow.FurtherUI.mountShell()
    const pages: string[] = []
    const record = (event: Event) => pages.push((event as CustomEvent).detail.name)
    dom.window.addEventListener('further-ui:page', record)
    try {
      const click = new dom.window.MouseEvent('click', { bubbles: true, cancelable: true })
      nav.querySelector('path')!.dispatchEvent(click)
      expect(click.defaultPrevented).toBe(true)
      expect(pages).toEqual(['activities'])
      expect(dom.window.location.hash).toBe('#activities')
      expect(panel.classList.contains('active')).toBe(true)
      expect(element('[data-panel="overview"]').classList.contains('active')).toBe(false)
      pages.length = 0
      dom.window.history.replaceState(null, '', '#users')
      dom.window.dispatchEvent(new Event('hashchange'))
      expect(pages).toEqual(['users'])
    } finally {
      dom.window.removeEventListener('further-ui:page', record)
    }
  })

  it('prevents an icon inside a shell anchor from also following its native href', () => {
    const dom = originalDocument()
    dom.window.history.replaceState(null, '', '#overview')
    const nav = document.createElement('a')
    nav.href = '#overview'
    nav.dataset.nav = 'users'
    nav.innerHTML = '<svg viewBox="0 0 24 24"><path d="M3 12h18"/></svg>用户'
    element('.nav-rail').append(nav)
    const runtimeWindow = dom.window as unknown as OriginalWindow
    runtimeWindow.FurtherUI.mountShell()
    const click = new dom.window.MouseEvent('click', { bubbles: true, cancelable: true })
    nav.querySelector('path')!.dispatchEvent(click)
    expect(click.defaultPrevented).toBe(true)
    expect(dom.window.location.hash).toBe('#users')
    expect(element('[data-page-title]').textContent).toBe('用户')
  })

  it('retains pointer ripples despite retired off flags and system reduced motion', () => {
    const dom = originalDocument(undefined, true)
    const button = element('#action')
    button.getBoundingClientRect = () => ({ x: 10, y: 10, left: 10, top: 10, width: 120, height: 40, right: 130, bottom: 50, toJSON() {} })
    button.dispatchEvent(new dom.window.MouseEvent('pointerdown', { bubbles: true, clientX: 30, clientY: 25, button: 0 }))
    expect(button.querySelector('.md-ripple')).not.toBeNull()
    button.querySelectorAll('.md-ripple').forEach((ripple) => ripple.remove())
    dom.window.document.documentElement.dataset.furtherMotion = 'off'
    dom.window.document.documentElement.dataset.reducedMotion = 'true'
    button.dispatchEvent(new dom.window.MouseEvent('pointerdown', { bubbles: true, clientX: 30, clientY: 25, button: 0 }))
    expect(button.querySelector('.md-ripple')).not.toBeNull()
  })

  it('keeps the original dialog opening and animated closing lifecycle', () => {
    const dom = originalDocument()
    const runtimeWindow = dom.window as unknown as OriginalWindow
    runtimeWindow.FurtherUI.openDialog('sample-dialog')
    const dialog = element('#sample-dialog')
    expect(dialog.classList.contains('show')).toBe(true)
    runtimeWindow.FurtherUI.closeDialog('sample-dialog')
    expect(dialog.classList.contains('closing')).toBe(true)
  })

  it('removes the previous page from layout and interaction until its exit finishes', () => {
    vi.useFakeTimers()
    const dom = originalDocument()
    dom.window.history.replaceState(null, '', '#overview')
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    api.mountShell()
    element('[data-nav="users"]').click()
    const outgoing = element('[data-panel="overview"]')
    const incoming = element('[data-panel="users"]')
    expect(outgoing.classList.contains('active')).toBe(false)
    expect(outgoing.classList.contains('rf-leaving')).toBe(true)
    expect(outgoing.style.position).toBe('absolute')
    expect(outgoing.hasAttribute('inert')).toBe(true)
    expect(outgoing.getAttribute('aria-hidden')).toBe('true')
    expect(incoming.classList.contains('rf-entering')).toBe(true)
    expect(incoming.hasAttribute('inert')).toBe(false)
    vi.advanceTimersByTime(180)
    expect(outgoing.classList.contains('rf-leaving')).toBe(false)
    expect(outgoing.style.position).toBe('')
    expect(outgoing.hasAttribute('inert')).toBe(false)
    expect(getComputedStyle(outgoing).display).toBe('none')
    expect(element('.main-content').style.position).toBe('')
    vi.advanceTimersByTime(30)
    expect(incoming.classList.contains('entering')).toBe(true)
    vi.advanceTimersByTime(10)
    expect(incoming.classList.contains('entering')).toBe(false)
  })

  it('cancels stale page exits when rapidly returning to a page', () => {
    vi.useFakeTimers()
    const dom = originalDocument()
    dom.window.history.replaceState(null, '', '#overview')
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    api.mountShell()
    element('[data-nav="users"]').click()
    vi.advanceTimersByTime(60)
    element('[data-nav="overview"]').click()
    const overview = element('[data-panel="overview"]')
    expect(overview.classList.contains('rf-leaving')).toBe(false)
    expect(overview.classList.contains('rf-entering')).toBe(true)
    expect(overview.hasAttribute('inert')).toBe(false)
    expect(overview.style.display).toBe('')
    vi.advanceTimersByTime(300)
    expect(overview.classList.contains('active')).toBe(true)
    expect(getComputedStyle(overview).display).toBe('block')
    expect(element('[data-panel="users"]').classList.contains('active')).toBe(false)
    expect(document.querySelector('.rf-leaving')).toBeNull()
  })

  it('keeps a scrolled page exit in the viewport while resetting the incoming page scroll', () => {
    vi.useFakeTimers()
    const dom = originalDocument()
    dom.window.history.replaceState(null, '', '#overview')
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    const main = element('.main-content')
    const area = document.createElement('div')
    area.className = 'content-area'
    main.append(area)
    main.querySelectorAll('.view').forEach(panel => area.append(panel))
    api.mountShell()
    main.scrollTop = 200
    element('[data-nav="users"]').click()
    const outgoing = element('[data-panel="overview"]')
    expect(outgoing).toHaveClass('rf-leaving')
    expect(outgoing.style.top).toBe('-200px')
    expect(main.scrollTop).toBe(0)
    expect(element('[data-panel="users"]')).toHaveClass('rf-entering')
    vi.advanceTimersByTime(320)
    expect(outgoing.style.top).toBe('')
    expect(document.querySelector('.rf-leaving')).toBeNull()
  })

  it('preserves hidden subpage semantics and ignores child animation completions', () => {
    vi.useFakeTimers()
    const dom = originalDocument()
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    const subpage = document.createElement('section')
    subpage.style.display = 'grid'
    subpage.style.margin = '12px'
    subpage.innerHTML = '<button>操作</button>'
    element('.main-content').append(subpage)
    api.transitionVisibility(subpage, false, (visible) => { subpage.hidden = !visible })
    expect(subpage.hidden).toBe(true)
    expect(getComputedStyle(subpage).display).toBe('grid')
    subpage.firstElementChild!.dispatchEvent(new Event('animationend', { bubbles: true }))
    expect(subpage.classList.contains('rf-leaving')).toBe(true)
    subpage.dispatchEvent(new Event('animationend', { bubbles: true }))
    expect(subpage.classList.contains('rf-leaving')).toBe(false)
    expect(subpage.style.margin).toBe('12px')
    expect(subpage.style.display).toBe('grid')
    expect(subpage.style.getPropertyPriority('display')).toBe('')
    expect(subpage.hidden).toBe(true)
    expect(getComputedStyle(subpage).display).toBe('none')
  })

  it('keeps page and dialog transition durations despite retired motion-off flags', () => {
    vi.useFakeTimers()
    const dom = originalDocument(undefined, true)
    dom.window.history.replaceState(null, '', '#overview')
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    api.mountShell()
    document.documentElement.dataset.furtherMotion = 'off'
    document.documentElement.dataset.reducedMotion = 'true'
    element('[data-nav="users"]').click()
    expect(element('[data-panel="overview"]')).toHaveClass('rf-leaving')
    expect(element('[data-panel="users"]')).toHaveClass('rf-entering')
    api.openDialog('sample-dialog')
    api.closeDialog('sample-dialog')
    expect(element('#sample-dialog')).toHaveClass('show', 'closing')
    vi.advanceTimersByTime(229)
    expect(element('#sample-dialog')).toHaveClass('show', 'closing')
    vi.advanceTimersByTime(1)
    expect(element('#sample-dialog')).not.toHaveClass('show')
  })

  it('normalizes old local and server motion preferences while preserving other settings', () => {
    const retained = { theme: 'dark', timezone: 'Asia/Shanghai', pageSize: 50, sidebarCollapsed: true, hourCycle: '12', hideSuccessfulLogs: true }
    const dom = originalDocument({ ...retained, reducedMotion: true }, true)
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    expect(api.preferences()).toMatchObject({ ...retained, reducedMotion: false })
    expect(JSON.parse(localStorage.getItem('further_user_preferences')!)).toMatchObject({ ...retained, reducedMotion: false })
    expect(localStorage.getItem('further-motion')).toBe('on')
    expect(document.documentElement.dataset.furtherMotion).toBe('on')
    expect(document.documentElement.dataset.reducedMotion).toBe('false')
    const next = api.savePreferences({ ...retained, reducedMotion: true }, { source: 'server' })
    expect(next).toMatchObject({ ...retained, reducedMotion: false })
    expect(JSON.parse(localStorage.getItem('further_user_preferences')!)).toMatchObject({ ...retained, reducedMotion: false })
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(document.documentElement.dataset.railCollapsed).toBe('true')
  })

  it('waits for compositor completion before hiding a dialog on a busy frame', async () => {
    vi.useFakeTimers()
    const dom = originalDocument()
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    const dialog = element('#sample-dialog')
    const complete: Array<() => void> = []
    const animate = vi.fn(() => ({
      finished: new Promise<void>(resolve => complete.push(resolve)),
      cancel: vi.fn(),
    } as unknown as Animation))
    dialog.animate = animate
    ;(dialog.firstElementChild as HTMLElement).animate = animate
    api.openDialog(dialog.id)
    api.closeDialog(dialog.id)
    vi.advanceTimersByTime(500)
    expect(dialog).toHaveClass('show', 'closing')
    complete.slice(-2).forEach(resolve => resolve())
    await Promise.resolve()
    await Promise.resolve()
    expect(dialog).not.toHaveClass('show')
  })

  it('keeps a dropdown exit painted until completion and ignores an interrupted close', async () => {
    vi.useFakeTimers()
    const dom = originalDocument()
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    const menu = document.createElement('div')
    menu.className = 'model-picker-menu'; menu.hidden = true
    element('.main-content').append(menu)
    const effects: Array<{ finish: () => void; cancel: ReturnType<typeof vi.fn> }> = []
    menu.animate = vi.fn(() => {
      let finish!: () => void
      const finished = new Promise<void>(resolve => { finish = resolve })
      const cancel = vi.fn()
      effects.push({ finish, cancel })
      return { finished, cancel } as unknown as Animation
    })
    const apply = (shown: boolean) => { menu.hidden = !shown }
    api.transitionDropdown(menu, true, apply)
    expect(menu.hidden).toBe(false)
    api.transitionDropdown(menu, false, apply)
    expect(menu).toHaveAttribute('inert')
    vi.advanceTimersByTime(500)
    expect(menu.hidden).toBe(false)
    api.transitionDropdown(menu, true, apply)
    expect(effects[1].cancel).toHaveBeenCalledOnce()
    effects[1].finish(); await Promise.resolve()
    expect(menu.hidden).toBe(false)
    expect(menu).not.toHaveAttribute('inert')
    effects[2].finish(); await Promise.resolve()
    api.transitionDropdown(menu, false, apply)
    effects[3].finish(); await Promise.resolve()
    expect(menu.hidden).toBe(true)
    expect(menu.dataset.dropdownMotion).toBeUndefined()
  })

  it('animates mobile navigation while preserving Escape focus and quick reopen behavior', async () => {
    const dom = originalDocument()
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    api.mountShell()
    const menu = element('.mobile-nav-menu')
    const trigger = element('[data-mobile-menu-trigger]')
    const effects: Array<() => void> = []
    menu.animate = vi.fn(() => ({ finished: new Promise<void>(resolve => effects.push(resolve)), cancel: vi.fn() } as unknown as Animation))
    trigger.click()
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(document.activeElement).toBe(menu.querySelector('.mobile-nav-option'))
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(document.activeElement).toBe(trigger)
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(menu).toHaveAttribute('inert')
    trigger.click()
    effects[1](); await Promise.resolve()
    expect(menu.hidden).toBe(false)
    expect(menu).not.toHaveAttribute('inert')
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    effects[2](); await Promise.resolve()
  })

  it('does not let a previous dialog close hide a quickly reopened dialog', () => {
    vi.useFakeTimers()
    const dom = originalDocument()
    const api = (dom.window as unknown as OriginalWindow).FurtherUI
    const dialog = element('#sample-dialog')
    api.openDialog(dialog.id)
    api.closeDialog(dialog.id)
    vi.advanceTimersByTime(60)
    api.openDialog(dialog.id)
    vi.advanceTimersByTime(200)
    expect(dialog.classList.contains('show')).toBe(true)
    expect(dialog.classList.contains('closing')).toBe(false)
    expect(dialog.getAttribute('aria-hidden')).toBe('false')
    api.closeDialog(dialog.id)
    dialog.firstElementChild!.dispatchEvent(new Event('animationend', { bubbles: true }))
    expect(dialog.classList.contains('closing')).toBe(true)
    dialog.dispatchEvent(new Event('animationend', { bubbles: true }))
    expect(dialog.classList.contains('show')).toBe(false)
  })
})
