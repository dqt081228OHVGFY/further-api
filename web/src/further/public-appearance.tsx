import { Check, Moon, Palette, Sun } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTheme } from '@/context/theme-provider'
import { getWorkspacePalettePreference, selectWorkspacePalette, setWorkspacePaletteMode } from './appearance-preferences'
import { Modal } from './feature-workspace/ui'

const palettes = [['white', '云白', '轻盈的日常配色'], ['blue', '晴空蓝', '柔和的蓝色工作空间'], ['yellow', '暖阳黄', '温暖明亮的日常配色'], ['duanwu', '端午 · 粽叶青', '竹青与浅绿'], ['qixi', '七夕 · 星河紫', '暮蓝与淡紫'], ['mid_autumn', '中秋 · 月光金', '月白与桂花金'], ['chongyang', '重阳 · 秋菊黄', '秋色与暖金'], ['new_year_eve', '除夕 · 灯火红', '朱红与暖杏'], ['national_day', '国庆 · 山河红', '赤陶与金色']] as const
export function PublicAppearance() {
  const { resolvedTheme, setTheme } = useTheme()
  const [open, setOpen] = useState(false)
  const [preference, setPreference] = useState(getWorkspacePalettePreference)
  useEffect(() => {
    const sync = () => setPreference(getWorkspacePalettePreference())
    window.addEventListener('further-ui:palette', sync)
    sync()
    return () => window.removeEventListener('further-ui:palette', sync)
  }, [])
  const title = palettes.find(([key]) => key === preference.effectivePalette)?.[1] || '晴空蓝'
  return <>
    <div className='further-appearance-controls'>
      <button type='button' aria-label='选择配色' title={preference.mode === 'fixed' ? '配色已固定' : '自动节日配色'} onClick={() => setOpen(true)}><Palette size={19} /></button>
      <button type='button' aria-label={resolvedTheme === 'dark' ? '切换浅色' : '切换深色'} title={resolvedTheme === 'dark' ? '切换浅色' : '切换深色'} onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}>{resolvedTheme === 'dark' ? <Sun size={19} /> : <Moon size={19} />}</button>
    </div>
    {open && <Modal title='你的空间，你的颜色' close={() => setOpen(false)}>
      <p>固定所选配色，跨页面保持一致。自动节日配色只在你开启后生效。</p>
      <div className='further-palette-mode' role='group' aria-label='配色模式'>
        <button type='button' data-palette-mode='fixed' aria-pressed={preference.mode === 'fixed'} onClick={() => setWorkspacePaletteMode('fixed')}><b>固定配色</b><small>保持当前颜色，不自动切换</small></button>
        <button type='button' data-palette-mode='auto' aria-pressed={preference.mode === 'auto'} onClick={() => setWorkspacePaletteMode('auto')}><b>自动节日</b><small>根据站点当前节日换色</small></button>
      </div>
      <div className='further-appearance-options'>{palettes.map(([key, name, hint]) => <button type='button' key={key} data-palette={key} aria-pressed={preference.effectivePalette === key} onClick={() => selectWorkspacePalette(key)}><i aria-hidden='true' /><span><b>{name}</b><small>{hint}</small></span>{preference.effectivePalette === key && <Check size={17} />}</button>)}</div>
      <div className='rfw-actions'><span role='status'>{preference.mode === 'fixed' ? '已固定' : '自动节日'} · {title}</span><button type='button' className='rfw-primary' data-modal-close>完成</button></div>
    </Modal>}
  </>
}
