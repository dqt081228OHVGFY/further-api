import { AlertCircle, Check, RotateCcw, X } from 'lucide-react';
import { useEffect, useEffectEvent, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { MATERIAL_MOTION } from '../material-motion';

export function Notice({
  error,
  success
}: {
  error?: string;
  success?: string;
}) {
  return <>
      {error && <div className='rfw-message rfw-error' role='alert'>
          <AlertCircle size={18} />
          {error}
        </div>}
      {success && <div className='rfw-message rfw-success' role='status'>
          <Check size={18} />
          {success}
        </div>}
    </>;
}
export function Page({
  title,
  description,
  loading,
  error,
  reload,
  children,
  actions
}: {
  title: string;
  description?: string;
  loading?: boolean;
  error?: string;
  reload?: () => unknown;
  children: ReactNode;
  actions?: ReactNode;
}) {
  const [hasContent, setHasContent] = useState(!loading);
  if (!loading && !hasContent) setHasContent(true);
  return <section className='rfw-page' aria-busy={loading || undefined}>
      <header className='rfw-header'>
        <div>
          <span className='rfw-eyebrow'>further-api</span>
          <h1>{title}</h1>
          {description && <p>{description}</p>}
        </div>
        <div className='rfw-toolbar'>
          {actions}
          {reload && <button type='button' className='rfw-icon-button' disabled={loading} aria-label='重新载入' title='重新载入' onClick={() => void reload()}>
              <RotateCcw size={18} />
            </button>}
        </div>
      </header>
      <Notice error={error} />
      {loading && !hasContent ? <div className='rfw-loading' role='status'>
          正在载入…
        </div> : <>
          {loading && <div className='rfw-refresh-indicator' aria-label='正在更新数据' />}
          {children}
        </>}
    </section>;
}
export function Modal({
  title,
  children,
  close,
  busy = false,
  dismiss = false
}: {
  title: string;
  children: ReactNode;
  close: () => void;
  busy?: boolean;
  dismiss?: boolean;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const restoreFocus = useRef<Element | null>(null);
  const titleId = useId();
  const closeRef = useRef(close);
  useLayoutEffect(() => { closeRef.current = close; }, [close]);
  const [closing, setClosing] = useState(false);
  const closeRequested = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const surfaceAnimation = useRef<Animation | null>(null);
  const animateSurface = (opening: boolean) => {
    const dialog = ref.current;
    if (!dialog) return;
    dialog.dataset.rfwMotion = 'on';
    if (typeof dialog.animate !== 'function') return;
    const current = getComputedStyle(dialog);
    const first = opening ? {
      opacity: 0,
      translate: 'none',
      scale: '1'
    } : {
      opacity: current.opacity,
      translate: current.translate || 'none',
      scale: current.scale || '1'
    };
    surfaceAnimation.current?.cancel();
    dialog.dataset.rfwMotionEngine = 'web';
    surfaceAnimation.current = dialog.animate([first, opening ? {
      opacity: 1,
      translate: 'none',
      scale: '1'
    } : {
      opacity: 0,
      translate: 'none',
      scale: '1'
    }], {
      duration: opening ? MATERIAL_MOTION.enter : MATERIAL_MOTION.exit,
      easing: opening ? MATERIAL_MOTION.decelerate : MATERIAL_MOTION.accelerate,
      fill: opening ? 'none' : 'forwards'
    });
    return surfaceAnimation.current;
  };
  const requestClose = () => {
    if (busy || closeRequested.current) return;
    closeRequested.current = true;
    const animation = animateSurface(false);
    setClosing(true);
    const finish = () => {
      if (closeRequested.current && ref.current && (!animation || surfaceAnimation.current === animation)) closeRef.current();
    };
    if (animation?.finished) void animation.finished.then(finish).catch(() => {});
    else timer.current = setTimeout(finish, MATERIAL_MOTION.exit);
  };
  useLayoutEffect(() => {
    const dialog = ref.current;
    let previous = document.activeElement;
    while (previous?.shadowRoot?.activeElement) previous = previous.shadowRoot.activeElement;
    restoreFocus.current = previous;
    dialog?.showModal();
    closeRequested.current = false;
    animateSurface(true);
    return () => {
      closeRequested.current = false;
      clearTimeout(timer.current);
      surfaceAnimation.current?.cancel();
      dialog?.close();
      if (previous instanceof HTMLElement && previous.isConnected) {
        previous.focus({
          preventScroll: true
        });
      }
    };
  }, []);
  const closeAfterSuccess = useEffectEvent(requestClose);
  useEffect(() => {
    let active = true;
    if (dismiss && !busy) queueMicrotask(() => { if (active) closeAfterSuccess(); });
    return () => { active = false; };
  }, [dismiss, busy]);
  return <dialog ref={ref} className={`rfw-dialog${closing ? ' rfw-dialog-closing' : ''}`} aria-labelledby={titleId} aria-busy={busy} onCancel={event => {
    event.preventDefault();
    requestClose();
  }} onClick={event => {
    const bounds = event.currentTarget.getBoundingClientRect();
    const outside = event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom;
    if (event.target === event.currentTarget && outside || event.target instanceof Element && event.target.closest('[data-modal-close]')) {
      requestClose();
    }
  }}>
      <header>
        <div>
          <span className='rfw-eyebrow'>further-api</span>
          <h2 id={titleId}>{title}</h2>
        </div>
        <button type='button' className='rfw-icon-button' onClick={requestClose} disabled={busy || closing} aria-label='关闭'>
          <X size={20} />
        </button>
      </header>
      {children}
    </dialog>;
}
export function Empty({
  children = '当前没有记录。'
}: {
  children?: ReactNode;
}) {
  return <div className='rfw-empty'>{children}</div>;
}
