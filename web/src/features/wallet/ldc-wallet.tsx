import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowUpRight, History, RefreshCw } from 'lucide-react'

import { Dialog } from '@/components/dialog'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSystemConfig } from '@/hooks/use-system-config'
import { getSelf } from '@/lib/api'
import { formatQuota } from '@/lib/format'
import { useNativeFeatureIsland } from '@/relayfor/native-feature-context'

import { AffiliateRewardsCard } from './components/affiliate-rewards-card'
import { BillingHistoryDialog } from './components/dialogs/billing-history-dialog'
import { TransferDialog } from './components/dialogs/transfer-dialog'
import { SubscriptionPlansCard } from './components/subscription-plans-card'
import { WalletStatsCard } from './components/wallet-stats-card'
import { useAffiliate } from './hooks/use-affiliate'
import { useRedemption } from './hooks/use-redemption'
import { getLdcInfo, getLdcOrders, payLdc, quoteLdc, sitePaymentLogo, submitLdcOrder, type LdcInfo, type LdcQuote, type LdcOrderRecord } from './ldc-api'
import { clearWalletHistoryRequest } from './lib/history-url'
import type { UserWalletData } from './types'

export function Wallet({ initialShowHistory = false }: { initialShowHistory?: boolean }) {
  const { logo } = useSystemConfig()
  const island = useNativeFeatureIsland()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [info, setInfo] = useState<LdcInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [amount, setAmount] = useState('')
  const [quote, setQuote] = useState<(LdcQuote & { requestedAmount: string }) | null>(null)
  const [busy, setBusy] = useState(false)
  const lock = useRef(false)
  const [error, setError] = useState('')
  const [orders, setOrders] = useState<LdcOrderRecord[]>([])
  const [ordersError, setOrdersError] = useState('')
  const [historyOpen, setHistoryOpen] = useState(initialShowHistory)
  const [transferOpen, setTransferOpen] = useState(false)
  const [redemption, setRedemption] = useState('')
  const { redeemCode, redeeming } = useRedemption()
  const { affiliateLink, loading: affiliateLoading, transferQuota, transferring } = useAffiliate()

  const refreshUser = useCallback(async () => {
    const response = await getSelf()
    if (response.success && response.data) setUser(response.data as UserWalletData)
  }, [])
  const refresh = useCallback(async () => {
    setLoading(true); setError(''); setInfo(null)
    try {
      const [next] = await Promise.all([getLdcInfo(), refreshUser()])
      setInfo(next); setAmount(current => current || next.min_topup || '1')
    } catch (cause) { setError(cause instanceof Error ? cause.message : '充值信息读取失败') }
    finally { setLoading(false) }
  }, [refreshUser])
  const refreshOrders = useCallback(async () => {
    setOrdersError('')
    try { setOrders(await getLdcOrders()) }
    catch (cause) { setOrdersError(cause instanceof Error ? cause.message : '订单读取失败') }
  }, [])
  useEffect(() => { void refresh() }, [refresh])
  useEffect(() => { void refreshOrders() }, [refreshOrders])
  useEffect(() => {
    if (initialShowHistory) { setHistoryOpen(true); clearWalletHistoryRequest(island) }
  }, [initialShowHistory, island])

  async function requestQuote() {
    if (lock.current || !info?.enabled || !info.configured) return
    if (!/^(?:\d+)(?:\.\d+)?$/.test(amount) || Number(amount) <= 0 || Number(amount) < Number(info.min_topup) || (info.max_topup && Number(amount) > Number(info.max_topup))) {
      setError('请输入充值范围内的有效金额'); return
    }
    lock.current = true; setBusy(true); setError('')
    try { setQuote({ ...await quoteLdc({ amount }), requestedAmount: amount }) }
    catch (cause) { setError(cause instanceof Error ? cause.message : '暂时无法获取报价') }
    finally { lock.current = false; setBusy(false) }
  }
  async function pay() {
    if (!quote || lock.current) return
    lock.current = true; setBusy(true); setError('')
    try {
      const order = await payLdc({ amount: quote.requestedAmount, expected_credits: quote.credits })
      submitLdcOrder(order)
    } catch (cause) { setError(cause instanceof Error ? cause.message : '未能创建 LDC 订单，请重试') }
    finally { lock.current = false; setBusy(false) }
  }

  return <>
    <SectionPageLayout>
      <SectionPageLayout.Title>钱包与 LDC 充值</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-5'>
          <WalletStatsCard user={user} loading={loading} />
          <div className='grid items-start gap-5 xl:grid-cols-[minmax(0,1.05fr)_minmax(340px,.95fr)]'>
            <Card className='overflow-hidden'>
              <CardHeader className='border-b bg-muted/30 p-6'>
                <div className='flex items-start justify-between gap-4'>
                  <div className='flex items-center gap-3'><img src={sitePaymentLogo(logo)} alt='' className='h-11 w-11 rounded-xl object-contain' /><div><p className='text-muted-foreground text-xs tracking-[.18em]'>RELAYFOR / LDC</p><CardTitle className='mt-1 text-xl'>为下一次创造补充额度</CardTitle></div></div>
                  <Button variant='ghost' size='icon' aria-label='刷新充值信息' disabled={busy || loading} onClick={() => void refresh()}><RefreshCw className='h-4 w-4' /></Button>
                </div>
                <p className='text-muted-foreground mt-2 text-sm'>使用 LDC 完成付款，到账后即可用于 API 调用。</p>
              </CardHeader>
              <CardContent className='space-y-5 p-6'>
                {error && <p role='alert' className='text-destructive text-sm'>{error}</p>}
                {!loading && (!info?.enabled || !info.configured) && <p role='status' className='rounded-lg border p-4 text-sm'>LDC 充值尚未开放，请稍后再来。</p>}
                <div className='space-y-2'><Label htmlFor='ldc-wallet-amount'>充值额度（{info?.wallet_unit || 'USD'}）</Label><Input id='ldc-wallet-amount' inputMode='decimal' value={amount} disabled={busy || loading} onChange={event => { setAmount(event.target.value); setQuote(null) }} /><p className='text-muted-foreground text-xs'>最低 {info?.min_topup || '—'} {info?.wallet_unit || 'USD'}{info?.max_topup ? `，最高 ${info.max_topup}` : ''} · 1 {info?.wallet_unit || 'USD'} = {info?.credits_per_unit || '—'} LDC</p></div>
                <div className='flex flex-wrap gap-2'>{(info?.amount_options || []).map(preset => <Button key={preset} variant={amount === preset ? 'default' : 'outline'} disabled={busy} onClick={() => { setAmount(String(preset)); setQuote(null) }}>{preset} {info?.wallet_unit || 'USD'}</Button>)}</div>
                <Button className='w-full' size='lg' disabled={busy || loading || !info?.enabled || !info.configured} onClick={() => void requestQuote()}>{busy ? '正在读取报价…' : '查看 LDC 报价'}<ArrowUpRight className='ml-2 h-4 w-4' /></Button>
                <p className='text-muted-foreground text-xs'>确认报价后前往 LDC 付款。到账状态以订单记录为准。</p>
                <div className='border-t pt-5'><div className='mb-3 flex items-center justify-between'><Label htmlFor='wallet-redemption'>兑换码</Label><Button size='sm' variant='ghost' onClick={() => setHistoryOpen(true)}><History className='mr-2 h-4 w-4' />订单与退款记录</Button></div><div className='flex gap-2'><Input id='wallet-redemption' value={redemption} onChange={event => setRedemption(event.target.value)} placeholder='输入额度或订阅兑换码' /><Button variant='outline' disabled={!redemption.trim() || redeeming} onClick={async () => { if (await redeemCode(redemption.trim())) { setRedemption(''); await refreshUser() } }}>{redeeming ? '兑换中…' : '兑换'}</Button></div></div>
              </CardContent>
            </Card>
            <SubscriptionPlansCard onPurchaseSuccess={refreshUser} />
          </div>
          <Card><CardHeader className='flex flex-row items-center justify-between'><div><CardTitle>LDC 订单</CardTitle><p className='text-muted-foreground mt-1 text-sm'>查看最近 100 笔充值与订阅支付的实际状态。</p></div><Button variant='outline' onClick={() => void refreshOrders()}><RefreshCw className='mr-2 h-4 w-4' />刷新订单</Button></CardHeader><CardContent>{ordersError ? <p role='alert' className='text-destructive text-sm'>{ordersError}</p> : orders.length ? <div className='divide-y'>{orders.map(order => <div key={order.trade_no} className='flex flex-wrap items-center justify-between gap-3 py-4'><div><p className='font-medium'>{order.kind === 'subscription' ? order.plan_title || '订阅套餐' : `充值 ${order.wallet_units} USD`}</p><p className='text-muted-foreground mt-1 break-all font-mono text-xs'>{order.trade_no}</p></div><div className='text-right'><p className='font-semibold'>{order.credits} LDC</p><p className='text-muted-foreground mt-1 text-xs'>{({ pending: '待支付', success: '已到账', expired: '已过期', refunded: '已退款', failed: '支付失败', cancelled: '已取消' } as Record<string, string>)[order.status] || order.status}</p></div></div>)}</div> : <p className='text-muted-foreground py-4 text-sm'>还没有 LDC 订单。</p>}</CardContent></Card>
          <AffiliateRewardsCard user={user} affiliateLink={affiliateLink} onTransfer={() => setTransferOpen(true)} complianceConfirmed loading={affiliateLoading} />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
    <Dialog open={quote !== null} onOpenChange={open => { if (!open && !busy) setQuote(null) }} title='确认 LDC 充值' contentHeight='auto' bodyClassName='space-y-5'>
      <dl className='space-y-3 rounded-xl border p-5'><div className='flex justify-between'><dt>账户获得额度</dt><dd>{quote ? formatQuota(quote.quota) : '—'}</dd></div><div className='flex items-center justify-between border-t pt-3'><dt>本次支付</dt><dd className='text-primary text-2xl font-semibold'>{quote?.credits} LDC</dd></div></dl>
      {error && <p role='alert' className='text-destructive text-sm'>{error}</p>}
      <p className='text-muted-foreground text-sm'>前往 LDC 确认支付，完成后返回查看到账状态。</p>
      <div className='flex justify-end gap-2'><Button variant='outline' disabled={busy} onClick={() => setQuote(null)}>取消</Button><Button disabled={busy || !quote} onClick={() => void pay()}><img src={sitePaymentLogo(logo)} alt='' className='mr-2 h-5 w-5 object-contain' />{busy ? '正在创建订单…' : '确认并前往 LDC'}</Button></div>
    </Dialog>
    <BillingHistoryDialog open={historyOpen} onOpenChange={setHistoryOpen} />
    <TransferDialog open={transferOpen} onOpenChange={setTransferOpen} availableQuota={user?.aff_quota ?? 0} transferring={transferring} onConfirm={async value => { const result = await transferQuota(value); if (result) await refreshUser(); return result }} />
  </>
}


