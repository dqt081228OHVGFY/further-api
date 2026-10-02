import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { useSystemConfig } from '@/hooks/use-system-config'
import { formatQuota } from '@/lib/format'
import { getLdcInfo, payLdc, quoteLdc, sitePaymentLogo, submitLdcOrder, type LdcQuote } from '@/features/wallet/ldc-api'
import { formatDuration } from '../../lib'
import type { PlanRecord } from '../../types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: PlanRecord | null
  purchaseLimit?: number
  purchaseCount?: number
  onPurchaseSuccess?: () => void | Promise<void>
}

export function SubscriptionPurchaseDialog({ open, onOpenChange, plan: record, purchaseLimit = 0, purchaseCount = 0 }: Props) {
  const { t } = useTranslation()
  const { logo } = useSystemConfig()
  const plan = record?.plan
  const [quote, setQuote] = useState<LdcQuote | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [paying, setPaying] = useState(false)
  const locked = useRef(false)
  const limitReached = purchaseLimit > 0 && purchaseCount >= purchaseLimit

  useEffect(() => {
    let active = true
    setQuote(null); setError('')
    if (!open || !plan) return
    setLoading(true)
    void (async () => {
      try {
        const info = await getLdcInfo()
        if (!info.enabled || !info.configured) throw new Error('LDC 支付尚未开放')
        const result = await quoteLdc({ plan_id: plan.id })
        if (active) setQuote(result)
      } catch (cause) { if (active) setError(cause instanceof Error ? cause.message : '套餐报价读取失败') }
      finally { if (active) setLoading(false) }
    })()
    return () => { active = false }
  }, [open, plan?.id])

  async function pay() {
    if (!plan || !quote || Number(quote.plan_id) !== plan.id || limitReached || locked.current) return
    locked.current = true; setPaying(true); setError('')
    try { submitLdcOrder(await payLdc({ plan_id: plan.id, expected_credits: quote.credits })) }
    catch (cause) { setError(cause instanceof Error ? cause.message : '创建 LDC 订单失败') }
    finally { locked.current = false; setPaying(false) }
  }

  if (!plan) return null
  return <Dialog open={open} onOpenChange={value => { if (!paying) onOpenChange(value) }} title='确认开通订阅' contentHeight='auto' bodyClassName='space-y-5'>
    <div className='flex items-center gap-3'><img src={sitePaymentLogo(logo)} alt='' className='h-11 w-11 rounded-xl object-contain' /><div><p className='text-muted-foreground text-xs tracking-[.15em]'>RELAYFOR / LDC</p><h3 className='mt-1 text-lg font-semibold'>{plan.title}</h3></div></div>
    <dl className='space-y-3 rounded-xl border bg-muted/20 p-5'><div className='flex justify-between gap-4'><dt className='text-muted-foreground'>有效期</dt><dd>{formatDuration(plan, t)}</dd></div><div className='flex justify-between gap-4'><dt className='text-muted-foreground'>套餐额度</dt><dd>{Number(plan.total_amount) > 0 ? formatQuota(plan.total_amount) : '不限额'}</dd></div><div className='flex items-center justify-between border-t pt-4'><dt>本次支付</dt><dd className='text-primary text-2xl font-semibold'>{loading ? '报价中…' : quote ? `${quote.credits} LDC` : '—'}</dd></div></dl>
    {error && <p role='alert' className='text-destructive text-sm'>{error}</p>}
    {limitReached && <p role='alert' className='text-destructive text-sm'>已达到此套餐的购买次数上限。</p>}
    <p className='text-muted-foreground text-sm'>使用 LDC 支付。付款完成并确认到账后，套餐权益自动生效。</p>
    <div className='flex justify-end gap-2'><Button variant='outline' disabled={paying} onClick={() => onOpenChange(false)}>取消</Button><Button onClick={() => void pay()} disabled={loading || paying || !quote || limitReached}><img src={sitePaymentLogo(logo)} alt='' className='mr-2 h-5 w-5 object-contain' />{paying ? '正在创建订单…' : '确认并前往 LDC'}</Button></div>
  </Dialog>
}


