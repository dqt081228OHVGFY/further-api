import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { afterEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(import.meta.dirname, '../../../../relayfor-ui/assets/vip-workspace.js'), 'utf8')
function setup(page: 'admin' | 'console', request: ReturnType<typeof vi.fn>) {
  document.body.dataset.page = page
  document.body.dataset.gatewayRole = page === 'admin' ? '100' : '1'
  document.body.innerHTML = '<nav class="nav-rail"><button data-nav="subscriptions">订阅</button></nav><main class="content-area"></main>'
  const toast = vi.fn()
  const confirm = vi.fn().mockResolvedValue(true)
  const date = vi.fn((value:number)=>String(value))
  Object.defineProperty(window, 'ProxyLan', {configurable:true,value:{
    request, toast, escapeHTML:(value:string)=>value.replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('"','&quot;'), number:String,
    moneyFromQuota:(value:number)=>'$'+(value/500000).toFixed(2), usdFromQuota:(value:number)=>value/500000, quotaFromUSD:(value:number)=>Math.round(value*500000),
    date, openDialog:vi.fn(), closeDialog:vi.fn(), showBusy:vi.fn(), confirm,
  }})
  const registration = vi.spyOn(document, 'addEventListener')
  window.eval(runtime)
  document.dispatchEvent(new Event('relayfor:ready'))
  for(const [name, listener] of registration.mock.calls) { if(name === 'relayfor:ready') document.removeEventListener(name, listener) }
  registration.mockRestore()
  return {toast,confirm,date,load:(options?: {reuse: boolean})=>(window as unknown as {ProxyLanVIP:{load:(options?: {reuse: boolean})=>Promise<void>}}).ProxyLanVIP.load(options)}
}
afterEach(()=>{document.body.replaceChildren(); vi.restoreAllMocks()})
const plan={id:7,title:'VIP Studio',subtitle:'模型分组权益',price_amount:2.5,currency:'USD',duration_unit:'day',duration_value:30,total_amount:5000000,quota_reset_period:'monthly',upgrade_group:'vip',enabled:true,allow_balance_pay:true,stripe_price_id:'existing-price',creem_product_id:'existing-product',custom_seconds:43200}
describe('VIP native card workspace',()=>{
  it('renders admin plans while payment availability is still pending',async()=>{
    let finishPayment!: (value: unknown) => void
    const payment = new Promise(resolve => {finishPayment = resolve})
    const request=vi.fn((path:string)=>path.includes('/plans')?Promise.resolve([{plan}]):payment)
    const app=setup('admin',request)
    await app.load()
    expect(document.querySelector('[data-vip-plan="7"]')).not.toBeNull()
    expect(document.querySelector<HTMLButtonElement>('[data-vip-create]')?.disabled).toBe(false)
    expect(document.querySelector('[data-panel="vip"]')?.hasAttribute('aria-busy')).toBe(false)
    expect(document.querySelector('[data-vip-availability]')?.textContent).toContain('正在读取支付状态')
    finishPayment({enabled:true,configured:true})
    await vi.waitFor(()=>expect(document.querySelector('[data-vip-availability]')?.classList.contains('hidden')).toBe(true))
  })
  it('keeps plan management available when the payment status request fails',async()=>{
    const request=vi.fn(async(path:string)=>{if(path.includes('/plans'))return [{plan}];throw new Error('payment unavailable')})
    const app=setup('admin',request); await app.load()
    await vi.waitFor(()=>expect(document.querySelector('[data-vip-availability]')?.textContent).toContain('支付状态暂时无法读取'))
    expect(document.querySelector('[data-vip-plan="7"]')).not.toBeNull()
    expect(document.querySelector<HTMLButtonElement>('[data-vip-create]')?.disabled).toBe(false)
  })
  it('reuses recent navigation data without replacing cards but refreshes on demand and after expiry',async()=>{
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
    now += 30001
    await app.load({reuse:true})
    expect(request).toHaveBeenCalledWith('/api/subscription/admin/plans')
  })
  it('renders actual prices, duration and quota in cards',async()=>{
    const request=vi.fn(async(path:string)=>path.includes('/plans')?[{plan}]:{enabled:true,configured:true})
    const app=setup('admin',request); await app.load()
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('$2.50')
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('30 天')
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('$10.00')
    expect(document.querySelector('[data-vip-plan="7"]')?.textContent).toContain('vip')
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
    await vi.waitFor(()=>expect(request).toHaveBeenCalledWith('/api/subscription/admin/plans/7',expect.objectContaining({method:'PUT',body:{plan:expect.objectContaining({price_amount:0,sort_order:0,custom_seconds:43200})}})))
    expect(form.querySelector('[name="stripe_price_id"], [name="creem_product_id"], [name="allow_balance_pay"]')).toBeNull()
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
    expect(app.date).toHaveBeenCalledWith(end*1000)
  })
})

