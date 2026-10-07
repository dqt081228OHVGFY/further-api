import { Fingerprint, KeyRound, Laptop, LockKeyhole, Mail, ShieldCheck } from 'lucide-react';
import { QRCodeSVG } from 'qrcode.react';
import { useEffect, useEffectEvent, useRef, useState } from 'react';
import { toast } from 'sonner';
import { beginPasskeyRegistration, deletePasskey, finishPasskeyRegistration } from '@/features/auth/passkey/api';
import { checkVerificationMethods, verify } from '@/features/auth/secure-verification/api';
import type { VerificationInput, VerificationOperation, VerificationRequirements } from '@/features/auth/secure-verification/types';
import { bindEmail, changeAccountPassword, deleteUserAccount, resendEmailBinding, startEmailBinding, unbindCustomOAuth } from '@/features/profile/api';
import { createAccessToken, enable2FA, revokeAccessToken, setup2FA } from '@/features/security/api';
import { clearAuthentication, disable2FA, regenerate2FABackupCodes } from '@/lib/api';
import { buildRegistrationResult, createCredential, prepareCredentialCreationOptions } from '@/lib/passkey';
import { asRow, asRows, label, read, write, type Row } from './api';
import { Empty, Modal, Notice, Page } from './ui';
import { message, useResource } from "./workspace-resource";

const names = {
  password: '账户密码',
  '2fa': '身份验证器 / 备用码',
  passkey: '通行密钥',
  oauth: '第三方账户',
  session: '当前会话'
};
type Action = {
  title: string;
  description: string;
  operation: VerificationOperation;
  run: (proof: string, signal: AbortSignal) => Promise<unknown>;
};
type Secret = {
  title: string;
  value: string;
};
const endpoints = {
  user: '/api/user/self',
  twofa: '/api/user/2fa/status',
  passkey: '/api/user/passkey',
  token: '/api/user/token/status',
  sessions: '/api/user/sessions',
  bindings: '/api/user/oauth/bindings'
} as const;
type ResourceName = keyof typeof endpoints;
async function loadSecurity() {
  const keys = Object.keys(endpoints) as ResourceName[];
  const results = await Promise.allSettled(keys.map(key => read(endpoints[key])));
  const data: Partial<Record<ResourceName, unknown>> = {};
  const errors: Partial<Record<ResourceName, string>> = {};
  results.forEach((result, index) => {
    if (result.status === 'fulfilled') data[keys[index]] = result.value;else errors[keys[index]] = message(result.reason);
  });
  if (results.every(result => result.status === 'rejected')) {
    throw new Error('账户安全信息暂时无法载入，请重试。');
  }
  return {
    user: asRow(data.user),
    twofa: asRow(data.twofa),
    passkey: asRow(data.passkey),
    token: asRow(data.token),
    sessions: asRows(data.sessions),
    bindings: asRows(data.bindings),
    errors
  };
}
function Verification({
  action,
  close,
  complete
}: {
  action: Action;
  close: () => void;
  complete: (value: unknown) => void;
}) {
  const abort = useRef(new AbortController());
  const [requirements, setRequirements] = useState<VerificationRequirements>();
  const [method, setMethod] = useState<VerificationInput['method']>('password');
  const [value, setValue] = useState('');
  const [provider, setProvider] = useState('');
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [finished, setFinished] = useState<{
    value: unknown;
  } | null>(null);
  const load = async (signal = abort.current.signal) => {
    setLoading(true);
    setError('');
    try {
      const result = await checkVerificationMethods(action.operation.scope, signal);
      if (signal.aborted) return;
      setRequirements(result);
      const first = result.methods.find(option => option.available);
      if (first) setMethod(first.method);
      setProvider(result.oauth_providers[0]?.slug || '');
    } catch (cause) {
      if (!signal.aborted) setError(message(cause));
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  };
  const loadVerification = useEffectEvent(load);
  useEffect(() => {
    const controller = new AbortController();
    abort.current = controller;
    queueMicrotask(() => { if (!controller.signal.aborted) void loadVerification(controller.signal); });
    return () => controller.abort();
  }, []);
  const submit = async () => {
    if (!requirements || busy || finished) return;
    setBusy(true);
    setError('');
    try {
      const input: VerificationInput = (() => {
        if (method === 'password') {
          return {
            method,
            password: value
          };
        } else {
          if (method === '2fa') {
            return {
              method,
              code: value.trim()
            };
          } else {
            if (method === 'oauth') {
              return {
                method,
                provider
              };
            } else {
              return {
                method
              };
            }
          }
        }
      })();
      const proof = await verify(input, action.operation, requirements.password_encryption_enabled, abort.current.signal);
      const result = await action.run(proof.proof_token, abort.current.signal);
      if (!abort.current.signal.aborted) {setFinished({
        value: result
      });}
    } catch (cause) {
      if (!abort.current.signal.aborted) setError(message(cause));
    } finally {
      if (!abort.current.signal.aborted) setBusy(false);
    }
  };
  const available = requirements?.methods.filter(option => option.available) || [];
  return <Modal title={action.title} close={() => finished ? complete(finished.value) : close()} busy={busy} dismiss={Boolean(finished)}>
      <div className='rfw-security-callout'>
        <ShieldCheck size={22} />
        <p>{action.description}</p>
      </div>
      <Notice error={error} />
      {finished && <p className='rfw-modal-completed'><ShieldCheck size={18} />身份已确认，操作已完成。</p>}
      {(() => {
      if (loading) {
        return <div className='rfw-loading' role='status'>
          正在检查可用的身份验证方式…
        </div>;
      } else {
        if (!requirements) {
          return <button type='button' className='rfw-secondary' onClick={() => void load()}>
          重新检查
        </button>;
        } else {
          if (!available.length) {
            return <Notice error='当前账户没有可用的验证方式，请联系管理员。' />;
          } else {
            return <form onSubmit={event => {
              event.preventDefault();
              void submit();
            }}>
          <div className='rfw-field'>
            <label htmlFor='rfw-verification-method'>验证方式</label>
            <select id='rfw-verification-method' value={method} disabled={busy} onChange={event => {
                  setMethod(event.target.value as VerificationInput['method']);
                  setValue('');
                  setError('');
                }}>
              {available.map(option => <option key={option.method} value={option.method}>
                  {names[option.method]}
                </option>)}
            </select>
          </div>
          {(method === 'password' || method === '2fa') && <div className='rfw-field'>
              <label htmlFor='rfw-verification-value'>
                {method === 'password' ? '当前账户密码' : '验证码或备用码'}
              </label>
              <input id='rfw-verification-value' type={method === 'password' ? 'password' : 'text'} value={value} onChange={event => setValue(event.target.value)} autoComplete={method === 'password' ? 'current-password' : 'one-time-code'} required disabled={busy} autoFocus />
            </div>}
          {method === 'oauth' && <div className='rfw-field'>
              <label htmlFor='rfw-verification-provider'>第三方账户</label>
              <select id='rfw-verification-provider' value={provider} disabled={busy} onChange={event => setProvider(event.target.value)}>
                {requirements.oauth_providers.map(option => <option value={option.slug} key={option.slug}>
                    {option.name}
                  </option>)}
              </select>
            </div>}
          {(method === 'passkey' || method === 'oauth') && <p className='rfw-help'>
              继续后将打开{method === 'passkey' ? '设备身份验证' : '第三方登录'}
              窗口，请按提示完成验证。
            </p>}
          {busy && <Notice success='正在验证并执行操作，请稍候…' />}
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
              取消
            </button>
            <button type='submit' className='rfw-primary' disabled={busy || method === 'oauth' && !provider}>
              {busy ? '正在处理…' : '验证并继续'}
            </button>
          </div>
        </form>;
          }
        }
      }
    })()}
    </Modal>;
}
function deviceName(agent: unknown) {
  const text = String(agent || '');
  const os = (() => {
    if (/iPhone|iPad/i.test(text)) {
      return 'iOS';
    } else {
      if (/Android/i.test(text)) {
        return 'Android';
      } else {
        if (/Windows/i.test(text)) {
          return 'Windows';
        } else {
          if (/Macintosh|Mac OS/i.test(text)) {
            return 'macOS';
          } else {
            if (/Linux/i.test(text)) {
              return 'Linux';
            } else {
              return '设备';
            }
          }
        }
      }
    }
  })();
  const browser = (() => {
    if (/Edg\//.test(text)) {
      return 'Edge';
    } else {
      if (/Firefox\//.test(text)) {
        return 'Firefox';
      } else {
        if (/Chrome\//.test(text)) {
          return 'Chrome';
        } else {
          if (/Safari\//.test(text)) {
            return 'Safari';
          } else {
            return '浏览器';
          }
        }
      }
    }
  })();
  return text ? `${os} · ${browser}` : '未知设备';
}
function timeLabel(value: unknown) {
  const timestamp = (() => {
    if (typeof value === 'number') {
      return value * 1000;
    } else {
      if (typeof value === 'string' && /^\d+$/.test(value)) {
        return Number(value) * 1000;
      } else {
        return String(value || '');
      }
    }
  })();
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? '尚无记录' : date.toLocaleString('zh-CN');
}
function scrollSection(source: Element, id: string) {
  const root = source.getRootNode() as Document | ShadowRoot;
  root.querySelector(`#${id}`)?.scrollIntoView({
    behavior: 'smooth',
    block: 'start'
  });
}
export function SecurityScreen() {
  const state = useResource(loadSecurity);
  const [action, setAction] = useState<Action | null>(null);
  const [secret, setSecret] = useState<Secret | null>(null);
  const [success, setSuccess] = useState('');
  const [error, setError] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [email, setEmail] = useState('');
  const [emailFlow, setEmailFlow] = useState<Row | null>(null);
  const [oldCode, setOldCode] = useState('');
  const [code, setCode] = useState('');
  const [setup, setSetup] = useState<Row | null>(null);
  const [savedCodes, setSavedCodes] = useState(false);
  const [busy, setBusy] = useState(false);
  const [dismissSetup, setDismissSetup] = useState(false);
  const [dismissEmail, setDismissEmail] = useState(false);
  const [dismissSession, setDismissSession] = useState(false);
  const [confirmSession, setConfirmSession] = useState<Row | 'others' | null>(null);
  const notify = (value: string, failure = false) => {
    if (failure) {
      setError(value);
      toast.error(value);
    } else {
      setSuccess(value);
      toast.success(value);
    }
  };
  const complete = (value: unknown) => {
    const scope = action?.operation.scope;
    setAction(null);
    setError('');
    if (scope === '2fa.setup') {
      setSetup(asRow(value));
      setDismissSetup(false);
      setCode('');
      setSavedCodes(false);
      notify('身份验证器信息已准备好，完成验证码确认后才会启用。');
    } else if (scope === 'account.binding.bind' && asRow(value).flow_token) {
      setEmailFlow(asRow(value));
      setDismissEmail(false);
      setCode('');
      setOldCode('');
      notify('验证码已发送，请完成邮箱验证。');
    } else {
      const codes = Array.isArray(value) ? value : asRow(value).backup_codes;
      if (typeof value === 'string') {
        setSecret({
          title: '保存账户访问令牌',
          value
        });
      } else if (Array.isArray(codes)) {
        setSecret({
          title: '保存新的备用码',
          value: codes.join('\n')
        });
      }
      notify(asRow(value).notification_warning ? '操作已完成，但邮件通知暂时无法发送。' : `${action?.title || '操作'}已完成。`);
    }
    void state.reload();
  };
  const run = (title: string, description: string, operation: VerificationOperation, perform: Action['run']) => {
    setError('');
    setSuccess('');
    setAction({
      title,
      description,
      operation,
      run: perform
    });
  };
  const simple = async (perform: () => Promise<unknown>, done: string) => {
    if (busy) return;
    setBusy(true);
    setError('');
    setSuccess('');
    try {
      const result = await perform();
      notify(asRow(result).notification_warning ? `${done}，但邮件通知暂时无法发送。` : done);
      void state.reload();
    } catch (cause) {
      notify(message(cause), true);
    } finally {
      setBusy(false);
    }
  };
  const copy = async (value: string) => {
    setError('');
    try {
      await navigator.clipboard.writeText(value);
      notify('已复制，请保存到安全的位置。');
    } catch {
      notify('复制失败，请长按或选择内容手动保存。', true);
    }
  };
  const data = state.data;
  const hasPasskey = Boolean(data?.passkey.enabled || data?.passkey.exists);
  const activeSessions = data?.sessions.filter(session => !session.current).length || 0;
  const builtInBindings = ['github_id', 'discord_id', 'wechat_id', 'oidc_id', 'telegram_id', 'linux_do_id'].filter(key => data?.user[key]);
  return <div className='rfw-account-security'>
      <Page title='安全与访问' description='管理登录保护、账户授权与设备会话。' loading={state.loading} error={state.error} reload={state.reload}>
        <Notice error={error} success={success} />
        {data && <>
            <section className='rfw-security-hero' aria-label='账户安全概览'>
              <div className='rfw-security-emblem' aria-hidden='true'><ShieldCheck size={36} /></div>
              <div><span className='rfw-eyebrow'>你的账户，由你掌控</span><h2>{(() => {
                if (data.errors.twofa) {
                  return '检查账户的保护状态';
                } else {
                  if (data.twofa.enabled) {
                    return '双重验证正在保护你的账户';
                  } else {
                    return '为账户增加一道登录保护';
                  }
                }
              })()}</h2><p>{data.twofa.enabled ? '定期检查授权与登录设备，备用码请保存在安全的位置。' : '密码、身份验证器与通行密钥配合使用，让登录更安心。'}</p></div>
            </section>
            {Object.keys(data.errors).length > 0 && <div className='rfw-security-section-head'>
                <p className='rfw-help'>
                  部分安全信息未能载入，你仍可管理已载入的项目。
                </p>
                <button type='button' className='rfw-secondary' disabled={state.loading} onClick={() => void state.reload()}>
                  {state.loading ? '正在重试…' : '重试载入'}
                </button>
              </div>}
            <div className='rfw-security-overview'>
              <div>
                <ShieldCheck size={26} />
                <span>
                  登录保护
                  <strong>
                    {(() => {
                  if (data.errors.twofa) {
                    return '状态暂不可用';
                  } else {
                    if (data.twofa.enabled) {
                      return '双重验证已启用';
                    } else {
                      return '建议启用双重验证';
                    }
                  }
                })()}
                  </strong>
                </span>
              </div>
              <div>
                <Fingerprint size={26} />
                <span>
                  快捷登录
                  <strong>
                    {(() => {
                  if (data.errors.passkey) {
                    return '状态暂不可用';
                  } else {
                    if (hasPasskey) {
                      return '通行密钥已就绪';
                    } else {
                      return '可绑定你的设备';
                    }
                  }
                })()}
                  </strong>
                </span>
              </div>
              <div>
                <Laptop size={26} />
                <span>
                  登录设备
                  <strong>
                    {data.errors.sessions ? '状态暂不可用' : `${data.sessions.length} 个活动会话`}
                  </strong>
                </span>
              </div>
            </div>
            <nav className='rfw-security-navigation' aria-label='账户安全导航'>
              <a href='#rfw-security-login' onClick={event => {
            event.preventDefault();
            scrollSection(event.currentTarget, 'rfw-security-login');
          }}><LockKeyhole size={16} />登录保护</a>
              <a href='#rfw-security-access' onClick={event => {
            event.preventDefault();
            scrollSection(event.currentTarget, 'rfw-security-access');
          }}><KeyRound size={16} />账户与授权</a>
              <a href='#rfw-security-devices' onClick={event => {
            event.preventDefault();
            scrollSection(event.currentTarget, 'rfw-security-devices');
          }}><Laptop size={16} />登录设备</a>
            </nav>
            <section id='rfw-security-login' className='rfw-security-block'>
              <div className='rfw-security-block-heading'><span>01</span><div><h2>登录保护</h2><p>选择适合你的登录与身份验证方式。</p></div></div>
              <div className='rfw-security-grid'>
              <article className='rfw-card rfw-security-card'>
                <div className='rfw-security-card-title'>
                  <LockKeyhole size={22} />
                  <h3>账户密码</h3>
                  <span className='rfw-security-badge'>{(() => {
                    if (data.errors.user) return '未载入';
                    return data.user.has_password === false ? '待设置' : '已设置';
                  })()}</span>
                </div>
                <Notice error={data.errors.user} />
                <p className='rfw-security-card-description'>确认账户身份后保存新密码，其他设备需要重新登录。</p>
                <form className='rfw-security-card-form' onSubmit={event => {
                event.preventDefault();
                if (password !== confirmPassword) {
                  notify('两次输入的密码不一致。', true);
                  return;
                }
                run('更新账户密码', '确认身份后保存新密码，并退出其他登录会话。', {
                  scope: data.user.has_password === false ? 'account.password.set' : 'account.password.change'
                }, async (proof, signal) => {
                  const response = await changeAccountPassword({
                    password
                  }, proof, signal);
                  setPassword('');
                  setConfirmPassword('');
                  return response;
                });
              }}>
                  <div className='rfw-security-card-body'>
                  <div className='rfw-field'>
                    <label htmlFor='rfw-new-password'>新密码</label>
                    <input id='rfw-new-password' type='password' autoComplete='new-password' placeholder='至少 8 位字符' minLength={8} maxLength={128} value={password} onChange={event => setPassword(event.target.value)} required />
                  </div>
                  <div className='rfw-field'>
                    <label htmlFor='rfw-confirm-password'>再次输入</label>
                    <input id='rfw-confirm-password' type='password' autoComplete='new-password' minLength={8} maxLength={128} value={confirmPassword} onChange={event => setConfirmPassword(event.target.value)} required />
                  </div>
                  </div>
                  <div className='rfw-security-card-actions'>
                  <button type='submit' className='rfw-primary' disabled={Boolean(data.errors.user)}>
                    更新密码
                  </button>
                  </div>
                </form>
              </article>
              <article className='rfw-card rfw-security-card'>
                <div className='rfw-security-card-title'>
                  <ShieldCheck size={22} />
                  <h3>双重验证</h3>
                  <span className='rfw-security-badge'>
                    {(() => {
                    if (data.errors.twofa) {
                      return '未载入';
                    } else {
                      if (data.twofa.enabled) {
                        return '已启用';
                      } else {
                        return '未启用';
                      }
                    }
                  })()}
                  </span>
                </div>
                <Notice error={data.errors.twofa} />
                <p className='rfw-security-card-description'>
                  {data.twofa.enabled ? `身份验证器已绑定，剩余 ${label(data.twofa.backup_codes_remaining)} 个备用码。` : '即使密码泄露，登录时仍需你的身份验证器确认。'}
                </p>
                <div className='rfw-security-card-body'>
                <div className='rfw-security-detail'>
                  <span>推荐方式</span>
                  <strong>身份验证器 + 离线备用码</strong>
                </div>
                <p className='rfw-security-note'>备用码可在无法使用身份验证器时验证身份，请保存在安全的位置。</p>
                </div>
                <div className='rfw-toolbar rfw-security-card-actions'>
                  {data.twofa.enabled ? <>
                      <button type='button' className='rfw-secondary' onClick={() => run('重新生成备用码', '现有备用码将全部失效。完成后请保存新的备用码。', {
                    scope: '2fa.backup_codes.regenerate'
                  }, regenerate2FABackupCodes)}>
                        更新备用码
                      </button>
                      <button type='button' className='rfw-danger' onClick={() => run('停用双重验证', '停用后将不再要求身份验证器验证码。', {
                    scope: '2fa.disable'
                  }, disable2FA)}>
                        停用
                      </button>
                    </> : <button type='button' className='rfw-primary' disabled={Boolean(data.errors.twofa)} onClick={() => run('启用双重验证', '先验证账户身份，再绑定身份验证器。', {
                  scope: '2fa.setup'
                }, setup2FA)}>
                      开始绑定
                    </button>}
                </div>
              </article>
              <article className='rfw-card rfw-security-card'>
                <div className='rfw-security-card-title'>
                  <Fingerprint size={22} />
                  <h3>通行密钥</h3>
                  <span className='rfw-security-badge'>
                    {(() => {
                    if (data.errors.passkey) {
                      return '未载入';
                    } else {
                      if (hasPasskey) {
                        return '已绑定';
                      } else {
                        return '待绑定';
                      }
                    }
                  })()}
                  </span>
                </div>
                <Notice error={data.errors.passkey} />
                <p className='rfw-security-card-description'>通过设备指纹、面容或安全密钥登录，减少输入密码。</p>
                <div className='rfw-security-card-body'>
                  <div className='rfw-security-detail'><span>验证方式</span><strong>指纹、面容或安全密钥</strong></div>
                  <p className='rfw-security-note'>新通行密钥注册成功后，会替换账户原有的通行密钥。</p>
                </div>
                <div className='rfw-toolbar rfw-security-card-actions'>
                  <button type='button' className='rfw-primary' disabled={Boolean(data.errors.passkey)} onClick={() => run(hasPasskey ? '更新通行密钥' : '注册通行密钥', hasPasskey ? '新通行密钥注册成功后将替换原有通行密钥。' : '验证身份后，按设备提示创建通行密钥。', {
                  scope: 'passkey.register'
                }, async (proof, signal) => {
                  const start = await beginPasskeyRegistration(proof, signal);
                  if (!start.flow_token) {
                    throw new Error('注册流程已失效，请重试。');
                  }
                  const credential = await createCredential(prepareCredentialCreationOptions(start.options ?? start), signal);
                  const payload = buildRegistrationResult(credential as PublicKeyCredential | null);
                  if (!payload) throw new Error('已取消注册。');
                  return finishPasskeyRegistration(start.flow_token, payload, signal);
                })}>
                    {hasPasskey ? '更新密钥' : '绑定设备'}
                  </button>
                  {hasPasskey && <button type='button' className='rfw-danger' onClick={() => run('删除通行密钥', '删除后，此通行密钥将无法用于账户登录。', {
                  scope: 'passkey.delete'
                }, deletePasskey)}>
                      删除
                    </button>}
                </div>
              </article>
              </div>
            </section>
            <section id='rfw-security-access' className='rfw-security-block'>
              <div className='rfw-security-block-heading'><span>02</span><div><h2>账户与授权</h2><p>集中管理账户令牌、通知邮箱与外部身份。</p></div></div>
              <div className='rfw-security-grid'>
              <article className='rfw-card rfw-security-card'>
                <div className='rfw-security-card-title'>
                  <KeyRound size={22} />
                  <h3>账户访问令牌</h3>
                  <span className='rfw-security-badge'>
                    {(() => {
                    if (data.errors.token) {
                      return '未载入';
                    } else {
                      if (data.token.exists) {
                        return '已授权';
                      } else {
                        return '未创建';
                      }
                    }
                  })()}
                  </span>
                </div>
                <Notice error={data.errors.token} />
                <p className='rfw-security-card-description'>
                  用于账户管理接口。新令牌仅展示一次，重新生成会使原令牌失效。
                </p>
                <div className='rfw-security-card-body'>
                {data.token.exists ? <div className='rfw-security-detail'>
                    <span>最近使用 · {timeLabel(data.token.last_used_at)}</span>
                    <strong>{label(data.token.token_ref)}</strong>
                  </div> : <div className='rfw-security-detail'><span>使用范围</span><strong>账户管理接口</strong></div>}
                  <p className='rfw-security-note'>调用模型时请使用 API Key；账户令牌用于管理账户。</p>
                </div>
                <div className='rfw-toolbar rfw-security-card-actions'>
                  <button type='button' className='rfw-primary' disabled={Boolean(data.errors.token)} onClick={() => run('生成账户访问令牌', '新令牌具有账户管理权限，生成后请立即保存。已有令牌会失效。', {
                  scope: 'access_token.generate'
                }, createAccessToken)}>
                    {data.token.exists ? '重新生成' : '生成令牌'}
                  </button>
                  {Boolean(data.token.exists) && <button type='button' className='rfw-danger' onClick={() => run('撤销账户访问令牌', '使用此令牌的账户管理请求将立即失效。', {
                  scope: 'access_token.revoke'
                }, revokeAccessToken)}>
                      撤销授权
                    </button>}
                </div>
              </article>
              <article className='rfw-card rfw-security-card'>
                <div className='rfw-security-card-title'>
                  <Mail size={22} />
                  <h3>邮箱绑定</h3>
                  <span className='rfw-security-badge'>{(() => {
                    if (data.errors.user) return '未载入';
                    return data.user.email ? '已绑定' : '待绑定';
                  })()}</span>
                </div>
                <Notice error={data.errors.user} />
                <p className='rfw-security-card-description'>接收账户通知。更换已绑定邮箱时，还需验证原邮箱。</p>
                <form className='rfw-security-card-form' onSubmit={event => {
                event.preventDefault();
                const address = email.trim();
                run('绑定邮箱', '验证身份后会发送邮箱验证码。如已绑定邮箱，还需验证原邮箱。', {
                  scope: 'account.binding.bind',
                  context: {
                    provider: 'email',
                    email: address
                  }
                }, (proof, signal) => startEmailBinding(address, proof, signal));
              }}>
                  <div className='rfw-security-card-body'>
                  <div className='rfw-security-detail'><span>当前邮箱</span><strong>{(() => {
                    if (data.errors.user) return '暂时无法读取';
                    return data.user.email ? label(data.user.email) : '尚未绑定邮箱';
                  })()}</strong></div>
                  <div className='rfw-field'>
                    <label htmlFor='rfw-bind-email'>新邮箱地址</label>
                    <input type='email' id='rfw-bind-email' value={email} onChange={event => setEmail(event.target.value)} autoComplete='email' placeholder='name@example.com' required />
                  </div>
                  </div>
                  <div className='rfw-security-card-actions'>
                  <button type='submit' className='rfw-primary' disabled={Boolean(data.errors.user)}>
                    验证并绑定
                  </button>
                  </div>
                </form>
              </article>
              <article className='rfw-card rfw-security-card'>
                <div className='rfw-security-card-title'>
                  <ShieldCheck size={22} />
                  <h3>第三方登录</h3>
                  <span className='rfw-security-badge'>{data.errors.bindings ? '未载入' : `${builtInBindings.length + data.bindings.length} 个绑定`}</span>
                </div>
                <Notice error={data.errors.bindings} />
                <p className='rfw-security-card-description'>管理用于登录的第三方身份，解绑前会再次确认账户身份。</p>
                <div className='rfw-security-card-body'>
                <div className='rfw-security-bindings-list'>
                {builtInBindings.map(key => <div className='rfw-security-detail' key={key}>
                      <span>
                        {({
                    github_id: 'GitHub',
                    discord_id: 'Discord',
                    wechat_id: '微信',
                    oidc_id: 'OIDC',
                    telegram_id: 'Telegram',
                    linux_do_id: 'Linux DO'
                  } as Record<string, string>)[key]}
                      </span>
                      <strong>{label(data.user[key])}</strong>
                    </div>)}
                {data.bindings.map(binding => <div className='rfw-security-binding' key={String(binding.provider_id)}>
                    <div>
                      <strong>
                        {label(binding.provider_name ?? binding.name)}
                      </strong>
                      <p>{label(binding.provider_user_id)}</p>
                    </div>
                    <button type='button' className='rfw-danger' onClick={() => run('解除第三方绑定', '解绑后将无法通过这个第三方账户登录。', {
                  scope: 'account.binding.unbind',
                  context: {
                    provider_id: Number(binding.provider_id)
                  }
                }, (proof, signal) => unbindCustomOAuth(Number(binding.provider_id), proof, signal))}>
                      解绑
                    </button>
                  </div>)}
                {!data.bindings.length && !data.errors.bindings && <div className='rfw-security-empty-state'><ShieldCheck size={24} aria-hidden='true' /><p>暂无自定义登录服务绑定。</p></div>}
                </div>
                </div>
                <div className='rfw-security-card-actions'><p className='rfw-security-card-footnote'><ShieldCheck size={16} aria-hidden='true' />解除绑定后，对应账户将无法用于登录。</p></div>
              </article>
            </div>
            </section>
            <article id='rfw-security-devices' className='rfw-card rfw-security-sessions'>
              <div className='rfw-security-section-head'>
                <div>
                  <h2>登录设备</h2>
                  <p>查看活动会话，及时退出不再使用的设备。</p>
                </div>
                <button type='button' className='rfw-secondary' disabled={busy || !activeSessions || Boolean(data.errors.sessions)} onClick={() => {
              setError('');
              setSuccess('');
              setDismissSession(false);
              setConfirmSession('others');
            }}>
                  退出其他设备
                </button>
              </div>
              <Notice error={data.errors.sessions} />
              <div className='rfw-session-list'>
                {data.sessions.map(session => <div className='rfw-session' key={String(session.sid)}>
                    <span className='rfw-session-icon'>
                      <Laptop size={23} />
                    </span>
                    <div className='rfw-session-copy'>
                      <strong title={String(session.user_agent || '')}>
                        {deviceName(session.user_agent)}
                        {Boolean(session.current) && <span className='rfw-security-badge'>当前设备</span>}
                      </strong>
                      <p>
                        {label(session.ip)} · 最近活动{' '}
                        {timeLabel(session.last_active_at)}
                      </p>
                    </div>
                    {!session.current && <button type='button' className='rfw-danger' disabled={busy} onClick={() => {
                setError('');
                setSuccess('');
                setDismissSession(false);
                setConfirmSession(session);
              }}>
                        退出登录
                      </button>}
                  </div>)}
              </div>
              {!data.sessions.length && !data.errors.sessions && <Empty>暂无可显示的登录会话。</Empty>}
            </article>
            <div className='rfw-security-danger-zone'>
              <div>
                <strong>注销账户</strong>
                <p>删除账户后，余额、密钥与账户访问将失效，无法恢复。</p>
              </div>
              <button type='button' className='rfw-danger' disabled={Boolean(data.errors.user)} onClick={() => run('删除账户', '此操作不可撤销。验证后将删除你的账户，余额和密钥将失效。', {
            scope: 'account.delete'
          }, async (proof, signal) => {
            const response = await deleteUserAccount(proof, signal);
            clearAuthentication();
            location.assign('/');
            return response;
          })}>
                删除账户
              </button>
            </div>
          </>}
        {action && <Verification key={action.operation.scope} action={action} close={() => setAction(null)} complete={complete} />}
        {secret && <Modal title={secret.title} close={() => setSecret(null)}>
            <p>此内容仅完整展示一次。请复制到密码管理器或其他安全位置。</p>
            <pre className='rfw-code rfw-secret' tabIndex={0}>
              {secret.value}
            </pre>
            <Notice error={error} success={success} />
            <div className='rfw-actions'>
              <button type='button' className='rfw-secondary' data-modal-close>
                已保存，关闭
              </button>
              <button type='button' className='rfw-primary' onClick={() => void copy(secret.value)}>
                复制内容
              </button>
            </div>
          </Modal>}
        {setup && <Modal title='绑定身份验证器' close={() => setSetup(null)} busy={busy} dismiss={dismissSetup}>
            <Notice error={error} success={success} />
            <div className='rfw-enrollment'>
              <div className='rfw-enrollment-qr'>
                {typeof setup.qr_code_data === 'string' && <QRCodeSVG value={setup.qr_code_data} size={176} bgColor='#fff' fgColor='#17293a' />}
              </div>
              <div>
                <strong>1. 扫码添加账户</strong>
                <p>使用身份验证器扫描二维码；也可手动输入下方密钥。</p>
                <pre className='rfw-code'>{label(setup.secret)}</pre>
                <button type='button' className='rfw-secondary' onClick={() => void copy(String(setup.secret || ''))}>
                  复制绑定密钥
                </button>
              </div>
            </div>
            <strong>2. 保存备用码</strong>
            <p>无法使用身份验证器时可用备用码登录，每个备用码只能使用一次。</p>
            <pre className='rfw-code'>
              {Array.isArray(setup.backup_codes) ? setup.backup_codes.join('\n') : label(setup.backup_codes)}
            </pre>
            <button type='button' className='rfw-secondary' onClick={() => void copy(Array.isArray(setup.backup_codes) ? setup.backup_codes.join('\n') : label(setup.backup_codes))}>
              复制备用码
            </button>
            <form onSubmit={event => {
          event.preventDefault();
          void simple(async () => {
            const response = await enable2FA(code.trim(), String(setup.flow_token), new AbortController().signal);
            setDismissSetup(true);
            return response;
          }, '双重验证已启用。');
        }}>
              <label className='rfw-switch'>
                <input type='checkbox' checked={savedCodes} onChange={event => setSavedCodes(event.target.checked)} required />
                我已妥善保存备用码
              </label>
              <div className='rfw-field'>
                <label htmlFor='rfw-enable-code'>
                  3. 输入身份验证器中的 6 位验证码
                </label>
                <input id='rfw-enable-code' value={code} onChange={event => setCode(event.target.value)} autoComplete='one-time-code' inputMode='numeric' pattern='[0-9]{6}' maxLength={6} required />
              </div>
              <div className='rfw-actions'>
                <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
                  稍后绑定
                </button>
                <button type='submit' className='rfw-primary' disabled={busy || !savedCodes}>
                  {busy ? '正在绑定…' : '完成绑定'}
                </button>
              </div>
            </form>
          </Modal>}
        {emailFlow && <Modal title='验证新邮箱' close={() => setEmailFlow(null)} busy={busy} dismiss={dismissEmail}>
            <Notice error={error} success={success} />
            <p>
              验证码已发送至 {label(emailFlow.email)}。请在有效期内完成验证。
            </p>
            <form onSubmit={event => {
          event.preventDefault();
          void simple(async () => {
            const response = await bindEmail(String(emailFlow.flow_token), code.trim(), oldCode.trim(), new AbortController().signal);
            setDismissEmail(true);
            setEmail('');
            return response;
          }, '邮箱绑定已更新。');
        }}>
              {Boolean(emailFlow.old_email_required) && <div className='rfw-field'>
                  <label htmlFor='rfw-old-email-code'>原邮箱验证码</label>
                  <input id='rfw-old-email-code' value={oldCode} onChange={event => setOldCode(event.target.value)} required autoComplete='one-time-code' inputMode='numeric' />
                </div>}
              <div className='rfw-field'>
                <label htmlFor='rfw-new-email-code'>新邮箱验证码</label>
                <input id='rfw-new-email-code' value={code} onChange={event => setCode(event.target.value)} required autoComplete='one-time-code' inputMode='numeric' />
              </div>
              <div className='rfw-actions'>
                <button type='button' className='rfw-secondary' disabled={busy} onClick={() => {
              if (Number(emailFlow.resend_at) * 1000 > Date.now()) {
                notify('验证码刚刚发送，请稍后再试。', true);
                return;
              }
              void simple(async () => {
                const response = await resendEmailBinding(String(emailFlow.flow_token), new AbortController().signal);
                setEmailFlow(asRow(response));
                return response;
              }, '验证码已重新发送。');
            }}>
                  重新发送
                </button>
                <button type='submit' className='rfw-primary' disabled={busy}>
                  {busy ? '正在验证…' : '确认绑定'}
                </button>
              </div>
            </form>
          </Modal>}
        {confirmSession && <Modal title={confirmSession === 'others' ? '退出其他登录设备' : '退出此设备'} close={() => setConfirmSession(null)} busy={busy} dismiss={dismissSession}>
            <p>
              {confirmSession === 'others' ? `将结束其他 ${activeSessions} 个登录会话，当前设备保持登录。` : `将结束 ${deviceName(confirmSession.user_agent)} 的登录会话，该设备需要重新登录。`}
            </p>
            <Notice error={error} />
            {dismissSession && <p className='rfw-modal-completed'><ShieldCheck size={18} />所选设备已退出登录。</p>}
            <div className='rfw-actions'>
              <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
                取消
              </button>
              <button type='button' className='rfw-danger' disabled={busy} onClick={() => void simple(async () => {
            const response = confirmSession === 'others' ? await write('/api/user/sessions/revoke-others', 'POST') : await write(`/api/user/sessions/${encodeURIComponent(String(confirmSession.sid))}`, 'DELETE');
            setDismissSession(true);
            return response;
          }, '所选设备已退出登录。')}>
                {busy ? '正在退出…' : '确认退出'}
              </button>
            </div>
          </Modal>}
      </Page>
    </div>;
}
