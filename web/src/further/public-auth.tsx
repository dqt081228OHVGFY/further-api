import './feature-workspace/workspace.css';
import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Turnstile } from '@/components/turnstile';
import { sendPasswordResetEmail, wechatLoginByCode } from '@/features/auth/api';
import { OAUTH_POPUP_CALLBACK_MESSAGE, OAUTH_POPUP_RESULT_MESSAGE } from '@/features/auth/constants';
import { getOAuthSessionStorage, consumeOAuthLoginRedirect, resolveOAuthCallbackMode } from '@/features/auth/lib/oauth-callback-mode';
import { parseTelegramBindCallback, postTelegramBindResult } from '@/features/auth/lib/oauth-bind-window';
import { getLoginVerificationRequirements, isLoginChallenge, verifyLogin } from '@/features/auth/secure-verification/api';
import type { LoginChallenge, VerificationRequirements } from '@/features/auth/secure-verification/types';
import { submitSetup, buildSetupPayload } from '@/features/setup/api';
import { loadWorkspaceBootstrap } from './workspace-bootstrap';
import type { SetupUsageMode } from '@/features/setup/types';
import { api, isAuthBundle } from '@/lib/api';
import { requireServerSuccess } from '@/lib/server-error-message';
import { statusQueryOptions } from '@/lib/status-query';
import { useAuthStore } from '@/stores/auth-store';
import { Notice } from './feature-workspace/ui';
import { message } from "./feature-workspace/workspace-resource";
import { finishPublicLogin } from "./auth-navigation";

export function LoginVerification({
  challenge,
  redirectTo
}: {
  challenge?: LoginChallenge;
  redirectTo?: string;
}) {
  const [initialSession] = useState(() => useAuthStore.getState().auth.session?.sid);
  const [requirements, setRequirements] = useState<VerificationRequirements>();
  const [method, setMethod] = useState<'2fa' | 'passkey'>('2fa');
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    const current = new AbortController();
    controller.current = current;
    if (!challenge) {
      return () => current.abort();
    }
    void getLoginVerificationRequirements(challenge, current.signal).then(value => {
      if (current.signal.aborted) return;
      setRequirements(value);
      if (!value.methods.some(option => option.available && option.method === '2fa')) setMethod('passkey');
    }).catch(cause => {
      if (!current.signal.aborted) setError(message(cause));
    });
    return () => current.abort();
  }, [challenge]);
  const submit = async () => {
    if (!challenge || !controller.current || busy) return;
    const current = controller.current;
    setBusy(true);
    setError('');
    try {
      if (useAuthStore.getState().auth.session?.sid !== initialSession) throw new Error('登录会话发生变化，请重新登录。');
      const bundle = await verifyLogin(method === '2fa' ? {
        method,
        code
      } : {
        method
      }, challenge, current.signal);
      if (!current.signal.aborted) finishPublicLogin(bundle, redirectTo);
    } catch (cause) {
      if (!current.signal.aborted) setError(message(cause));
    } finally {
      if (!current.signal.aborted) setBusy(false);
    }
  };
  return <><h1>完成登录验证</h1><p>使用账户已启用的方式确认身份。</p><Notice error={error || (challenge ? "" : "验证流程已结束，请重新登录。")} /><form onSubmit={event => {
      event.preventDefault();
      void submit();
    }}>{requirements && <div className='rfw-field'><label htmlFor='further-login-method'>验证方式</label><select id='further-login-method' value={method} disabled={busy} onChange={event => setMethod(event.target.value as '2fa' | 'passkey')}>{requirements.methods.filter(option => option.available && (option.method === '2fa' || option.method === 'passkey')).map(option => <option key={option.method} value={option.method}>{option.method === '2fa' ? '身份验证器 / 备用码' : '通行密钥'}</option>)}</select></div>}{method === '2fa' && <div className='rfw-field'><label htmlFor='further-login-code'>验证码或备用码</label><input id='further-login-code' value={code} onChange={event => setCode(event.target.value)} autoComplete='one-time-code' required disabled={busy} /></div>}<div className='rfw-actions'><a href='/auth/?mode=login'>返回登录</a><button className='rfw-primary' type='submit' disabled={!requirements || busy}>{busy ? '验证中…' : '验证并登录'}</button></div></form></>;
}
function OAuthFlow() {
  const [params] = useState(() => new URLSearchParams(location.search));
  const provider = location.pathname.split('/')[2] || params.get('provider') || '';
  const state = params.get('state') || '';
  const code = params.get('code') || '';
  const [error, setError] = useState('');
  const [status, setStatus] = useState('正在完成授权…');
  const [challenge, setChallenge] = useState<LoginChallenge>();
  const [redirectTo, setRedirectTo] = useState<string>();
  const request = useRef<Promise<unknown> | null>(null);
  useEffect(() => {
    let active = true;
    if (!/^[a-z0-9_-]+$/i.test(provider)) {
      queueMicrotask(() => setError('授权提供商无效，请重新登录。'));
      return;
    }
    const telegram = provider === 'telegram' ? parseTelegramBindCallback({
      telegram_bind: params.get('telegram_bind') || undefined,
      flow_token: params.get('flow_token') || undefined,
      error_code: params.get('error_code') || undefined
    }) : null;
    if (telegram) {
      if (postTelegramBindResult(telegram, window.opener, location.origin)) {
        queueMicrotask(() => setStatus('绑定结果已发送，可以关闭此窗口。'));
        window.close();
      } else queueMicrotask(() => setError('绑定窗口已经结束，请返回账户设置重试。'));
      return;
    }
    const mode = resolveOAuthCallbackMode(provider, state, {
      opener: window.opener,
      storage: getOAuthSessionStorage(window)
    });
    if (mode === 'bind' || mode === 'verify') {
      const opener = window.opener;
      if (!opener || opener.closed) {
        queueMicrotask(() => setError('原授权窗口已关闭，请返回账户设置重试。'));
        return;
      }
      const listener = (event: MessageEvent) => {
        const result = event.data;
        if (event.origin !== location.origin || event.source !== opener || result?.type !== OAUTH_POPUP_RESULT_MESSAGE || result?.intent !== mode || result?.provider !== provider || result?.state !== state) return;
        if (result.success) {
          setStatus('授权已完成，可以关闭此窗口。');
          window.close();
        } else setError(String(result.message || '授权失败，请返回账户设置重试。'));
      };
      const deadline = window.setTimeout(() => setError('授权响应超时，请返回账户设置重试。'), 30000);
      window.addEventListener('message', listener);
      opener.postMessage({
        type: OAUTH_POPUP_CALLBACK_MESSAGE,
        intent: mode,
        provider,
        state,
        code,
        error: params.get('error') || undefined,
        errorDescription: params.get('error_description') || undefined
      }, location.origin);
      return () => {
        window.clearTimeout(deadline);
        window.removeEventListener('message', listener);
      };
    }
    if (!code && !params.get('error')) {
      queueMicrotask(() => setError('授权回调缺少必要信息，请重新登录。'));
      return;
    }
    request.current ??= provider === 'wechat' && location.pathname === '/oauth' ? wechatLoginByCode(code).then(value => requireServerSuccess(value).data) : api.get(`/api/oauth/${encodeURIComponent(provider)}`, {
      params: {
        code: code || undefined,
        state,
        error: params.get('error') || undefined,
        error_description: params.get('error_description') || undefined
      },
      skipAuthRefresh: true,
      skipBusinessError: true,
      singleUseAuthorization: true
    }).then(response => requireServerSuccess(response.data).data);
    void request.current.then(result => {
      if (!active) return;
      const target = params.get('redirect') || consumeOAuthLoginRedirect(state) || undefined;
      if (isAuthBundle(result)) {
        setStatus('登录成功，正在打开工作区…');
        finishPublicLogin(result, target);
      } else if (isLoginChallenge(result)) {
        setChallenge(result);
        setRedirectTo(target);
      } else setError('授权未返回有效会话，请重新登录。');
    }).catch(cause => {
      if (active) setError(message(cause));
    });
    return () => {
      active = false;
    };
  }, [provider, state, code, params]);
  return challenge ? <LoginVerification challenge={challenge} redirectTo={redirectTo} /> : <><h1>账户授权</h1><p role='status'>{status}</p><Notice error={error} /><div className='rfw-actions'><a className='rfw-secondary' href='/auth/?mode=login'>返回登录</a>{window.opener && <button className='rfw-primary' onClick={() => window.close()} type="button">关闭窗口</button>}</div></>;
}
function ForgotPassword() {
  const {
    data: status,
    isLoading,
    error: statusError
  } = useQuery(statusQueryOptions);
  const [email, setEmail] = useState('');
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [remaining, setRemaining] = useState(0);
  useEffect(() => {
    if (!remaining) return;
    const timer = window.setTimeout(() => setRemaining(remaining - 1), 1000);
    return () => clearTimeout(timer);
  }, [remaining]);
  const enabled = Boolean(status?.turnstile_check);
  const send = async () => {
    if (enabled && !token) return;
    setBusy(true);
    setError('');
    try {
      requireServerSuccess(await sendPasswordResetEmail(email, token));
      setSuccess('重置邮件已发送，请检查收件箱。');
      setRemaining(30);
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  return <><h1>找回密码</h1><p>向绑定邮箱发送重置链接。</p><Notice error={error || (statusError ? '登录配置暂时无法载入，请刷新重试。' : '')} success={success} /><form onSubmit={event => {
      event.preventDefault();
      void send();
    }}><div className='rfw-field'><label htmlFor='further-reset-email'>邮箱</label><input type='email' id='further-reset-email' autoComplete='email' value={email} onChange={event => setEmail(event.target.value)} required /></div>{enabled && typeof status?.turnstile_site_key === 'string' && <Turnstile siteKey={status.turnstile_site_key} onVerify={setToken} onExpire={() => setToken('')} />}<div className='rfw-actions'><a href='/auth/?mode=login'>返回登录</a><button type='submit' className='rfw-primary' disabled={busy || remaining > 0 || isLoading || Boolean(statusError) || enabled && !token}>{(() => {
            if (busy) {
              return '发送中…';
            } else {
              if (remaining) {
                return `${remaining} 秒后可重发`;
              } else {
                return '发送重置邮件';
              }
            }
          })()}</button></div></form></>;
}
function ResetPassword() {
  const params = new URLSearchParams(location.search);
  const email = params.get('email') || '';
  const token = params.get('token') || '';
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      const response = requireServerSuccess((await api.post('/api/user/reset', {
        email,
        token
      }, {
        skipAuthRefresh: true,
        singleUseAuthorization: true
      })).data);
      if (typeof response.data !== 'string' || !response.data) throw new Error('服务未返回新密码。');
      setPassword(response.data);
      history.replaceState(null, '', location.pathname);
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  return <><h1>重置账户密码</h1><p>确认后将生成新密码，其他登录会话会失效。</p><Notice error={error || (!password && (!email || !token) ? '重置链接无效，请重新申请。' : '')} success={success} />{password ? <><div className='rfw-field'><label htmlFor='further-new-password'>新密码</label><input id='further-new-password' value={password} readOnly autoComplete='off' /></div><button className='rfw-secondary' onClick={() => void navigator.clipboard.writeText(password).then(() => setSuccess('已复制，请妥善保存。')).catch(() => setError('复制失败，请手动保存。'))} type="button">复制新密码</button></> : <button className='rfw-primary' disabled={busy || !email || !token} onClick={() => void submit()} type="button">{busy ? '重置中…' : '确认重置密码'}</button>}<div className='rfw-actions'><a href='/forgot-password'>重新申请链接</a><a className='rfw-secondary' href='/auth/?mode=login'>返回登录</a></div></>;
}
function Setup() {
  const status = useQuery({
    queryKey: ['further-setup'],
    queryFn: async () => (await loadWorkspaceBootstrap()).setup,
    retry: 1
  });
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [mode, setMode] = useState<SetupUsageMode>('external');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const rootInitialized = Boolean(status.data?.root_init);
  const save = async () => {
    if (!rootInitialized && (password.length < 8 || password !== confirm)) {
      setError('密码至少 8 个字符，且两次输入必须一致。');
      return;
    }
    setBusy(true);
    setError('');
    try {
      requireServerSuccess(await submitSetup(buildSetupPayload({
        username,
        password,
        confirmPassword: confirm,
        usageMode: mode
      }, rootInitialized)));
      location.replace('/auth/?mode=login');
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  return <><h1>初始化 further-api</h1><p>设置管理员账户与运行方式。</p><Notice error={error || (status.error ? message(status.error) : '')} />{(() => {
      if (status.isLoading) {
        return <p role='status'>正在读取初始化状态…</p>;
      } else {
        if (status.data?.status) {
          return <><p>服务已完成初始化。</p><a className='rfw-primary' href='/auth/?mode=login'>进入登录</a></>;
        } else {
          return status.data && <form onSubmit={event => {
            event.preventDefault();
            void save();
          }}>{!rootInitialized && <><div className='rfw-field'><label htmlFor='further-setup-name'>管理员用户名</label><input id='further-setup-name' value={username} onChange={event => setUsername(event.target.value)} autoComplete='username' minLength={1} maxLength={20} required /></div><div className='rfw-field'><label htmlFor='further-setup-password'>管理员密码</label><input id='further-setup-password' type='password' autoComplete='new-password' minLength={8} maxLength={128} value={password} onChange={event => setPassword(event.target.value)} required /></div><div className='rfw-field'><label htmlFor='further-setup-confirm'>确认密码</label><input id='further-setup-confirm' type='password' autoComplete='new-password' value={confirm} onChange={event => setConfirm(event.target.value)} required /></div></>}<div className='rfw-field'><label htmlFor='further-setup-mode'>运行方式</label><select id='further-setup-mode' value={mode} onChange={event => setMode(event.target.value as SetupUsageMode)}><option value='external'>面向用户提供服务</option><option value='self'>个人使用</option><option value='demo'>公开演示</option></select></div><div className='rfw-actions'><span>数据库：{status.data.database_type}</span><button className='rfw-primary' type='submit' disabled={busy}>{busy ? '保存中…' : '完成初始化'}</button></div></form>;
        }
      }
    })()}</>;
}
export function PublicAuthPage({
  pathname
}: {
  pathname: string;
}) {
  const [pending] = useState(() => useAuthStore.getState().auth.pendingLoginVerification);
  const page = (() => {
    if (pathname.startsWith('/setup')) {
      return <Setup />;
    } else {
      if (pathname.startsWith('/oauth')) {
        return <OAuthFlow />;
      } else {
        if (pathname.startsWith('/forgot-password')) {
          return <ForgotPassword />;
        } else {
          if (pathname === '/otp') {
            return <LoginVerification challenge={pending?.challenge} redirectTo={pending?.redirectTo} />;
          } else {
            return <ResetPassword />;
          }
        }
      }
    }
  })();
  return <section className='further-content'><div className='rfw-card' style={{
      maxWidth: 640,
      margin: 'auto'
    }}>{page}</div></section>;
}
