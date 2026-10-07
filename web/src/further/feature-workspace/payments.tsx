import { ArrowUpRight, CreditCard, Receipt, Wallet } from 'lucide-react'
import { useState } from 'react'
import { asRow, asRows, label, pageData, read, write, type Row } from './api'
import { checkoutValue, paymentHistory, type Checkout } from './checkout-values'
import { dateTime } from './operation-values'
import { Empty, Modal, Notice, Page } from './ui'
import { message, useResource } from './workspace-resource'
import './payments.css'

const providers: Record<string, string> = {
  epay: '在线支付', stripe: '银行卡 · Stripe', creem: '银行卡 · Creem',
  waffo: 'Waffo', waffo_pancake: 'Waffo Pancake', ldc: 'LINUX DO Credit',
}
const statuses: Record<string, string> = {
  pending: '等待付款', success: '已到账', completed: '已到账', failed: '支付失败',
  expired: '订单已过期', canceled: '已取消', cancelled: '已取消', refunded: '已退款',
}
function methodKey(method: Row) { return `${String(method.provider)}:${String(method.type)}` }

export function PaymentsScreen() {
  const info = useResource(async () => asRow(await read('/api/user/topup/info')))
  const [page, setPage] = useState(1)
  const history = useResource(async () => {
    const [external, ldc] = await Promise.all([
      read('/api/user/topup/self', { p: page, page_size: 20 }), read('/api/user/ldc/orders'),
    ])
    const rows = pageData(external)
    return { items: paymentHistory(rows.items, page === 1 ? asRows(ldc) : []), total: rows.total }
  }, [page])
  const [selected, setSelected] = useState('')
  const [amount, setAmount] = useState('10')
  const [productID, setProductID] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [order, setOrder] = useState<Checkout>()
  const [quote, setQuote] = useState<Row>()
  const methods = asRows(info.data?.pay_methods).filter(item => providers[String(item.provider)] && item.type)
  const active = methods.find(item => methodKey(item) === selected) || methods[0]
  const provider = String(active?.provider || '')
  const products = asRows(info.data?.creem_products)
  const product = products.find(item => String(item.productId) === productID) || products[0]
  const minimum = Math.max(Number(active?.min_topup) || 1, 0.01)
  let submitCaption = '创建支付订单'
  if (provider === 'ldc') submitCaption = '核对充值报价'
  if (busy) submitCaption = '正在处理…'
  const create = async () => {
    if (busy || !active) return
    if (provider !== 'creem' && (!/^\d+(\.\d{1,6})?$/.test(amount) || Number(amount) < minimum ||
        (provider === 'stripe' && !Number.isInteger(Number(amount))))) {
      setError(`请填写不低于 ${minimum} 的有效充值额度${provider === 'stripe' ? '，银行卡充值须为整数' : ''}。`)
      return
    }
    setBusy(true); setError('')
    try {
      if (provider === 'ldc') {
        const next = asRow(await write('/api/user/ldc/quote', 'POST', { amount }))
        if (next.currency !== 'LDC' || next.kind !== 'wallet' || !/^\d+\.\d{2}$/.test(String(next.credits)) ||
            Number(next.credits) <= 0 || Number(next.wallet_units) !== Number(amount)) throw new Error('充值报价无效，请重新读取。')
        setQuote(next)
      } else {
        const next = await write('/api/user/checkout', 'POST', {
          provider, method: active.type, amount: provider === 'creem' ? String(product?.quota || '') : amount,
          ...(provider === 'creem' ? { product_id: product?.productId } : {}),
          return_url: '/wallet',
        })
        setOrder(checkoutValue(next, location.origin))
        void history.reload()
      }
    } catch (cause) { setError(message(cause)) }
    finally { setBusy(false) }
  }
  const confirmLDC = async () => {
    if (busy || !quote) return
    setBusy(true); setError('')
    try {
      const next = await write('/api/user/ldc/pay', 'POST', { amount: quote.wallet_units, expected_credits: quote.credits })
      setOrder(checkoutValue(next, location.origin, true))
      setQuote(undefined); void history.reload()
    } catch (cause) { setError(message(cause)) }
    finally { setBusy(false) }
  }
  return <Page title='充值与支付' description='选择支付方式补充账户额度，在这里核对订单和到账状态。'
    loading={info.loading} error={info.error} reload={async () => { await Promise.all([info.reload(), history.reload()]) }}>
    <div className='rfw-payment-layout'>
      <section className='rfw-card rfw-payment-entry'>
        <div className='rfw-payment-title'><Wallet size={23} /><div><h2>补充账户额度</h2><p>账户额度统一按 USD 计量。</p></div></div>
        {!methods.length ? <Empty>暂未开放在线充值。</Empty> : <form onSubmit={event => { event.preventDefault(); void create() }}>
          <fieldset disabled={busy || info.loading}>
            <legend>支付方式</legend>
            <div className='rfw-payment-methods'>{methods.map(method => <label key={methodKey(method)} className={active === method ? 'selected' : ''}>
              <input type='radio' name='payment-method' checked={active === method} onChange={() => {
                setSelected(methodKey(method)); setAmount(String(Math.max(10, Number(method.min_topup) || 1))); setError('')
              }} /><CreditCard size={19} /><span><b>{String(method.name || providers[String(method.provider)])}</b><small>{providers[String(method.provider)]} · {String(method.currency)}</small></span>
            </label>)}</div>
            {provider === 'creem' ? <div className='rfw-field'><label htmlFor='wallet-product'>充值套餐</label><select id='wallet-product' value={String(product?.productId || '')} onChange={event => setProductID(event.target.value)}>
              {products.map(item => <option key={String(item.productId)} value={String(item.productId)}>{String(item.quota)} USD 额度 · {String(item.price)} {String(item.currency)}</option>)}
            </select></div> : <div className='rfw-field'><label htmlFor='wallet-amount'>充值额度（USD）</label><input id='wallet-amount' inputMode='decimal' value={amount} onChange={event => setAmount(event.target.value)} required /><small>最低充值 {minimum} USD。{provider === 'stripe' ? '请输入整数。' : '实付金额以订单确认为准。'}</small></div>}
            <Notice error={!quote && !order ? error : ''} />
            <button type='submit' className='rfw-primary' disabled={provider === 'creem' && !product}>{submitCaption}<ArrowUpRight size={17} /></button>
          </fieldset>
        </form>}
      </section>
      <aside className='rfw-card rfw-payment-guide'><Receipt size={25} /><h2>付款与到账</h2><p>核对订单金额后，前往所选支付服务完成付款。到账以订单记录为准。</p><ol><li>选择充值额度和支付方式</li><li>核对订单，完成付款</li><li>返回此页刷新到账状态</li></ol><a href='/console/#redeem'>已有兑换码，前往兑换 <ArrowUpRight size={16} /></a></aside>
    </div>
    <section className='rfw-payment-history'><header><h2>订单记录</h2><button type='button' className='rfw-secondary' disabled={history.loading} onClick={() => void history.reload()}>刷新订单</button></header>
      <Notice error={history.error} />
      {history.loading && !history.data ? <Empty>正在读取订单…</Empty> : <div className='rfw-table-wrap'><table className='rfw-table'><thead><tr><th>订单</th><th>充值额度</th><th>实付</th><th>状态</th></tr></thead><tbody>
        {(history.data?.items || []).map(item => <tr key={String(item.trade_no)}><td><b>{label(item.trade_no)}</b><small>{dateTime(item.created)} · {providers[String(item.provider)] || label(item.provider)}</small></td><td>{item.kind === 'subscription' ? label(item.plan_title) : `${label(item.units)} USD`}</td><td>{label(item.cost)} {label(item.currency)}</td><td>{statuses[String(item.status)] || label(item.status)}</td></tr>)}
        {!history.data?.items.length && <tr><td colSpan={4}>还没有支付订单。</td></tr>}
      </tbody></table></div>}
      {((history.data?.total || 0) > 20 || page > 1) && <div className='rfw-pagination'><button type='button' disabled={page === 1 || history.loading} onClick={() => setPage(page - 1)}>上一页</button><span>第 {page} 页</span><button type='button' disabled={page * 20 >= (history.data?.total || 0) || history.loading} onClick={() => setPage(page + 1)}>下一页</button></div>}
    </section>
    {quote && <Modal title='确认 LDC 充值' busy={busy} close={() => { setQuote(undefined); setError('') }}><Notice error={error} /><div className='rfw-payment-confirm'><span>到账额度</span><strong>{String(quote.wallet_units)} USD</strong><p>实付 {String(quote.credits)} LDC。确认后将创建支付订单。</p></div><div className='rfw-actions'><button type='button' data-modal-close disabled={busy}>返回修改</button><button type='button' className='rfw-primary' disabled={busy} onClick={() => void confirmLDC()}>{busy ? '正在创建…' : '确认创建订单'}</button></div></Modal>}
    {order && <Modal title='订单已创建' close={() => setOrder(undefined)}><div className='rfw-payment-confirm'><span>订单编号</span><b>{order.tradeNo}</b>{order.walletUnits && <strong>{order.walletUnits} USD</strong>}<p>{order.money ? `实付 ${order.money} ${order.currency}。` : ''}完成付款后返回此页刷新到账状态。</p></div><div className='rfw-actions'><button type='button' data-modal-close>稍后支付</button>{order.method === 'GET' ? <a className='rfw-primary' href={order.url}>前往支付 <ArrowUpRight size={17} /></a> : <form action={order.url} method='POST'>{Object.entries(order.params).map(([name, value]) => <input type='hidden' key={name} name={name} value={value} />)}<button type='submit' className='rfw-primary'>前往支付 <ArrowUpRight size={17} /></button></form>}</div></Modal>}
  </Page>
}
