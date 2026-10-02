import { useEffect, useRef, useState } from 'react'
import { Copy, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useSystemConfig } from '@/hooks/use-system-config'
import { getLdcConfig, saveLdcConfig, sitePaymentLogo, type LdcConfig } from '@/features/wallet/ldc-api'

export function PaymentSettingsSection() {
  const { logo } = useSystemConfig()
  const [config, setConfig] = useState<LdcConfig | null>(null)
  const [secret, setSecret] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)
  const edits = useRef(0)
  const locked = useRef(false)
  async function load() {
    if (locked.current) return
    locked.current = true; setBusy(true); setError('')
    try { setConfig(await getLdcConfig()); setSecret(''); setPrivateKey(''); setSaved(false) }
    catch (cause) { setError(cause instanceof Error ? cause.message : '读取 LDC 配置失败') }
    finally { setBusy(false); locked.current = false }
  }
  useEffect(() => { void load() }, [])
  function update<K extends keyof LdcConfig>(key: K, value: LdcConfig[K]) {
    edits.current++; setSaved(false); setConfig(current => current ? { ...current, [key]: value } : current)
  }
  async function save() {
    if (!config || locked.current) return
    if (!config.client_id.trim() || !Number.isFinite(Number(config.credits_per_unit)) || Number(config.credits_per_unit) <= 0 || !Number.isFinite(Number(config.min_topup)) || Number(config.min_topup) <= 0) { setError('请填写应用 ID、有效的兑换比例和最低充值金额'); return }
    try { const url = new URL(config.callback_base_url); if (!['https:', 'http:'].includes(url.protocol) || (url.pathname !== '/' && url.pathname !== '') || url.search || url.hash) throw Error() } catch { setError('回调站点地址应为完整域名，如 https://relay.example.com'); return }
    locked.current = true; setBusy(true); setError(''); setSaved(false)
    const version = edits.current
    try {
      const next = await saveLdcConfig({ enabled: config.enabled, client_id: config.client_id.trim(), credits_per_unit: config.credits_per_unit, min_topup: config.min_topup, callback_base_url: config.callback_base_url.trim(), ...(secret ? { client_secret: secret } : {}), ...(privateKey ? { private_key: privateKey } : {}) })
      if (version === edits.current) { setConfig(next); setSecret(''); setPrivateKey(''); setSaved(true) }
      toast.success('LDC 支付配置已保存')
    } catch (cause) { setError(cause instanceof Error ? cause.message : '保存 LDC 配置失败') }
    finally { setBusy(false); locked.current = false }
  }

  return <section className='max-w-4xl space-y-6'>
    <header className='flex items-start justify-between gap-4 border-b pb-6'><div className='flex items-center gap-3'><img src={sitePaymentLogo(logo)} alt='' className='h-12 w-12 rounded-xl object-contain' /><div><p className='text-muted-foreground text-xs tracking-[.18em]'>RELAYFOR / PAYMENTS</p><h2 className='mt-1 text-2xl font-semibold'>LDC 支付</h2><p className='text-muted-foreground mt-1 text-sm'>充值与订阅使用同一套 LDC 应用配置。</p></div></div><Button variant='outline' disabled={busy} onClick={() => void load()}><RefreshCw className='mr-2 h-4 w-4' />重新读取</Button></header>
    {error && <p role='alert' className='text-destructive rounded-lg border p-4 text-sm'>{error}</p>}
    {config ? <form className='space-y-6' onSubmit={event => { event.preventDefault(); void save() }}>
      <div className='flex items-center justify-between rounded-xl border p-5'><div><Label htmlFor='ldc-enabled' className='text-base'>开放 LDC 支付</Label><p className='text-muted-foreground mt-1 text-sm'>{config.configured ? '应用已配置，启用后用户可充值和开通套餐。' : '保存应用配置并在 LDC 上传商户公钥后启用。'}</p></div><Switch id='ldc-enabled' checked={config.enabled} onCheckedChange={value => update('enabled', value)} /></div>
      <div className='grid gap-5 md:grid-cols-2'>
        <div className='space-y-2'><Label htmlFor='ldc-client-id'>LDC 应用 ID</Label><Input id='ldc-client-id' value={config.client_id} autoComplete='off' onChange={event => update('client_id', event.target.value)} required /></div>
        <div className='space-y-2'><Label htmlFor='ldc-client-secret'>应用密钥</Label><Input id='ldc-client-secret' type='password' autoComplete='new-password' value={secret} placeholder={config.secret_configured ? '已配置，留空保留现有密钥' : '填写 LDC 应用密钥'} onChange={event => { edits.current++; setSecret(event.target.value); setSaved(false) }} /><p className='text-muted-foreground text-xs'>密钥保存在服务端，页面不回显。</p></div>
        <div className='space-y-2'><Label htmlFor='ldc-rate'>1 USD 账户额度对应 LDC</Label><Input id='ldc-rate' inputMode='decimal' value={config.credits_per_unit} onChange={event => update('credits_per_unit', event.target.value)} required /></div>
        <div className='space-y-2'><Label htmlFor='ldc-minimum'>最低充值（USD）</Label><Input id='ldc-minimum' inputMode='decimal' value={config.min_topup} onChange={event => update('min_topup', event.target.value)} required /></div>
        <div className='space-y-2 md:col-span-2'><Label htmlFor='ldc-callback'>站点外部访问地址</Label><Input id='ldc-callback' type='url' placeholder='https://relay.example.com' value={config.callback_base_url} onChange={event => update('callback_base_url', event.target.value)} required /></div>
      </div>
      <div className='space-y-4 rounded-xl border bg-muted/20 p-5'><div><h3 className='font-medium'>商户签名与回调</h3><p className='text-muted-foreground mt-1 text-sm'>首次保存会生成 Ed25519 签名密钥。将下面的商户公钥填写到 LDC 应用，付款回调会自动验签。</p></div><div className='space-y-2'><div className='flex items-center justify-between'><Label htmlFor='ldc-public-key'>商户公钥</Label><Button type='button' size='sm' variant='ghost' disabled={!config.public_key} onClick={async () => { try { await navigator.clipboard.writeText(config.public_key); toast.success('公钥已复制') } catch { setError('复制失败，请手动选中公钥复制') } }}><Copy className='mr-2 h-3.5 w-3.5' />复制</Button></div><textarea id='ldc-public-key' className='bg-background min-h-24 w-full resize-y rounded-md border p-3 font-mono text-xs' readOnly value={config.public_key} placeholder='首次保存后生成' /></div><dl className='space-y-2 break-all text-sm'><div><dt className='text-muted-foreground'>异步通知地址</dt><dd className='mt-1 font-mono text-xs'>{config.notify_url || '保存后生成'}</dd></div><div><dt className='text-muted-foreground'>支付返回地址</dt><dd className='mt-1 font-mono text-xs'>{config.return_url || '保存后生成'}</dd></div></dl><details><summary className='cursor-pointer text-sm'>迁移已有商户签名私钥</summary><div className='mt-3 space-y-2'><Label htmlFor='ldc-private-key'>私钥（可选）</Label><Input id='ldc-private-key' type='password' autoComplete='new-password' value={privateKey} placeholder={config.private_key_configured ? '已配置，留空保留' : '留空自动生成'} onChange={event => { edits.current++; setPrivateKey(event.target.value); setSaved(false) }} /></div></details></div>
      <div className='flex items-center justify-end gap-4 border-t pt-5'><span role='status' className='text-muted-foreground text-sm'>{saved ? '已保存' : busy ? '正在保存…' : ''}</span><Button disabled={busy} type='submit'>保存 LDC 配置</Button></div>
    </form> : <p role='status' className='text-muted-foreground'>{busy ? '正在读取配置…' : '配置暂不可用，请重新读取。'}</p>}
  </section>
}

