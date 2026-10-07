import { useState } from 'react'
import { asRow, write } from './api'
import { Modal, Notice } from './ui'
import { message } from './workspace-resource'

export function PaymentActivation({ confirmed, disabled, updated }: {
  confirmed: boolean; disabled: boolean; updated: () => Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [accepted, setAccepted] = useState(false)
  const [busy, setBusy] = useState(false)
  const [completed, setCompleted] = useState(false)
  const [error, setError] = useState('')
  const activate = async () => {
    if (!accepted || busy) return
    setBusy(true); setError('')
    try {
      const response = asRow(await write('/api/option/payment_compliance', 'POST', { confirmed: true }))
      if (response.confirmed !== true) throw new Error('收款确认未完成，请重试。')
      await updated(); setCompleted(true)
    } catch (cause) { setError(message(cause)) }
    finally { setBusy(false) }
  }
  return <div className='rfw-card' style={{ marginBottom: 22 }}><h2>{confirmed ? '已确认收款服务规则' : '完成收款确认'}</h2><p>{confirmed ? '完成下方配置后，可用渠道会自动出现在充值与支付页面。' : '外部支付渠道需要站点所有者先确认收费服务责任，才能向用户开放。'}</p>{!confirmed && <div className='rfw-toolbar'><button type='button' className='rfw-primary' disabled={disabled} onClick={() => { setCompleted(false); setAccepted(false); setError(''); setOpen(true) }}>查看并确认</button></div>}
    {open && <Modal title='确认收款服务规则' busy={busy} dismiss={completed} close={() => setOpen(false)}><p>作为站点所有者，你负责收费服务的交付、订单核对和退款处理，并确保收款用途符合当地规定和支付服务条款。</p><label className='rfw-switch'><input type='checkbox' checked={accepted} disabled={busy} onChange={event => setAccepted(event.target.checked)} />我已了解并确认上述收款责任</label><Notice error={error} /><div className='rfw-actions'><button type='button' data-modal-close disabled={busy}>返回</button><button type='button' className='rfw-primary' disabled={busy || !accepted} onClick={() => void activate()}>{busy ? '正在确认…' : '确认收款规则'}</button></div></Modal>}
  </div>
}
