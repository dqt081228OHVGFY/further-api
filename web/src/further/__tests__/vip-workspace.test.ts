import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { afterEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/vip-workspace.js'), 'utf8')
const removeListeners: (() => void)[] = []
function setup(page: 'admin' | 'console', request: ReturnType<typeof vi.fn>) {
  document.body.dataset.page = page
  document.body.dataset.gatewayRole = page === 'admin' ? '100' : '1'
  document.body.innerHTML = `<nav class="nav-rail"><button data-nav="subscriptions">订阅与福利</button></nav><main class="content-area">${page === 'admin' ? '<section data-panel="subscriptions"><h1>订阅与福利</h1><span data-subscription-count="paid"></span><div data-subscription-page="paid"></div></section>' : ''}</main>`
  const toast = vi.fn()
  const confirm = vi.fn().mockResolvedValue(true)
  const closeDialog = vi.fn()
  const date = vi.fn((value:number)=>String(value))
  Object.defineProperty(window, 'FurtherUI', {configurable:true,value:{
    request, toast, escapeHTML:(value:string)=>value.replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('"','&quot;'), number:String,
    moneyFromQuota:(value:number)=>`$${(value/500000).toFixed(2)}`, usdFromQuota:(value:number)=>value/500000, quotaFromUSD:(value:number)=>Math.round(value*500000),
    date, openDialog:vi.fn(), closeDialog, showBusy:vi.fn(), confirm,
  }})
  const registration = vi.spyOn(document, 'addEventListener')
  const windowRegistration = vi.spyOn(window, 'addEventListener')
  window.eval(runtime)
  document.dispatchEvent(new Event('further:ready'))
  for(const [name, listener] of registration.mock.calls) { if(name === 'further:ready') document.removeEventListener(name, listener) }
  for (const [name, listener, options] of windowRegistration.mock.calls) {
    removeListeners.push(() => window.removeEventListener(name, listener, options))
  }
  registration.mockRestore()
  windowRegistration.mockRestore()
  return {toast,confirm,date,closeDialog,load:(options?: {reuse: boolean})=>(window as unknown as {FurtherUIVIP:{load:(options?: {reuse: boolean})=>Promise<void>}}).FurtherUIVIP.load(options)}
}
afterEach(()=>{
  removeListeners.splice(0).forEach(remove => remove())
  document.body.replaceChildren()
  history.replaceState(null, '', '/')
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})
const plan={id:7,title:'VIP Studio',subtitle:'调用额度权益',price_amount:2.5,currency:'USD',duration_unit:'day',duration_value:30,total_amount:5000000,quota_reset_period:'monthly',upgrade_group:'vip',enabled:true,allow_balance_pay:true,stripe_price_id:'existing-price',creem_product_id:'existing-product',custom_seconds:43200}
describe('VIP native card workspace',()=>{
  it('prepares cycle rules for review while preserving commercial fields until an explicit save', async () => {
    const request = vi.fn(async (path: string) => path.includes('/plans') ? [{ plan }] : { enabled: true, configured: true })
    const app = setup('admin', request)
    await app.load()
    document.querySelector<HTMLButtonElement>('[data-vip-edit="7"]')!.click()
    const form = document.querySelector<HTMLFormElement>('[data-vip-plan-form]')!
    const input = (name: string) => form.elements.namedItem(name) as HTMLInputElement
    const previousPrice = input('price_amount').value
    const previousQuota = input('quota_usd').value
    const previousPlan = form.dataset.existingPlan
    const requestsBefore = request.mock.calls.length
    document.querySelector<HTMLButtonElement>('[data-vip-cycle-preset="daily"]')!.click()
    expect(input('duration_unit').value).toBe('day')
    expect(input('duration_value').value).toBe('30')
    expect(input('quota_reset_period').value).toBe('daily')
    expect(input('price_amount').value).toBe(previousPrice)
    expect(input('quota_usd').value).toBe(previousQuota)
    expect(form.dataset.existingPlan).toBe(previousPlan)
    expect(request).toHaveBeenCalledTimes(requestsBefore)
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/subscription/admin/plans/7', expect.objectContaining({ method: 'PUT', body: expect.objectContaining({ plan: expect.objectContaining({ quota_reset_period: 'daily', price_amount: plan.price_amount, total_amount: plan.total_amount }) }) })))
  })
  it('opens the payment workspace once without loading the entitlement catalog', async () => {
    history.replaceState(null, '', '#vip/payment')
    const mount = vi.fn().mockResolvedValue({})
    vi.stubGlobal('FurtherFeatures', { mount })
    const request = vi.fn()
    setup('console', request)
    expect(location.hash).toBe('#wallet')
    window.dispatchEvent(new CustomEvent('further-ui:page', { detail: { name: 'wallet' } }))
    await vi.waitFor(() => expect(mount).toHaveBeenCalledTimes(1))
    expect(mount).toHaveBeenCalledWith(document.querySelector('[data-panel=wallet] [data-native-feature-path]'), { path: '/wallet' })
    window.dispatchEvent(new CustomEvent('further-ui:page', { detail: { name: 'wallet' } }))
    expect(mount).toHaveBeenCalledTimes(1)
    expect(request).not.toHaveBeenCalled()
  })
  it('loads the catalog only after the paid subscription view is opened', async () => {
    history.replaceState(null, '', '#overview')
    const request = vi.fn(async (path: string) => path.includes('/plans') ? [{plan}] : {enabled:true,configured:true})
    setup('admin', request)
    expect(request).not.toHaveBeenCalled()
    history.replaceState(null, '', '#subscriptions/site')
    window.dispatchEvent(new CustomEvent('further-ui:page', {detail:{name:'subscriptions'}}))
    expect(request).not.toHaveBeenCalled()
    history.replaceState(null, '', '#subscriptions/paid')
    window.dispatchEvent(new CustomEvent('further-ui:page', {detail:{name:'subscriptions'}}))
    await vi.waitFor(() => expect(document.querySelector('[data-vip-plan="7"]')).not.toBeNull())
    expect(request.mock.calls.filter(call => call[0] === '/api/subscription/admin/plans')).toHaveLength(1)
  })
  it('renders admin plans while payment availability is still pending',async()=>{
    let finishPayment!: (value: unknown) => void
    const payment = new Promise(resolve => {finishPayment = resolve})
    const request=vi.fn((path:string)=>path.includes('/plans')?Promise.resolve([{plan}]):payment)
    const app=setup('admin',request)
    await app.load()
    expect(document.querySelector('[data-vip-plan="7"]')).not.toBeNull()
    expect(document.querySelector<HTMLButtonElement>('[data-vip-create]')?.disabled).toBe(false)
    expect(document.querySelector('[data-subscription-page="paid"] .vip-workspace')?.hasAttribute('aria-busy')).toBe(false)
    expect(document.querySelector('[data-nav="vip"]')).toBeNull()
    expect(document.querySelectorAll('[data-panel="subscriptions"] h1')).toHaveLength(1)
    expect(document.querySelector('[data-subscription-count="paid"]')).toHaveTextContent('1')
    expect(document.querySelector('[data-subscription-page="paid"] .vip-summary-grid')).toBeNull()
    expect(document.querySelector('[data-vip-availability]')?.textContent).toContain('正在读取支付状态')
    finishPayment({enabled:true,configured:true})
    await vi.waitFor(()=>expect(document.querySelector('[data-vip-availability]')?.classList.contains('hidden')).toBe(true))
  })
  it('sends a saved admin VIP link to the unified subscription workspace', async () => {
    history.replaceState(null, '', '#vip')
    const request = vi.fn(async (path: string) => path.includes('/plans') ? [{plan}] : {enabled:true,configured:true})
    const app = setup('admin', request)
    await app.load()
    expect(location.hash).toBe('#subscriptions/paid')
    expect(document.querySelector('[data-subscription-page="paid"] [data-vip-plan="7"]')).not.toBeNull()
    history.replaceState(null, '', '/')
  })
  it('synchronizes the paid tab once when traversing a saved VIP link on the active workspace', async () => {
    history.replaceState(null, '', '#subscriptions/benefits')
    const request = vi.fn(async (path: string) => path.includes('/plans') ? [{plan}] : {enabled:true,configured:true})
    const app = setup('admin', request)
    await app.load()
    document.querySelector('[data-panel="subscriptions"]')!.classList.add('view', 'active')
    const navigation = vi.spyOn(document.querySelector<HTMLButtonElement>('[data-nav="subscriptions"]')!, 'click')
    const update = vi.fn()
    window.addEventListener('further-ui:page', update)
    history.replaceState(null, '', '#vip')
    window.dispatchEvent(new PopStateEvent('popstate'))
    window.dispatchEvent(new HashChangeEvent('hashchange'))
    expect(location.hash).toBe('#subscriptions/paid')
    expect(navigation).not.toHaveBeenCalled()
    expect(update).toHaveBeenCalledOnce()
    window.removeEventListener('further-ui:page', update)
  })
  it('keeps plan management available when the payment status request fails',async()=>{
    const request=vi.fn(async(path:string)=>{if(path.includes('/plans'))return [{plan}];throw new Error('payment unavailable')})
    const app=setup('admin',request); await app.load()
    await vi.waitFor(()=>expect(document.querySelector('[data-vip-availability]')?.textContent).toContain('支付状态暂时无法读取'))
    expect(document.querySelector('[data-vip-plan="7"]')).not.toBeNull()
    expect(document.querySelector<HTMLButtonElement>('[data-vip-create]')?.disabled).toBe(false)
  })
  it('keeps navigation stable after a long absence and refreshes only on demand',async()=>{
    let now = 100000
    vi.spyOn(Date, 'now').mockImplementation(()=>now)
    const request=vi.fn(async(path:string)=>path.includes('/plans')?[{plan}]:{enabled:true,configured:true})
    const app=setup('admin',request); await app.load()
    const card=document.querySelector('[data-vip-plan="7"]')
    request.mockClear()
    await app.load({reuse:true})
    expect(request).not.toHaveBeenCalled()
    expect(document.querySelector('[data-vip-plan="7"]')).toBe(card)
    await app.load()
    expect(request).toHaveBeenCalledWith('/api/subscription/admin/plans')
    request.mockClear()
    now += 3600000
    await app.load({reuse:true})
    expect(request).not.toHaveBeenCalled()
    expect(document.querySelector('[data-vip-plan="7"]')).toBe(card)
    await app.load()
    expect(request).toHaveBeenCalledWith('/api/subscription/admin/plans')
  })
  it('renders actual prices, duration and quota in cards',async()=>{
    const request=vi.fn(async(path:string)=>path.includes('/plans')?[{plan}]:{enabled:true,configured:true})
    const app=setup('admin',request); await app.load()
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('$2.50')
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('30 天')
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('$10.00')
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).not.toContain('权益分组')
  })
  it('keeps configuration required state actionable without reporting success',async()=>{
    const request=vi.fn(async(path:string)=>path.includes('/plans')?[]:{enabled:false,configured:false})
    const app=setup('admin',request); await app.load()
    expect(document.querySelector('[data-vip-availability]')?.textContent).toContain('尚未开放')
    expect(document.querySelector<HTMLButtonElement>('[data-vip-create]')?.disabled).toBe(false)
    expect(request.mock.calls.some(call=>call[0].includes('payment_compliance'))).toBe(false)
  })
  it('removes obsolete provider configuration and preserves explicit zero values during edits',async()=>{
    const request=vi.fn(async(path:string)=>path.includes('/plans')?[{plan}]:{enabled:true,configured:true})
    const app=setup('admin',request); await app.load()
    document.querySelector<HTMLButtonElement>('[data-vip-edit="7"]')!.click()
    const form=document.querySelector<HTMLFormElement>('[data-vip-plan-form]')!
    ;(form.elements.namedItem('price_amount') as HTMLInputElement).value='0'
    ;(form.elements.namedItem('sort_order') as HTMLInputElement).value='0'
    form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}))
    await vi.waitFor(()=>expect(request).toHaveBeenCalledWith('/api/subscription/admin/plans/7',expect.objectContaining({method:'PUT',body:{plan:expect.objectContaining({price_amount:0,sort_order:0,custom_seconds:43200,upgrade_group:'',downgrade_group:''})}})))
    expect(form.querySelector('[name="stripe_price_id"], [name="creem_product_id"], [name="allow_balance_pay"], [name="upgrade_group"], [name="downgrade_group"]')).toBeNull()
  })
  it('does not show a successful purchase when the server rejects LDC payment',async()=>{
    const request=vi.fn(async(path:string)=>{if(path==='/api/subscription/plans')return [{plan}];if(path==='/api/user/self')return {group:'default',quota:0};if(path==='/api/subscription/self')return {all_subscriptions:[]};if(path==='/api/user/ldc/quote')return {plan_id:7,credits:'2.50',currency:'LDC'};if(path==='/api/subscription/ldc/pay')throw new Error('LDC服务不可用');return {enabled:true,configured:true}})
    const app=setup('console',request); await app.load()
    document.querySelector<HTMLButtonElement>('[data-vip-buy="7"]')!.click()
    await vi.waitFor(()=>expect(document.querySelector('[data-vip-status]')?.textContent).toContain('LDC服务不可用'))
    expect(app.toast).not.toHaveBeenCalledWith(expect.stringContaining('已开通'))
  })
  it('shows small prices and custom duration accurately and submits the confirmed quote',async()=>{
    const precise={...plan,price_amount:0.001,duration_unit:'custom',custom_seconds:5400}
    const request=vi.fn(async(path:string)=>{if(path==='/api/subscription/plans')return [{plan:precise}];if(path==='/api/user/self')return {group:'default',quota:500000};if(path==='/api/subscription/self')return {all_subscriptions:[]};if(path==='/api/user/ldc/quote')return {plan_id:7,credits:'0.10',currency:'LDC'};if(path==='/api/subscription/ldc/pay')return {url:'https://credit.linux.do/epay/pay/submit.php',method:'POST',trade_no:'LDC-test',currency:'LDC',params:{sign:'signed'}};return {enabled:true,configured:true}})
    const submit=vi.spyOn(HTMLFormElement.prototype,'submit').mockImplementation(()=>{})
    const app=setup('console',request); await app.load()
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('$0.001')
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('1 小时 30 分钟')
    document.querySelector<HTMLButtonElement>('[data-vip-buy="7"]')!.click()
    await vi.waitFor(()=>expect(request).toHaveBeenCalledWith('/api/subscription/ldc/pay',{method:'POST',body:{plan_id:7,expected_credits:'0.10'}}))
    expect(app.confirm).toHaveBeenCalledWith(expect.stringContaining('0.10 LDC'),expect.anything())
    expect(submit).toHaveBeenCalledOnce()
    expect(app.toast).not.toHaveBeenCalledWith(expect.stringContaining('已开通'))
  })
  it('uses subscription title snapshots and converts Unix seconds to milliseconds',async()=>{
    const end=Math.floor(Date.now()/1000)+86400
    const request=vi.fn(async(path:string)=>{if(path==='/api/subscription/plans')return [];if(path==='/api/user/self')return {group:'vip',quota:500000};if(path==='/api/subscription/self')return {all_subscriptions:[{plan_title:'已停用的 VIP 套餐',subscription:{id:8,plan_id:7,status:'active',end_time:end,amount_total:500000,amount_used:0}}]};return {payment_compliance_confirmed:true}})
    const app=setup('console',request); await app.load()
    expect(document.querySelector('[data-vip-owned]')?.textContent).toContain('已停用的 VIP 套餐')
    expect(document.querySelector('[data-vip-current]')?.textContent).toContain('VIP 订阅已生效')
    expect(document.querySelector('[data-vip-current]')?.textContent).not.toContain('default')
    expect(document.querySelector('[data-vip-active-count]')?.textContent).toBe('1')
    expect(app.date).toHaveBeenCalledWith(end*1000)
  })
  it('preserves a failed plan draft and shows the server error inside its dialog', async () => {
    const request = vi.fn(async (path: string, options?: {method: string}) => {
      if (options?.method === 'PUT') throw new Error('套餐名称重复，请修改后重试')
      return path.includes('/plans') ? [{plan}] : {enabled:true,configured:true}
    })
    const app = setup('admin', request); await app.load()
    document.querySelector<HTMLButtonElement>('[data-vip-edit="7"]')!.click()
    const dialog = document.querySelector('#vip-plan-dialog')!
    const form = dialog.querySelector<HTMLFormElement>('form')!
    ;(form.elements.namedItem('title') as HTMLInputElement).value = '待保存的套餐'
    form.dispatchEvent(new Event('submit', {bubbles:true,cancelable:true}))
    await vi.waitFor(() => expect(dialog.querySelector('[data-vip-dialog-feedback]')).toHaveTextContent('套餐名称重复'))
    expect(dialog.querySelector('[data-vip-dialog-feedback]')).toHaveAttribute('role', 'alert')
    expect((form.elements.namedItem('title') as HTMLInputElement).value).toBe('待保存的套餐')
    expect((form.elements.namedItem('title') as HTMLInputElement).disabled).toBe(false)
    expect(dialog.hasAttribute('data-dialog-busy')).toBe(false)
    expect(app.closeDialog).not.toHaveBeenCalled()
  })
  it('prevents a second submission and dismissal while the plan mutation is pending', async () => {
    let finish!: (value: unknown) => void
    const mutation = new Promise(resolve => {finish = resolve})
    const request = vi.fn((path: string, options?: {method: string}) => options?.method === 'PUT' ? mutation : Promise.resolve(path.includes('/plans') ? [{plan}] : {enabled:true,configured:true}))
    const app = setup('admin', request); await app.load()
    document.querySelector<HTMLButtonElement>('[data-vip-edit="7"]')!.click()
    const dialog = document.querySelector('#vip-plan-dialog')!
    const form = dialog.querySelector<HTMLFormElement>('form')!
    form.dispatchEvent(new Event('submit', {bubbles:true,cancelable:true}))
    form.dispatchEvent(new Event('submit', {bubbles:true,cancelable:true}))
    expect(request.mock.calls.filter(call => call[1]?.method === 'PUT')).toHaveLength(1)
    expect(dialog).toHaveAttribute('data-dialog-busy', 'true')
    expect(dialog.querySelector('[data-dialog-close]')).toBeDisabled()
    finish({})
    await vi.waitFor(() => expect(app.closeDialog).toHaveBeenCalledWith('vip-plan-dialog'))
    expect(dialog.hasAttribute('data-dialog-busy')).toBe(false)
  })
  it('reveals and focuses an invalid field in an inactive editor step before writing', async () => {
    const request = vi.fn(async (path: string) => path.includes('/plans') ? [{plan}] : {enabled:true,configured:true})
    const app = setup('admin', request); await app.load()
    document.querySelector<HTMLButtonElement>('[data-vip-edit="7"]')!.click()
    const form = document.querySelector<HTMLFormElement>('[data-vip-plan-form]')!
    const limit = form.elements.namedItem('max_purchase_per_user') as HTMLInputElement
    limit.value = '-1'
    request.mockClear()
    form.dispatchEvent(new Event('submit', {bubbles:true,cancelable:true}))
    expect(document.querySelector('[data-vip-editor-tab="publication"]')).toHaveAttribute('aria-selected', 'true')
    expect(limit).toHaveFocus()
    expect(document.querySelector('[data-vip-dialog-feedback]')).toHaveAttribute('role', 'alert')
    expect(request).not.toHaveBeenCalled()
  })
  it('does not replace unchanged cards when refreshing their data', async () => {
    const request = vi.fn(async (path: string) => path.includes('/plans') ? [{plan}] : {enabled:true,configured:true})
    const app = setup('admin', request); await app.load()
    const card = document.querySelector('[data-vip-plan="7"]')
    await app.load()
    expect(document.querySelector('[data-vip-plan="7"]')).toBe(card)
  })
  it('discards an older member lookup after a newer user lookup finishes', async () => {
    let first!: (value: unknown) => void
    let second!: (value: unknown) => void
    const firstLookup = new Promise(resolve => {first = resolve})
    const secondLookup = new Promise(resolve => {second = resolve})
    const request = vi.fn((path: string) => {
      if (path.endsWith('/users/1/subscriptions')) return firstLookup
      if (path.endsWith('/users/2/subscriptions')) return secondLookup
      return Promise.resolve(path.includes('/plans') ? [{plan}] : {enabled:true,configured:true})
    })
    const app = setup('admin', request); await app.load()
    const form = document.querySelector<HTMLFormElement>('[data-vip-member-form]')!
    const user = form.elements.namedItem('user_id') as HTMLInputElement
    user.value = '1'; form.dispatchEvent(new Event('submit', {bubbles:true,cancelable:true}))
    user.value = '2'; form.dispatchEvent(new Event('submit', {bubbles:true,cancelable:true}))
    second([{plan_title:'用户二的权益', subscription:{id:2,status:'cancelled',end_time:1}}])
    await vi.waitFor(() => expect(document.querySelector('[data-vip-members]')).toHaveTextContent('用户二的权益'))
    first([{plan_title:'用户一的旧权益', subscription:{id:1,status:'cancelled',end_time:1}}])
    await Promise.resolve()
    expect(document.querySelector('[data-vip-members]')).not.toHaveTextContent('用户一的旧权益')
    expect(document.querySelector('[data-vip-members]')).toHaveAttribute('data-user-id', '2')
  })
  it('reads the saved plan after a concurrent older refresh has completed', async () => {
    let readCount = 0, finishOld!: (value: unknown) => void
    const oldRefresh = new Promise(resolve => {finishOld = resolve})
    const updated = {...plan,title:'已更新的套餐'}
    const request = vi.fn((path: string, options?: {method: string}) => {
      if (options?.method === 'PUT') return Promise.resolve({})
      if (path === '/api/subscription/admin/plans') {
        readCount++
        if (readCount === 2) return oldRefresh
        return Promise.resolve([{plan:readCount > 2 ? updated : plan}])
      }
      return Promise.resolve({enabled:true,configured:true})
    })
    const app = setup('admin', request); await app.load()
    const pendingRefresh = app.load()
    document.querySelector<HTMLButtonElement>('[data-vip-edit="7"]')!.click()
    const form = document.querySelector<HTMLFormElement>('[data-vip-plan-form]')!
    ;(form.elements.namedItem('title') as HTMLInputElement).value = updated.title
    form.dispatchEvent(new Event('submit', {bubbles:true,cancelable:true}))
    await vi.waitFor(() => expect(app.closeDialog).toHaveBeenCalled())
    finishOld([{plan}]); await pendingRefresh
    await vi.waitFor(() => expect(document.querySelector('[data-vip-plan="7"]')).toHaveTextContent(updated.title))
    expect(readCount).toBe(3)
  })
})
