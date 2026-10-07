import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { Blob } from 'node:buffer'

import { JSDOM, VirtualConsole } from 'jsdom'

const assets = resolve(import.meta.dirname, '../../further-ui/assets')
const scripts = ['festival-subscriptions.js', 'activities.js'].map((name) => readFileSync(resolve(assets, name), 'utf8'))
const keys = ['duanwu', 'qixi', 'mid_autumn', 'chongyang', 'new_year_eve', 'national_day']

async function settle() {
  for (let index = 0; index < 20; index++) await Promise.resolve()
}

async function mount(page) {
  const errors = [], calls = [], animations = [], motions = [], downloads = [], blobs = [], payments = [], notices = []
  const console = new VirtualConsole()
  console.on('jsdomError', (error) => errors.push(error))
  const dom = new JSDOM('<!doctype html><html><body><nav class="nav-rail"><button data-nav="profile">账户</button></nav><main class="content-area"><header class="top-app-bar"><span data-page-title></span><span data-page-subtitle></span></header></main></body></html>', {
    url: `http://localhost/${  page  }/#activities`, runScripts: 'outside-only', pretendToBeVisual: true, virtualConsole: console,
  })
  const { window } = dom
  const { document } = window
  document.body.dataset.page = page
  window.addEventListener('error', (event) => errors.push(event.error))
  window.addEventListener('unhandledrejection', (event) => errors.push(event.reason))
  window.Blob = Blob
  window.CSS = { escape: (value) => value }
  // Activity motion remains enabled even when the operating system suggests otherwise.
  window.matchMedia = () => ({ matches: true, addEventListener() {}, removeEventListener() {} })
  window.HTMLElement.prototype.animate = function (frames, options) {
    const animation = { cancel() {}, onfinish: null, finished: Promise.resolve() }
    animations.push({ target: this, frames, options })
    queueMicrotask(() => animation.onfinish?.())
    return animation
  }
  window.HTMLAnchorElement.prototype.click = function () { downloads.push({ name: this.download, href: this.href }) }
  window.HTMLFormElement.prototype.submit = function () { payments.push({ action: this.action, method: this.method, fields: Object.fromEntries(new window.FormData(this)) }) }
  window.URL.createObjectURL = (blob) => { blobs.push(blob); return 'blob:holiday-test' }
  window.URL.revokeObjectURL = () => {}
  let dailyDone = false
  const snapshot = {
    server_date: '2026-09-25', server_time: '2026-09-25T12:00:00+08:00', timezone: 'Asia/Shanghai', theme: 'mid_autumn', calendar_min_year: 2025, calendar_max_year: 2028,
    checkin: { enabled: true, min_quota: 75, max_quota: 75 }, history: [],
    campaigns: keys.map((key) => ({ key, title: `${key  }活动`, description: '节日说明', details: '', rules: '', enabled: true, active: key === 'mid_autumn', date: '2026-09-25', next_date: '2026-09-25', starts_at: '2026-09-25T00:00:00+08:00', ends_at: '2026-09-26T00:00:00+08:00', reward_quota: 100, can_claim: key === 'mid_autumn', claimed_today: false, days_before: 0, days_after: 0, daily_claim_limit: 0, daily_budget_quota: 0, target_group: '', subscription_plan_ids: key === 'mid_autumn' ? [7] : [] })),
  }
  const plan = { id: 7, title: '节日付费套餐', enabled: true, price_amount: 9.9, currency: 'USD', duration_unit: 'month', duration_value: 1, total_amount: 1000, upgrade_group: 'legacy-vip' }
  window.FurtherUI = {
    async request(url, options = {}) {
      calls.push({ url, method: options.method || 'GET', body: options.body })
      if (url === '/api/user/self') return { role: 100, quota: 1000 }
      if (url === '/api/user/benefits') return []
      if (url.includes('/activities') && options.method === 'PUT') {
        if (url.endsWith('/checkin')) Object.assign(snapshot.checkin, options.body)
        else Object.assign(snapshot.campaigns.find((campaign) => campaign.key === options.body.key), options.body)
        return options.body
      }
      if (url === '/api/user/activities/claim') {
        const campaign = snapshot.campaigns.find((item) => item.key === options.body.key)
        campaign.claimed_today = true
        campaign.can_claim = false
        const record = { campaign_key: campaign.key, claim_date: snapshot.server_date, reward_quota: 100 }
        snapshot.history.push(record)
        return record
      }
      if (url.includes('/activities')) return structuredClone(snapshot)
      if (url === '/api/user/checkin') {
        if (options.method === 'POST') { dailyDone = true; return { checkin_date: snapshot.server_date, quota_awarded: 75 } }
        return { enabled: true, min_quota: 75, max_quota: 75, stats: { checked_in_today: dailyDone, total_checkins: dailyDone ? 1 : 0, total_quota: dailyDone ? 75 : 0, records: [] } }
      }
      if (url.endsWith('/plans')) return [{ plan }]
      if (url === '/api/user/ldc/info') return { enabled: true, configured: true }
      if (url === '/api/user/ldc/quote') return { currency: 'LDC', credits: '9.90', plan_id: 7, plan_title: plan.title }
      if (url === '/api/subscription/ldc/pay') return { currency: 'LDC', method: 'POST', trade_no: 'holiday-order', url: 'https://credit.linux.do/epay/pay/submit.php', params: { out_trade_no: 'holiday-order', money: '9.90' } }
      throw new Error(`Unexpected request: ${  url}`)
    },
    escapeHTML(value) { const span = document.createElement('span'); span.textContent = value; return span.innerHTML },
    moneyFromQuota: (value, digits = 6) => `$${  (value / 500000).toFixed(digits)}`,
    quotaFromUSD: (value) => Math.round(Number(value) * 500000), usdFromQuota: (value) => value / 500000,
    number: (value) => String(value), date: (value) => String(value),
    toast(message, kind) { notices.push({ message, kind }) },
    showBusy(button, busy) { button.dataset.busy = String(busy); button.disabled = busy },
    setText(selector, value) { document.querySelectorAll(selector).forEach((node) => { node.textContent = value }) },
    confirm: async () => true,
    transitionVisibility(element, visible, apply, options) { apply(visible); motions.push({ element, visible, options }) },
  }
  for (const script of scripts) window.eval(script)
  window.FurtherUIActivities.mount()
  const panel = document.querySelector('[data-panel="activities"]')
  panel.classList.add('active')
  await window.FurtherUIActivities.load()
  await settle()
  const navigate = (key = '') => { window.history.replaceState(null, '', `#activities${key ? `/${key}` : ''}`); window.dispatchEvent(new window.HashChangeEvent('hashchange')) }
  return { dom, window, document, panel, calls, animations, motions, downloads, blobs, payments, notices, errors, navigate }
}

const user = await mount('console')
try {
  assert.equal(user.panel.querySelectorAll('.festival-gallery-card').length, 6)
  assert.equal(user.motions.length, 0, 'initial entry belongs to the parent shell')
  assert.equal(user.panel.querySelector('[data-activity-status]').hidden, true)
  assert.equal(user.panel.querySelectorAll('[data-activity-subscribe], [data-activity-subscriptions], .festival-my-subscriptions').length, 0)
  for (const key of keys) {
    user.navigate(key)
    const calendar = user.panel.querySelector('[data-activity-festival-page] .festival-calendar-card [data-activity-calendar]')
    assert.ok(calendar, `${key  } retains independent calendar export`)
    assert.equal(calendar.disabled, false)
    assert.equal(user.panel.querySelectorAll('.festival-follow-card, .festival-subscribe').length, 0)
    assert.ok(user.panel.querySelector('.festival-countdown'), `${key  } retains its activity window`)
  }
  assert.equal(user.motions.length, keys.length, 'each real festival route gets one outer entry')
  assert.ok(user.motions.every((motion) => motion.options.forceEnter === true))
  user.navigate('mid_autumn')
  assert.equal(user.panel.querySelector('.festival-plan-card').textContent.includes('权益分组'), false)
  assert.equal(user.panel.querySelector('.festival-plan-card').textContent.includes('legacy-vip'), false)
  const localEntries = user.motions.length
  const beforeExport = user.calls.length
  user.panel.querySelector('.festival-calendar-card [data-activity-calendar]').click()
  assert.equal(user.calls.length, beforeExport, 'calendar export works independently of APIs')
  assert.ok(user.downloads[0].name.endsWith('.ics'))
  const calendar = await user.blobs[0].text()
  assert.ok(calendar.includes('DTSTART;VALUE=DATE:20260925\r\nDTEND;VALUE=DATE:20260926'))
  assert.ok(calendar.includes('PRODID:-//further-api//Festival Calendar//ZH'))
  assert.ok(calendar.includes('SUMMARY:mid_autumn活动'))
  user.panel.querySelector('[data-festival-plan-buy="7"]').click()
  await settle()
  assert.equal(user.payments.length, 1, 'paid holiday plans still create their own payment')
  assert.equal(user.payments[0].method, 'post')
  assert.equal(user.payments[0].fields.out_trade_no, 'holiday-order')
  assert.ok(user.calls.some((call) => call.url === '/api/subscription/ldc/pay' && call.body.plan_id === 7))
  user.panel.querySelector('[data-activity-claim="mid_autumn"]').click()
  await settle()
  assert.ok(user.calls.some((call) => call.url === '/api/user/activities/claim'))
  assert.equal(user.panel.querySelector('[data-activity-claim="mid_autumn"]').disabled, true)
  assert.equal(user.motions.length, localEntries, 'reward data arrival does not replay the route entry')
  user.navigate()
  user.panel.querySelector('[data-activity-checkin]').click()
  await settle()
  assert.ok(user.calls.some((call) => call.url === '/api/user/checkin' && call.method === 'POST'))
  assert.equal(user.panel.querySelector('[data-activity-checkin-result]').hidden, false)
  assert.ok(user.panel.querySelector('.activity-checkin-card').classList.contains('is-checkin-success'))
  const reads = user.calls.length
  user.navigate('chongyang')
  user.navigate()
  await user.window.FurtherUIActivities.load()
  assert.equal(user.calls.length, reads, 'navigation reuses the loaded activity snapshot')
  const settledEntries = user.motions.length
  user.navigate()
  await user.window.FurtherUIActivities.load(true)
  assert.equal(user.motions.length, settledEntries, 'same routes and explicit data refresh do not replay entry')
  user.panel.classList.add('rf-entering')
  user.navigate('qixi')
  assert.equal(user.motions.length, settledEntries, 'active parent entry owns the entire surface')
  user.panel.classList.remove('rf-entering')
  user.navigate()
  const afterParentEntry = user.motions.length
  user.panel.getAnimations = () => [{ playState: 'running' }]
  user.navigate('duanwu')
  assert.equal(user.motions.length, afterParentEntry, 'parent Web Animation prevents nested entry')
  delete user.panel.getAnimations
  user.navigate()
  const history = user.panel.querySelector('.activity-history')
  history.querySelector('summary').click()
  await settle()
  assert.equal(history.open, true)
  assert.ok(user.animations.some((item) => item.target === history), 'reward history still animates')
  assert.equal(user.calls.some((call) => call.url === '/api/user/activities/subscription'), false)
  assert.equal(user.notices.some((notice) => notice.kind === 'danger'), false)
  assert.deepEqual(user.errors, [])
} finally { user.dom.window.close() }

const admin = await mount('admin')
try {
  assert.equal(admin.panel.querySelector('[data-activity-status]').hidden, true)
  assert.equal(admin.panel.querySelectorAll('[name="subscription_enabled"], [data-activity-preview="subscription"]').length, 0)
  const form = admin.panel.querySelector('[data-activity-campaign-form="mid_autumn"]')
  assert.equal(form.querySelector('[name="target_group"]'), null)
  assert.equal(form.querySelector('[data-activity-preview="audience"]').textContent, '全部用户')
  const tab = form.querySelector('[data-activity-editor-tab="subscriptions"]')
  assert.equal(tab.textContent.includes('节日套餐'), true)
  tab.click()
  await settle()
  assert.ok(admin.animations.length > 0, 'admin section switches keep their animations')
  assert.equal(form.querySelector('[data-activity-editor-section="subscriptions"]').hidden, false)
  assert.equal(form.querySelector('[name="festival_plan_ids"][value="7"]').checked, true)
  form.elements.title.value = '中秋新活动'
  form.elements.title.dispatchEvent(new admin.window.Event('input', { bubbles: true }))
  form.dispatchEvent(new admin.window.Event('submit', { bubbles: true, cancelable: true }))
  await settle()
  const saved = admin.calls.find((call) => call.url === '/api/admin/activities' && call.method === 'PUT')
  assert.ok(saved, 'admin configuration saves without the deleted switch')
  assert.equal(saved.body.title, '中秋新活动')
  assert.deepEqual([...saved.body.subscription_plan_ids], [7])
  assert.equal(saved.body.target_group, '')
  for (const field of ['subscription_enabled', 'subscribed', 'can_subscribe', 'subscription_count']) assert.equal(field in saved.body, false)
  assert.equal(admin.calls.some((call) => call.url === '/api/user/activities/subscription'), false)
  assert.equal(admin.notices.some((notice) => notice.kind === 'danger'), false)
  assert.deepEqual(admin.errors, [])
} finally { admin.dom.window.close() }

console.log('Holiday activities: free follow removed; six pages, calendar export, paid purchase, daily/holiday rewards, history/admin animations and configuration save passed.')
