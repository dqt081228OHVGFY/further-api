import { AlertCircle, ArrowUpRight, Check, ChevronRight, Plus, RotateCcw, Save, Search, Settings2, ShieldCheck } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
import { useWorkspaceRefresh } from '../native-feature-context';
import { PaymentActivation } from './payment-activation';
import { asRow, asRows, label, read, write, type Row } from './api';
import type { Field, SettingsPage } from './catalog';
import { Empty, Modal, Notice, Page } from './ui';
import { message, useResource } from "./workspace-resource";
import { validateSetting } from "./settings-validation";
import { settingPresetsFor, type SettingPreset } from './settings-presets';

type Values = Record<string, string>;
const domainKeys = ['passkey.rp_id', 'passkey.legacy_rp_ids', 'passkey.origins'] as const;
function stringValue(value: unknown): string {
  if (typeof value === 'string') return value;
  if (value === null || value === undefined) return '';
  return typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value);
}
function optionValues(value: unknown): Values {
  if (!Array.isArray(value)) throw new Error('设置响应格式不正确。');
  const entries = value.map(item => {
    const row = asRow(item);
    return [String(row.key || ''), stringValue(row.value)] as const;
  }).filter(([key]) => key);
  return Object.fromEntries(entries);
}
function FieldControl({
  field,
  value,
  onChange,
  disabled
}: {
  field: Field;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  if (field.options) {
    return <select id={`rfw-${field.key}`} value={value} onChange={event => onChange(event.target.value)} disabled={disabled}>
        {!field.options.some(option => option.value === value) && <option value={value}>{value ? '保留当前配置' : '请选择'}</option>}
        {field.options.map(option => <option key={option.value} value={option.value}>
            {option.label}
          </option>)}
      </select>;
  }
  if (field.kind === 'toggle') {
    return <label className='rfw-switch'>
        <input id={`rfw-${field.key}`} type='checkbox' checked={value === 'true'} onChange={event => onChange(String(event.target.checked))} disabled={disabled} />
        <span aria-hidden='true' />
        {value === 'true' ? '已启用' : '已停用'}
      </label>;
  }
  if (field.kind === 'multiline') {
    return <div className='rfw-setting-editor'>
        <textarea id={`rfw-${field.key}`} rows={field.format === 'json' ? 6 : 4} value={value} onChange={event => onChange(event.target.value)} disabled={disabled} spellCheck={false} />
        {field.format === 'json' && <button type='button' className='rfw-secondary' disabled={disabled || !value.trim() || !!validateSetting(field, value)} onClick={() => onChange(JSON.stringify(JSON.parse(value), null, 2))}>
            整理格式
          </button>}
      </div>;
  }
  return <input id={`rfw-${field.key}`} type={(() => {
    if (field.kind === 'number') {
      return 'number';
    } else {
      if (field.kind === 'secret') {
        return 'password';
      } else {
        if (field.format === 'url') {
          return 'url';
        } else {
          return 'text';
        }
      }
    }
  })()} min={field.min} step={field.kind === 'number' ? 'any' : undefined} value={value} onChange={event => onChange(event.target.value)} disabled={disabled} autoComplete={field.kind === 'secret' ? 'new-password' : undefined} />;
}
export function SettingsScreen({
  page,
  pathname
}: {
  page: SettingsPage;
  pathname: string;
}) {
  const surface = useRef<HTMLElement>(null);
  const [initial, setInitial] = useState<Values>({});
  const [draft, setDraft] = useState<Values>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  const [presetNotice, setPresetNotice] = useState('');
  const [loaded, setLoaded] = useState(false);
  const [search, setSearch] = useState('');
  const [clears, setClears] = useState<Set<string>>(new Set());
  const [clearField, setClearField] = useState<Field | null>(null);
  const [dismissClear, setDismissClear] = useState(false);
  const [domainReview, setDomainReview] = useState<{
    values: Values;
    details: Row;
    applied: boolean;
  } | null>(null);
  const [domainConfirmed, setDomainConfirmed] = useState(false);
  const [dismissDomains, setDismissDomains] = useState(false);
  const generation = useRef(0);
  const pending = useRef<{
    path: string;
    promise: Promise<Values>;
  } | null>(null);
  const isPolicy = pathname.startsWith('/system-settings/request-policies');
  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    const current = ++generation.current;
    const request = pending.current?.path === pathname ? pending.current : {
      path: pathname,
      promise: Promise.resolve().then(async () => {
        if (isPolicy) return Object.fromEntries(Object.entries(asRow(asRow(await read('/api/option/request_policy')).options)).map(([key, value]) => [key, stringValue(value)]));
        return optionValues(await read('/api/option/'));
      })
    };
    pending.current = request;
    try {
      const values = await request.promise;
      if (current !== generation.current) return;
      setInitial(values);
      setDraft(values);
      setClears(new Set());
      setSaved(false);
      setPresetNotice('');
      setLoaded(true);
    } catch (cause) {
      if (current === generation.current) {
        setError(cause instanceof Error ? cause.message : '无法载入设置。');
      }
    } finally {
      if (pending.current === request) pending.current = null;
      if (current === generation.current) setLoading(false);
    }
  }, [isPolicy, pathname]);
  useEffect(() => {
    let active = true;
    queueMicrotask(() => { if (active) void load(); });
    return () => {
      active = false;
      // Invalidate request generations even when the document has already moved on.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      generation.current++;
    };
  }, [load]);
  const changes = useMemo(() => page.fields.filter(field => clears.has(field.key) || (draft[field.key] ?? '') !== (initial[field.key] ?? '')), [draft, initial, page.fields, clears]);
  const presets = settingPresetsFor(pathname, draft, window.location.origin).filter(preset => Object.keys(preset.values).every(key => page.fields.some(field => field.key === key && field.kind !== 'secret')));
  const applyPreset = (preset: SettingPreset) => {
    if (loading || saving || !loaded || preset.unavailable) return;
    setDraft(current => ({ ...current, ...preset.values }));
    setClears(current => new Set([...current].filter(key => !Object.hasOwn(preset.values, key))));
    setError('');
    setSaved(false);
    setSearch('');
    setPresetNotice(`已填写“${preset.label}”，请检查下方更改后保存。`);
  };
  const previewDomains = async () => {
    setSaving(true);
    setError('');
    setSaved(false);
    try {
      const values = Object.fromEntries(domainKeys.map(key => [key, draft[key] || '']));
      const details = asRow(await write('/api/option/passkey/domains', 'PUT', {
        rp_id: values['passkey.rp_id'],
        legacy_rp_ids: values['passkey.legacy_rp_ids'],
        origins: values['passkey.origins'],
        preview: true
      }));
      if (typeof details.removal_confirmation !== 'string' || !details.removal_confirmation) {
        throw new Error('服务器未返回可用的域名预览，请重新检查。');
      }
      setDomainReview({
        values,
        details,
        applied: false
      });
      setDomainConfirmed(false);
      setDismissDomains(false);
    } catch (cause) {
      setError(message(cause));
    } finally {
      setSaving(false);
    }
  };
  const persist = async (review: typeof domainReview = null) => {
    if (saving || loading || !loaded || !changes.length) {
      return;
    }
    if (review && !review.applied && review.details.confirmation_required && !domainConfirmed) {
      return;
    }
    setSaving(true);
    setError('');
    setSaved(false);
    try {
      if (isPolicy) {
        const values = Object.fromEntries(changes.map(field => [field.key, draft[field.key] ?? '']));
        const result = asRow(await write('/api/option/request_policy', 'PATCH', {
          options: values
        }));
        const next = {
          ...initial,
          ...values,
          ...Object.fromEntries(Object.entries(asRow(result.options)).map(([key, value]) => [key, stringValue(value)]))
        };
        setInitial(next);
        setDraft(next);
      } else {
        const next = {
          ...initial
        };
        if (review && !review.applied) {
          const result = asRow(await write('/api/option/passkey/domains', 'PUT', {
            rp_id: review.values['passkey.rp_id'],
            legacy_rp_ids: review.values['passkey.legacy_rp_ids'],
            origins: review.values['passkey.origins'],
            preview: false,
            removal_confirmation: String(review.details.removal_confirmation)
          }));
          const saved = Object.fromEntries(domainKeys.map(key => [key, stringValue(result[key.slice('passkey.'.length)] ?? review.values[key])]));
          Object.assign(next, saved);
          setInitial({
            ...next
          });
          setDraft(current => ({
            ...current,
            ...saved
          }));
          setDomainReview(current => current && {
            ...current,
            applied: true
          });
        }
        const priority = (field: Field) => {
          if (field.kind !== 'toggle') return 1;
          return draft[field.key] === 'false' ? 0 : 2;
        };
        for (const field of [...changes].sort((left, right) => priority(left) - priority(right))) {
          if (review && (domainKeys as readonly string[]).includes(field.key)) {
            continue;
          }
          const value = clears.has(field.key) ? '' : draft[field.key] ?? '';
          await write('/api/option/', 'PUT', {
            key: field.key,
            value
          });
          const masked = field.kind === 'secret' || initial[`${field.key}Configured`] !== undefined;
          next[field.key] = masked ? '' : value;
          if (masked) {
            next[`${field.key}Configured`] = String(Boolean(value));
            setDraft(current => ({
              ...current,
              [field.key]: '',
              [`${field.key}Configured`]: String(Boolean(value))
            }));
          }
          setInitial({
            ...next
          });
          setClears(current => {
            const updated = new Set(current);
            updated.delete(field.key);
            return updated;
          });
        }
        setDraft(next);
        setClears(new Set());
      }
      setSaved(true);
      setPresetNotice('');
      if (review) setDismissDomains(true);
    } catch (cause) {
      setError(message(cause));
    } finally {
      setSaving(false);
    }
  };
  const save = async () => {
    if (!changes.length || saving || loading || !loaded) {
      return;
    }
    const invalid = changes.map(field => ({
      field,
      error: validateSetting(field, clears.has(field.key) ? '' : draft[field.key] ?? '')
    })).find(item => item.error);
    if (invalid) {
      setError(invalid.error);
      setSearch('');
      window.setTimeout(() => [...(surface.current?.querySelectorAll<HTMLElement>('input,textarea,select') || [])].find(element => element.id === `rfw-${invalid.field.key}`)?.focus(), 0);
      return;
    }
    if (pathname === '/system-settings/auth/passkey' && changes.some(field => (domainKeys as readonly string[]).includes(field.key))) {
      await previewDomains();
    } else await persist();
  };
  useWorkspaceRefresh(() => {
    if (saving || changes.length || clearField || domainReview || clears.size) {
      toast.info(saving ? '配置正在保存，请稍后刷新。' : '当前更改已保留，请先保存或撤销修改，再更新配置。');
      return;
    }
    return load();
  });
  const query = search.trim().toLowerCase();
  const visible = page.fields.filter(field => !query || `${field.label} ${field.hint || ''}`.toLowerCase().includes(query));
  const groups = [{
    id: 'switches',
    title: '启用规则',
    description: '决定哪些策略对当前网关生效。',
    fields: visible.filter(field => field.kind === 'toggle')
  }, {
    id: 'parameters',
    title: '连接与参数',
    description: '填写服务地址、限制值和认证信息。',
    fields: visible.filter(field => field.kind !== 'toggle' && field.kind !== 'multiline')
  }, {
    id: 'rules',
    title: '内容与规则',
    description: '编辑面向用户的内容和结构化规则。',
    fields: visible.filter(field => field.kind === 'multiline')
  }].filter(group => group.fields.length);
  return <section className='rfw-page rfw-settings-page' ref={surface} aria-busy={loading || saving || undefined}>
      <header className='rfw-header'>
        <div>
          <a className='rfw-settings-back' href='#settings'>
            <ChevronRight size={14} />
            安全与设置
          </a>
          <h1>{page.title}</h1>
          <p>{page.description}</p>
          {page.related?.length ? <div className='rfw-toolbar'>{page.related.map(link => <a key={link.href} href={link.href}>{link.label}<ArrowUpRight size={15} /></a>)}</div> : null}
        </div>
        <button type='button' className='rfw-icon-button' title='重新读取已保存的配置' aria-label='重新读取已保存的配置' onClick={() => void load()} disabled={loading || saving || !!changes.length}>
          <RotateCcw size={18} />
        </button>
      </header>
      {pathname === '/system-settings/billing/payment-channels' && loaded && <PaymentActivation confirmed={initial['payment_setting.compliance_confirmed'] === 'true'} disabled={loading || saving || !!changes.length} updated={load} />}
      {error && <div className='rfw-message rfw-error' role='alert'>
          <AlertCircle size={17} />
          {error}
        </div>}
      {saved && <div className='rfw-message rfw-success' role='status'>
          <Check size={17} />
          设置已保存
        </div>}
      {(() => {
      if (loading && !loaded) {
        return <div className='rfw-loading' role='status'>
          正在读取网关配置…
        </div>;
      } else {
        if (!loaded) {
          return <div className='rfw-empty'>
          <h2>暂时无法读取配置</h2>
          <p>重新读取成功后即可继续编辑。</p>
          <button type='button' className='rfw-secondary' onClick={() => void load()}>
            重新读取
          </button>
        </div>;
        } else {
          if (page.fields.length) {
            return <>
          {presets.length > 0 && <section className='rfw-setting-presets' aria-label='一键填写预设'>
            <div><h2>一键填写</h2><p>选择使用场景，先填写下方表单，检查后再保存。</p></div>
            <div className='rfw-setting-preset-grid'>{presets.map(preset => <article key={preset.id}>
              <button type='button' className='rfw-secondary' disabled={saving || loading || Boolean(preset.unavailable)} onClick={() => applyPreset(preset)}>{preset.label}</button>
              <p>{preset.description}</p>
              {preset.unavailable && <small>{preset.unavailable}</small>}
            </article>)}</div>
            {presetNotice && <p className='rfw-preset-notice' role='status'>{presetNotice}</p>}
          </section>}
          <div className='rfw-settings-overview'>
            <div>
              <Settings2 size={19} />
              <b>{page.fields.length} 项配置</b>
              <span>
                {page.fields.filter(field => field.kind === 'toggle' && draft[field.key] === 'true').length}{' '}
                项规则已开启
              </span>
            </div>
            <label className='rfw-settings-search'>
              <Search size={17} />
              <input type='search' aria-label='查找本页设置' placeholder='查找本页设置' value={search} onChange={event => setSearch(event.target.value)} />
            </label>
          </div>
          {loading && <div className='rfw-refresh-indicator' aria-label='正在更新配置' />}
          <form onSubmit={event => {
                event.preventDefault();
                void save();
              }}>
            <div className='rfw-settings-sections'>
              {groups.map((group, index) => <section className='rfw-settings-section' key={group.id} aria-labelledby={`rfw-group-${group.id}`}>
                  <div className='rfw-settings-section-title'>
                    <span>{String(index + 1).padStart(2, '0')}</span>
                    <h2 id={`rfw-group-${group.id}`}>{group.title}</h2>
                    <p>{group.description}</p>
                  </div>
                  <div className='rfw-settings-section-fields'>
                    {group.fields.map(field => {
                        const hint = field.hint || (field.format === 'json' ? '使用 JSON 对象或数组配置；保存前会检查格式。' : '');
                        const changed = changes.some(item => item.key === field.key);
                        const credential = field.kind === 'secret' || initial[`${field.key}Configured`] !== undefined;
                        const configured = initial[`${field.key}Configured`] === 'true' || Boolean(initial[field.key]);
                        return <div className={`rfw-field rfw-setting-row${field.kind === 'toggle' ? ' rfw-setting-toggle' : ''}${changed ? ' rfw-setting-changed' : ''}`} key={field.key}>
                          <div className='rfw-setting-label'>
                            <label htmlFor={`rfw-${field.key}`}>
                              {field.label}
                            </label>
                            {changed && <span className='rfw-setting-draft'>待保存</span>}
                            {hint && <small>{hint}</small>}
                            {credential && <small>
                                {(() => {
                                if (clears.has(field.key)) {
                                  return '已选择清除，保存配置后生效。';
                                } else {
                                  if (configured) {
                                    return '已有配置值；留空保留，填写新值后替换。';
                                  } else {
                                    return '尚未配置，填写后保存生效。';
                                  }
                                }
                              })()}
                              </small>}
                          </div>
                          <FieldControl field={field} value={draft[field.key] ?? ''} onChange={value => {
                            setDraft(current => ({
                              ...current,
                              [field.key]: value
                            }));
                            setClears(current => {
                              const updated = new Set(current);
                              updated.delete(field.key);
                              return updated;
                            });
                            setSaved(false);
                          }} disabled={saving || loading} />
                          {credential && configured && <div className='rfw-secret-actions'>
                              <span>当前值由服务器保管</span>
                              <button type='button' className={clears.has(field.key) ? 'rfw-secondary' : 'rfw-danger'} disabled={saving || loading} onClick={() => {
                              if (clears.has(field.key)) {
                                setClears(current => {
                                  const updated = new Set(current);
                                  updated.delete(field.key);
                                  return updated;
                                });
                                setError('');
                              } else {
                                setClearField(field);
                                setDismissClear(false);
                              }
                            }}>
                                {clears.has(field.key) ? '保留现有配置' : `清除${field.label}`}
                              </button>
                            </div>}
                        </div>;
                      })}
                  </div>
                </section>)}
            </div>
            {!groups.length && <div className='rfw-empty'>
                没有找到相关设置，换个关键词试试。
              </div>}
            <footer className='rfw-actions rfw-settings-savebar'>
              <span role='status'>
                {changes.length ? `${changes.length} 项更改等待保存` : '配置已与网关同步'}
              </span>
              <div>
                <button type='button' className='rfw-secondary' disabled={!changes.length || saving || loading} onClick={() => {
                      setDraft(initial);
                      setClears(new Set());
                      setError('');
                      setSaved(false);
                      setPresetNotice('');
                    }}>
                  撤销更改
                </button>
                <button type='submit' className='rfw-primary' disabled={!changes.length || saving || loading}>
                  <Save size={17} />
                  {saving ? '正在保存…' : '保存配置'}
                </button>
              </div>
            </footer>
          </form>
        </>;
          } else {
            return <div className='rfw-empty'>
          <p>{page.title}由独立管理页处理。</p>
          <a href='/admin/#settings'>返回安全与设置</a>
        </div>;
          }
        }
      }
    })()}
      {clearField && <Modal title='清除已保存的配置' close={() => {
      setClearField(null);
      setDismissClear(false);
    }} dismiss={dismissClear}>
          <p>
            保存配置时将清除「{clearField.label}
            」。使用此配置的连接需要重新填写有效值后才能继续使用。
          </p>
          <p className='rfw-help'>
            服务器仍保留当前值，确认后也需要点击页面的“保存配置”才会执行清除。
          </p>
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' data-modal-close>
              保留配置
            </button>
            <button type='button' className='rfw-danger' disabled={dismissClear} onClick={() => {
          setClears(current => new Set(current).add(clearField.key));
          setDraft(current => ({
            ...current,
            [clearField.key]: ''
          }));
          setDismissClear(true);
          setSaved(false);
          setError('');
        }}>
              选择清除
            </button>
          </div>
        </Modal>}
      {domainReview && <Modal title='检查通行密钥域名变更' busy={saving} dismiss={dismissDomains} close={() => {
      setDomainReview(null);
      setDomainConfirmed(false);
      setDismissDomains(false);
    }}>
          <Notice error={error} success={(() => {
        if (dismissDomains) {
          return '域名与策略已保存。';
        } else {
          if (domainReview.applied) {
            return '域名已保存，可重试剩余策略。';
          } else {
            return '';
          }
        }
      })()} />
          <p>
            先检查服务域名与保留的旧域名。现有通行密钥绑定到原域名，移除对应域名后可能无法再次使用。
          </p>
          <dl className='rfw-domain-summary'>
            <div>
              <dt>当前服务域名</dt>
              <dd>{label(domainReview.details.previous_rp_id)}</dd>
            </div>
            <div>
              <dt>保存后的服务域名</dt>
              <dd>{label(domainReview.details.effective_rp_id)}</dd>
            </div>
            <div>
              <dt>保留的旧服务域名</dt>
              <dd>{label(domainReview.details.legacy_rp_ids)}</dd>
            </div>
            <div>
              <dt>允许的网站来源</dt>
              <dd>{label(domainReview.details.origins)}</dd>
            </div>
          </dl>
          {Array.isArray(domainReview.details.removed_rp_ids) && domainReview.details.removed_rp_ids.length > 0 && <div className='rfw-domain-impact'>
                <strong>将移除的域名</strong>
                <p>
                  {domainReview.details.removed_rp_ids.map(String).join('、')}
                </p>
                <p>
                  受影响的凭据{' '}
                  {Number(domainReview.details.affected_credentials || 0)}{' '}
                  个；无法确认原域名的历史凭据{' '}
                  {Number(domainReview.details.unknown_credentials || 0)} 个。
                </p>
              </div>}
          {Boolean(domainReview.details.confirmation_required) && !domainReview.applied && <label className='rfw-domain-confirm'>
                <input type='checkbox' checked={domainConfirmed} disabled={saving} onChange={event => setDomainConfirmed(event.target.checked)} />
                <span>我已检查受影响的凭据，确认移除这些服务域名</span>
              </label>}
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' data-modal-close disabled={saving}>
              返回修改
            </button>
            {!domainReview.applied && <button type='button' className='rfw-secondary' disabled={saving} onClick={() => void previewDomains()}>
                重新检查
              </button>}
            <button type='button' className='rfw-primary' disabled={saving || dismissDomains || !domainReview.applied && Boolean(domainReview.details.confirmation_required) && !domainConfirmed} onClick={() => void persist(domainReview)}>
              {(() => {
            if (saving) {
              return '正在保存…';
            } else {
              if (domainReview.applied) {
                return '重试保存剩余策略';
              } else {
                return '确认保存域名与策略';
              }
            }
          })()}
            </button>
          </div>
        </Modal>}
    </section>;
}
const oauthFields = [{
  key: 'name',
  label: '登录服务名称',
  required: true,
  hint: '用户在登录页面看到的名称。'
}, {
  key: 'slug',
  label: '服务标识',
  required: true,
  hint: '使用小写字母、数字、连字符或下划线。'
}, {
  key: 'client_id',
  label: '应用标识',
  required: true
}, {
  key: 'client_secret',
  label: '应用密钥',
  required: true
}, {
  key: 'authorization_endpoint',
  label: '授权地址',
  required: true,
  url: true
}, {
  key: 'token_endpoint',
  label: '令牌交换地址',
  required: true,
  url: true
}, {
  key: 'user_info_endpoint',
  label: '账户信息地址',
  required: true,
  url: true
}, {
  key: 'scopes',
  label: '授权范围',
  hint: '使用空格分隔，例如 openid profile email。'
}, {
  key: 'user_id_field',
  label: '用户唯一标识字段'
}, {
  key: 'username_field',
  label: '用户名字段'
}, {
  key: 'display_name_field',
  label: '显示名称字段'
}, {
  key: 'email_field',
  label: '邮箱字段'
}] as const;
export function OAuthServicesScreen() {
  const state = useResource(() => read('/api/custom-oauth-provider/').then(asRows));
  const [draft, setDraft] = useState<Row | null>(null);
  const [remove, setRemove] = useState<Row | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [dismissEditor, setDismissEditor] = useState(false);
  const [dismissRemove, setDismissRemove] = useState(false);
  const [discovering, setDiscovering] = useState(false);
  const edit = (service?: Row) => {
    setError('');
    setSuccess('');
    setDismissEditor(false);
    setDraft(service ? {
      ...service,
      client_secret: ''
    } : {
      enabled: false,
      scopes: 'openid profile email',
      user_id_field: 'sub',
      username_field: 'preferred_username',
      display_name_field: 'name',
      email_field: 'email',
      auth_style: 0
    });
  };
  const save = async () => {
    if (!draft || busy || dismissEditor) return;
    const required = oauthFields.find(field => 'required' in field && field.required && !(field.key === 'client_secret' && draft.id) && !String(draft[field.key] || '').trim());
    if (required) {
      setError(`请填写${required.label}。`);
      return;
    }
    if (!/^[a-z0-9_-]+$/.test(String(draft.slug || ''))) {
      setError('服务标识只能使用小写字母、数字、连字符或下划线。');
      return;
    }
    setBusy(true);
    setError('');
    setSuccess('');
    try {
      const payload = Object.fromEntries(oauthFields.map(field => [field.key, String(draft[field.key] || '').trim()]));
      await write(draft.id ? `/api/custom-oauth-provider/${encodeURIComponent(String(draft.id))}` : '/api/custom-oauth-provider/', draft.id ? 'PUT' : 'POST', {
        ...payload,
        enabled: Boolean(draft.enabled),
        well_known: String(draft.well_known || '').trim(),
        auth_style: Number(draft.auth_style) || 0,
        access_policy: String(draft.access_policy || ''),
        access_denied_message: String(draft.access_denied_message || '')
      });
      setSuccess('登录服务已保存。');
      setDismissEditor(true);
      void state.reload();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  const discover = async () => {
    if (!draft || discovering || busy) return;
    const url = String(draft.well_known || '').trim();
    if (!url) {
      setError('请先填写身份服务的发现地址。');
      return;
    }
    setDiscovering(true);
    setError('');
    setSuccess('');
    try {
      const response = asRow(await write('/api/custom-oauth-provider/discovery', 'POST', {
        well_known_url: url
      }));
      const discovered = asRow(response.discovery ?? response.document ?? response);
      setDraft(current => current && {
        ...current,
        authorization_endpoint: discovered.authorization_endpoint || current.authorization_endpoint,
        token_endpoint: discovered.token_endpoint || current.token_endpoint,
        user_info_endpoint: discovered.userinfo_endpoint || discovered.user_info_endpoint || current.user_info_endpoint
      });
      setSuccess('已填写身份服务提供的连接地址，请检查后保存。');
    } catch (cause) {
      setError(message(cause));
    } finally {
      setDiscovering(false);
    }
  };
  return <Page title='自定义身份服务' description='为 further-api 接入自己的组织账户或第三方登录服务。' loading={state.loading} error={state.error} reload={state.reload} actions={<button type='button' className='rfw-primary' disabled={busy} onClick={() => edit()}>
          <Plus size={17} />
          添加登录服务
        </button>}>
      {!draft && !remove && <Notice error={error} success={success} />}
      <div className='rfw-oauth-intro'>
        <ShieldCheck size={22} />
        <div>
          <strong>统一登录，清晰授权</strong>
          <p>先填写应用凭据与连接地址，检查回调地址后再启用登录服务。</p>
        </div>
      </div>
      {state.data?.length ? <div className='rfw-oauth-services'>
          {state.data.map(service => <article key={String(service.id)} className='rfw-oauth-service'>
              <header>
                <h2>{label(service.name)}</h2>
                <span className='rfw-security-badge'>
                  {service.enabled ? '已启用' : '未启用'}
                </span>
              </header>
              <p>服务标识 · {label(service.slug)}</p>
              <div className='rfw-oauth-callback'>
                <span>应用回调地址</span>
                <code>
                  {location.origin}/api/oauth/
                  {encodeURIComponent(String(service.slug || ''))}
                </code>
              </div>
              <footer>
                <button type='button' className='rfw-secondary' onClick={() => edit(service)}>
                  编辑配置
                </button>
                <button type='button' className='rfw-danger' onClick={() => {
            setError('');
            setSuccess('');
            setDismissRemove(false);
            setRemove(service);
          }}>
                  删除服务
                </button>
              </footer>
            </article>)}
        </div> : <Empty>还没有自定义身份服务，添加后即可配置组织登录。</Empty>}
      {draft && <Modal title={draft.id ? '编辑登录服务' : '添加登录服务'} close={() => setDraft(null)} busy={busy || discovering} dismiss={dismissEditor}>
          <Notice error={error} success={success} />
          <form onSubmit={event => {
        event.preventDefault();
        void save();
      }}>
            <div className='rfw-oauth-discovery'>
              <div className='rfw-field'>
                <label htmlFor='rfw-oauth-discovery'>
                  身份服务发现地址（可选）
                </label>
                <input id='rfw-oauth-discovery' type='url' placeholder='https://example.com/.well-known/openid-configuration' value={String(draft.well_known || '')} onChange={event => setDraft({
              ...draft,
              well_known: event.target.value
            })} disabled={busy || discovering} />
              </div>
              <button type='button' className='rfw-secondary' disabled={busy || discovering} onClick={() => void discover()}>
                {discovering ? '正在获取…' : '自动填写地址'}
              </button>
            </div>
            <div className='rfw-settings-grid'>
              {oauthFields.map(field => <div className='rfw-field' key={field.key}>
                  <label htmlFor={`rfw-oauth-${field.key}`}>
                    {field.label}
                  </label>
                  <input id={`rfw-oauth-${field.key}`} type={(() => {
              if (field.key === 'client_secret') {
                return 'password';
              } else {
                if ('url' in field && field.url) {
                  return 'url';
                } else {
                  return 'text';
                }
              }
            })()} value={String(draft[field.key] || '')} onChange={event => setDraft({
              ...draft,
              [field.key]: event.target.value
            })} required={'required' in field && field.required && !(field.key === 'client_secret' && draft.id)} autoComplete={field.key === 'client_secret' ? 'new-password' : 'off'} disabled={busy || discovering} pattern={field.key === 'slug' ? '[a-z0-9_-]+' : undefined} />
                  {'hint' in field && <small>{field.hint}</small>}
                  {field.key === 'client_secret' && draft.id ? <small>留空会保留现有应用密钥。</small> : null}
                </div>)}
            </div>
            <details className='rfw-oauth-advanced'>
              <summary>高级授权规则</summary>
              <div className='rfw-field'>
                <label htmlFor='rfw-oauth-auth-style'>应用凭据传递方式</label>
                <select id='rfw-oauth-auth-style' value={String(draft.auth_style || 0)} disabled={busy || discovering} onChange={event => setDraft({
              ...draft,
              auth_style: Number(event.target.value)
            })}>
                  <option value='0'>自动选择</option>
                  <option value='1'>在请求内容中发送</option>
                  <option value='2'>通过请求头发送</option>
                </select>
              </div>
              <div className='rfw-field'>
                <label htmlFor='rfw-oauth-access-policy'>
                  访问判断规则（可选）
                </label>
                <textarea id='rfw-oauth-access-policy' value={String(draft.access_policy || '')} disabled={busy || discovering} onChange={event => setDraft({
              ...draft,
              access_policy: event.target.value
            })} />
                <small>
                  使用服务端支持的访问规则判断外部账户是否允许登录。
                </small>
              </div>
              <div className='rfw-field'>
                <label htmlFor='rfw-oauth-denied-message'>
                  未获授权时的提示
                </label>
                <input id='rfw-oauth-denied-message' value={String(draft.access_denied_message || '')} disabled={busy || discovering} onChange={event => setDraft({
              ...draft,
              access_denied_message: event.target.value
            })} />
              </div>
            </details>
            <label className='rfw-switch'>
              <input type='checkbox' checked={Boolean(draft.enabled)} disabled={busy || discovering} onChange={event => setDraft({
            ...draft,
            enabled: event.target.checked
          })} />
              向用户开放此登录服务
            </label>
            <div className='rfw-actions'>
              <button type='button' className='rfw-secondary' data-modal-close disabled={busy || discovering}>
                取消
              </button>
              <button type='submit' className='rfw-primary' disabled={busy || discovering || dismissEditor}>
                {busy ? '正在保存…' : '保存登录服务'}
              </button>
            </div>
          </form>
        </Modal>}
      {remove && <Modal title='删除登录服务' close={() => setRemove(null)} busy={busy} dismiss={dismissRemove}>
          <Notice error={error} success={success} />
          <p>
            删除「{label(remove.name)}
            」后，用户将无法通过此服务登录。请先确认受影响的用户仍有其他登录方式。
          </p>
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
              保留服务
            </button>
            <button type='button' className='rfw-danger' disabled={busy || dismissRemove} onClick={() => {
          setBusy(true);
          setError('');
          void write(`/api/custom-oauth-provider/${encodeURIComponent(String(remove.id))}`, 'DELETE').then(async () => {
            setSuccess('登录服务已删除。');
            setDismissRemove(true);
            void state.reload();
          }).catch(cause => setError(message(cause))).finally(() => setBusy(false));
        }}>
              {busy ? '正在删除…' : '确认删除'}
            </button>
          </div>
        </Modal>}
    </Page>;
}
export function UnsupportedScreen({
  path
}: {
  path: string;
}) {
  return <section className='rfw-page'>
      <div className='rfw-empty'>
        <AlertCircle size={26} />
        <h1>此功能地址不可用</h1>
        <p>{label(path)}</p>
        <a href={location.pathname.startsWith('/admin') ? '/admin/#overview' : '/console/#overview'}>
          返回总览
        </a>
      </div>
    </section>;
}
