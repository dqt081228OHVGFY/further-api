import { Activity, ArrowRight, Check, ChevronDown, KeyRound, Sparkles } from 'lucide-react'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import './welcome-page.css'

export interface WelcomePageProps {
  onComplete: () => void | Promise<void>
  onEntered?: () => void
  displayName?: string
  appearanceControls?: ReactNode
  enterLabel?: string
}

const steps = [
  { icon: KeyRound, title: '准备一把访问密钥', short: '安全接入', description: '为应用创建独立 API Key，调用权限和额度都由你掌握。', detail: '在管理页面的「API Key」中创建密钥并妥善保存。每个应用使用独立密钥，后续管理更方便。', action: '从密钥开始' },
  { icon: Sparkles, title: '发送第一条请求', short: '开始体验', description: '选择可用模型，按照使用文档示例发起第一次调用。', detail: '在「模型与价格」选择可用模型，打开「使用文档」，将站点地址、API Key 和模型名称填入示例并发送请求。', action: '了解调用方式' },
  { icon: Activity, title: '看清每一次使用', short: '掌握用量', description: '调用记录、可用额度与订阅权益，随时在管理页面查看。', detail: '在首页查看可用额度与调用情况。「调用记录」追踪请求，「订阅计划」管理套餐，「活动与签到」和「VIP 权益」查看可用福利。', action: '认识管理页面' },
] as const

export function WelcomePage({ onComplete, onEntered, displayName, appearanceControls, enterLabel = '进入管理页面' }: WelcomePageProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const guideRef = useRef<HTMLDivElement>(null)
  const guideAnimation = useRef<Animation | null>(null)
  const exitAnimation = useRef<Animation | null>(null)
  const fallbackExit = useRef<ReturnType<typeof setTimeout> | null>(null)
  const disposed = useRef(false)
  const exitStarted = useRef(false)
  const previousGuideOpen = useRef(false)
  const [guideOpen, setGuideOpen] = useState(false)
  const [activeStep, setActiveStep] = useState(0)
  const [leaving, setLeaving] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const guideId = useId()
  const name = displayName?.trim()
  let pageState = 'ready'
  let buttonLabel = enterLabel
  if (saving) {
    pageState = 'saving'
    buttonLabel = '正在准备你的空间'
  }
  if (leaving) {
    pageState = 'leaving'
    buttonLabel = '正在进入管理页面'
  }

  useEffect(() => {
    disposed.current = false
    return () => {
      disposed.current = true
      guideAnimation.current?.cancel()
      exitAnimation.current?.cancel()
      if (fallbackExit.current !== null) clearTimeout(fallbackExit.current)
    }
  }, [])

  useEffect(() => {
    const node = guideRef.current
    if (!node || previousGuideOpen.current === guideOpen) return
    previousGuideOpen.current = guideOpen
    const currentHeight = node.getBoundingClientRect().height
    const currentStyle = window.getComputedStyle(node)
    const currentOpacity = node.hidden ? '0' : currentStyle.opacity
    const currentTransform = node.hidden ? 'translateY(-8px)' : currentStyle.transform
    guideAnimation.current?.cancel()
    if (guideOpen) node.hidden = false
    const targetHeight = guideOpen ? node.scrollHeight : 0
    if (typeof node.animate !== 'function') {
      node.hidden = !guideOpen
      return
    }
    const animation = node.animate([
      { height: `${currentHeight}px`, opacity: currentOpacity, transform: currentTransform },
      { height: `${targetHeight}px`, opacity: guideOpen ? 1 : 0, transform: guideOpen ? 'translateY(0)' : 'translateY(-8px)' },
    ], { duration: 280, easing: 'cubic-bezier(.25,.1,.25,1)' })
    guideAnimation.current = animation
    void animation.finished.then(() => {
      if (guideAnimation.current !== animation) return
      node.hidden = !guideOpen
      guideAnimation.current = null
    }).catch(() => {})
  }, [guideOpen])

  const finish = async () => {
    if (exitStarted.current) return
    exitStarted.current = true
    setSaving(true)
    setSaveError('')
    try {
      await onComplete()
    } catch {
      if (!disposed.current) {
        exitStarted.current = false
        setSaving(false)
        setSaveError('暂时没能保存欢迎页状态，请再试一次。')
      }
      return
    }
    if (disposed.current) return
    setSaving(false)
    setLeaving(true)
    const complete = () => {
      if (!disposed.current) onEntered?.()
    }
    const node = rootRef.current
    if (node && typeof node.animate === 'function') {
      const animation = node.animate([
        { opacity: 1, transform: 'none' },
        { opacity: 0, transform: 'none' },
      ], { duration: 180, easing: 'cubic-bezier(.25,.1,.25,1)', fill: 'forwards' })
      exitAnimation.current = animation
      void animation.finished.then(complete).catch(() => {})
    } else {
      fallbackExit.current = setTimeout(complete, 180)
    }
  }

  const showStep = (index: number) => {
    setActiveStep(index)
    setGuideOpen(true)
  }

  return <div className='further-welcome' ref={rootRef} data-welcome-state={pageState}>
    <header className='fwelcome-header'>
      <div className='fwelcome-brand'>
        <img src='/further-api-logo.svg?v=handshake-20261004' alt='' width='36' height='36' />
        <span>further-api</span>
      </div>
      <div className='fwelcome-header-end'><span className='fwelcome-new'><Check size={14} aria-hidden='true' />你的空间已就绪</span>{appearanceControls}</div>
    </header>

    <main className='fwelcome-main'>
      <section className='fwelcome-hero' aria-labelledby='fwelcome-title'>
        <div className='fwelcome-hero-copy'>
          <p className='fwelcome-eyebrow'><span aria-hidden='true' />一个入口，开始更多可能</p>
          <h1 id='fwelcome-title'><span className='fwelcome-title-line'>{name ? `你好，${name}` : '欢迎来到'}<span className='fwelcome-wave' aria-hidden='true'>✦</span></span><span className='fwelcome-title-line fwelcome-title-brand'>further-api</span></h1>
          <p className='fwelcome-intro'>你的 AI 网关已经准备好。<br />从第一条请求开始，把灵感变成真正可用的体验。</p>
          <div className='fwelcome-hero-caption'><span className='fwelcome-caption-rule' aria-hidden='true' />接入、体验、管理，从这里自然展开。</div>
        </div>
        <div className='fwelcome-art' aria-hidden='true'>
          <div className='fwelcome-orbit fwelcome-orbit-outer' />
          <div className='fwelcome-orbit fwelcome-orbit-inner' />
          <svg className='fwelcome-connection' viewBox='0 0 360 280' fill='none'>
            <path pathLength='1' d='M43 208C86 220 101 120 177 139C251 156 245 42 310 72' />
          </svg>
          <div className='fwelcome-hero-logo'><img src='/further-api-logo.svg?v=handshake-20261004' alt='' width='108' height='108' /></div>
          <span className='fwelcome-art-label fwelcome-art-label-key'><KeyRound size={16} />连接</span>
          <span className='fwelcome-art-label fwelcome-art-label-spark'><Sparkles size={16} />创造</span>
          <span className='fwelcome-orbit-dot fwelcome-orbit-dot-one' />
          <span className='fwelcome-orbit-dot fwelcome-orbit-dot-two' />
          <span className='fwelcome-art-star fwelcome-art-star-one'>✦</span>
          <span className='fwelcome-art-star fwelcome-art-star-two'>✧</span>
        </div>
      </section>

      <section className='fwelcome-start' aria-labelledby='fwelcome-start-title'>
        <div className='fwelcome-section-title'><h2 id='fwelcome-start-title'>让第一次使用更顺手</h2><span>从任意一步开始</span></div>
        <div className='fwelcome-cards'>
          {steps.map(({ icon: Icon, title, short, description, action }, index) => <button className='fwelcome-card' key={title} type='button' disabled={leaving || saving} aria-controls={guideId} aria-expanded={guideOpen && activeStep === index} onClick={() => showStep(index)}>
            <div className='fwelcome-card-top'><span className='fwelcome-card-icon'><Icon size={23} aria-hidden='true' /></span><span className='fwelcome-card-number'>0{index + 1}</span></div>
            <span className='fwelcome-card-kicker'>{short}</span>
            <strong>{title}</strong>
            <span className='fwelcome-card-description'>{description}</span>
            <span className='fwelcome-card-action'>{action}<ArrowRight size={17} aria-hidden='true' /></span>
          </button>)}
        </div>
      </section>

      <section className='fwelcome-next' aria-label='开始使用'>
        <div className='fwelcome-next-copy'><strong>准备好，开始你的第一次调用。</strong><span>你可以随时在管理页面继续探索。</span></div>
        <button type='button' className='fwelcome-primary' disabled={leaving || saving} aria-busy={saving} onClick={() => void finish()}><span>{buttonLabel}</span><ArrowRight size={20} aria-hidden='true' /></button>
      </section>
      {saveError && <p className='fwelcome-save-error' role='alert'>{saveError}</p>}

      <section className='fwelcome-guide'>
        <button type='button' className='fwelcome-guide-toggle' disabled={leaving || saving} aria-expanded={guideOpen} aria-controls={guideId} onClick={() => setGuideOpen(value => !value)}><span>三步开始调用</span><ChevronDown size={19} aria-hidden='true' /></button>
        <div className='fwelcome-guide-body' ref={guideRef} id={guideId} aria-hidden={!guideOpen} hidden>
          <ol>{steps.map(({ title, detail }, index) => <li key={title} data-active={activeStep === index ? 'true' : undefined}><span className='fwelcome-step-number' aria-hidden='true'>{index + 1}</span><div><h3>{title}</h3><p>{detail}</p></div></li>)}</ol>
        </div>
      </section>
    </main>
    <footer className='fwelcome-footer'>further-api · 让每一次连接都更简单</footer>
  </div>
}
