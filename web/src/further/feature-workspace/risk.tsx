import '../../../../further-ui/assets/risk.css';
import './risk-workspace.css';
import { Activity, ArrowRight, Check, CheckCheck, Clock3, Eye, Filter, Fingerprint, Layers3, LockKeyhole, RefreshCw, Search, ShieldAlert, ShieldCheck, SlidersHorizontal, Sparkles, Users } from 'lucide-react';
import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react';
import { toast } from 'sonner';
import { useAuthStore } from '@/stores/auth-store';
import { asRow, read, write } from './api';
import { Modal, Notice } from './ui';
import { message, useResource } from "./workspace-resource";

const signals = [['regular_timing', '规律探测', '请求以相近间隔反复出现'], ['repeated_input', '重复输入', '多次提交相同或相近内容'], ['multi_model', '模型轮询', '在多个模型之间频繁切换'], ['post_402', '额度耗尽后继续请求', '余额不足后仍持续发起调用'], ['non_stream', '非流式调用', '集中使用一次性返回的请求'], ['night_activity', '深夜活跃', '在北京时间深夜持续调用'], ['post_channel_unavailable', '故障后持续重试', '渠道不可用后仍重复请求'], ['blocked_liveness', '测活拦截', '小体量探测请求命中拦截'], ['multi_ip', '来源分散', '同一账户出现多个请求来源']] as const;
type RiskStatus = 'unreviewed' | 'watching' | 'confirmed' | 'dismissed';
type RiskWindow = '3d' | '7d';
type RiskUser = {
  user_id: number;
  username: string;
  score: number;
  level: string;
  status: RiskStatus;
  signals: Record<(typeof signals)[number][0], number>;
  request_count: number;
  ip_count: number;
  last_request_at: number | string | null;
};
type RiskUserDetail = RiskUser & {
  evidence: Record<string, unknown>;
};
type RiskList = {
  items: RiskUser[];
  total: number;
};
const levels: Record<string, string> = {
  low: '正常',
  medium: '需要关注',
  high: '高风险',
  critical: '严重风险'
};
const statuses: Record<RiskStatus, string> = {
  unreviewed: '待复核',
  watching: '持续观察',
  confirmed: '已确认风险',
  dismissed: '已排除'
};
const evidenceLabels: Record<string, string> = {
  requests: '采样请求',
  distinct_models: '使用模型',
  distinct_ips: '来源 IP 数',
  repeated_inputs: '重复内容请求',
  regular_intervals: '规律间隔次数',
  post_402_requests: '额度不足后请求',
  non_stream_requests: '非流式请求',
  night_requests: '深夜请求',
  post_channel_unavailable_requests: '故障后请求',
  blocked_liveness_requests: '拦截的测活请求',
  sample_limited: '是否达到采样上限'
};
const numberFormatter = new Intl.NumberFormat('zh-CN');
const timeFormatter = new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
const count = (value: unknown) => numberFormatter.format(Math.max(0, Number(value) || 0));
const score = (value: unknown) => Math.min(100, Math.max(0, Math.round(Number(value) || 0)));
function moment(value: number | string | null) {
  if (!value) return '暂无请求';
  const parsed = new Date(typeof value === 'number' ? value * 1000 : value);
  return Number.isNaN(parsed.getTime()) ? '暂无请求' : timeFormatter.format(parsed);
}
function RiskScore({
  value,
  level
}: {
  value: number;
  level: string;
}) {
  return <div className={`frisk-score frisk-${level}`} style={{
    '--risk-score': `${score(value)}%`
  } as CSSProperties} aria-label={`风险评分 ${score(value)} 分`}><span><strong>{score(value)}</strong><small>风险分</small></span></div>;
}
function DetectionPolicy() {
  const role = useAuthStore(state => Number(state.auth.user?.role || 0));
  const resource = useResource(async () => role >= 100 ? asRow(asRow(await read('/api/option/request_policy')).options) : {}, [role]);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [initial, setInitial] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    if (!resource.data) return;
    const source = resource.data;
    const values = Object.fromEntries(['CheckSensitiveEnabled', 'CheckSensitiveOnPromptEnabled', 'SensitiveWords'].map(key => [key, String(source[key] ?? (key === 'SensitiveWords' ? '' : 'false'))]));
    let active = true;
    queueMicrotask(() => { if (active) { setDraft(values); setInitial(values); } });
    return () => { active = false; };
  }, [resource.data]);
  const changed = Object.keys(draft).filter(key => draft[key] !== initial[key]);
  const words = (draft.SensitiveWords || '').split('\n').map(word => word.trim()).filter(Boolean);
  const update = (key: string, value: string) => {
    setDraft(current => ({
      ...current,
      [key]: value
    }));
    setSaved(false);
  };
  const save = async () => {
    if (!changed.length || busy || resource.loading || !resource.data || resource.error) return;
    setBusy(true);
    setError('');
    setSaved(false);
    try {
      const options = Object.fromEntries(changed.map(key => [key, draft[key]]));
      const result = asRow(await write('/api/option/request_policy', 'PATCH', {
        options
      }));
      const next = {
        ...draft,
        ...Object.fromEntries(Object.entries(asRow(result.options)).filter(([key]) => key in draft).map(([key, value]) => [key, String(value)]))
      };
      setDraft(next);
      setInitial(next);
      setSaved(true);
      toast.success('检测策略已保存，新请求将使用更新后的规则。');
    } catch (cause) {
      setError(message(cause));
      toast.error('检测策略保存失败，请检查提示后重试。');
    } finally {
      setBusy(false);
    }
  };
  if (role < 100) return <div className='frisk-empty'><LockKeyhole size={32} /><h2>检测策略由超级管理员维护</h2><p>你仍可查看风险账户、检查证据并提交复核结果。</p></div>;
  return <div className='frisk-policy frisk-enter'><Notice error={resource.error || error} success={saved ? '检测策略已保存。' : ''} />
    {(() => {
      if (resource.loading && !resource.data) {
        return <div className='frisk-loading' role='status'><RefreshCw size={20} />正在读取当前检测策略…</div>;
      } else {
        if (!resource.data) {
          return <div className='frisk-empty'><h2>检测策略暂时无法读取</h2><p>读取成功后即可继续编辑已保存的检测范围和词条。</p><button type='button' className='frisk-button' onClick={() => void resource.reload()}>重新读取策略</button></div>;
        } else {
          return <>
      {resource.loading && <div className='rfw-refresh-indicator' aria-label='正在更新检测策略' />}
      <div className='frisk-section-heading'><div><span className='frisk-kicker'>新请求保护</span><h2>内容检测与测活词条</h2><p>明确检测范围，再维护需要拦截的词条。</p></div><button type='button' className='frisk-button' onClick={() => void resource.reload()} disabled={busy || resource.loading || !!changed.length}><RefreshCw size={16} />读取最新</button></div>
      <div className='frisk-policy-grid'><div className='frisk-policy-main'>
        {[['CheckSensitiveEnabled', '启用内容检测', '让网关对请求内容执行已配置的词条检测。'], ['CheckSensitiveOnPromptEnabled', '检测请求提示词', '将用户提交的提示词纳入检测范围。']].map(([key, title, description]) => <label className='frisk-toggle-row' key={key}><span><strong>{title}</strong><small>{description}</small></span><input type='checkbox' checked={draft[key] === 'true'} disabled={busy || resource.loading || !!resource.error} onChange={event => update(key, String(event.target.checked))} /><i aria-hidden='true' /></label>)}
        <div className='frisk-word-editor'><label htmlFor='frisk-words'>拦截词条 <span>{words.length} 条</span></label><p>每行填写一个词条。删除内容并保存可清空词条列表。</p><textarea id='frisk-words' rows={8} value={draft.SensitiveWords || ''} disabled={busy || resource.loading || !!resource.error} onChange={event => update('SensitiveWords', event.target.value)} placeholder='在此填写需要检测的词条，每行一条' spellCheck={false} /></div>
      </div><aside className='frisk-policy-aside'><ShieldCheck size={30} /><h3>先验证，再扩大范围</h3><p>宽泛的词条可能命中正常请求。建议从明确的探测内容开始，再结合风险账户的聚合证据调整。</p><ol><li>开启需要的检测范围</li><li>维护清晰、具体的词条</li><li>保存后观察新请求与复核队列</li></ol><div className='frisk-info'><Fingerprint size={18} /><span>行为评分持续结合九类信号。内容检测与人工复核分别生效。</span></div></aside></div>
      <footer className='frisk-savebar'><span role='status'>{changed.length ? `${changed.length} 项修改尚未保存` : '当前策略与服务器一致'}</span><div><button type='button' className='frisk-button' disabled={!changed.length || busy || resource.loading} onClick={() => {
                  setDraft(initial);
                  setError('');
                }}>还原修改</button><button type='button' className='frisk-button frisk-button-primary' disabled={!changed.length || busy || resource.loading || !!resource.error} onClick={() => void save()}><Check size={17} />{busy ? '正在保存…' : '保存检测策略'}</button></div></footer>
    </>;
        }
      }
    })()}
  </div>;
}
function hostRiskEntranceRunning(workspace: HTMLElement | null): boolean {
  const owner = workspace?.closest<HTMLElement>('[data-native-feature-root]');
  if (!owner || owner.dataset.nativeMotionEngine !== 'web') return false;
  if (owner.dataset.nativeMotionRunning === 'true') return true;
  return owner.getAnimations?.({ subtree: false }).some(animation => animation.playState === 'running' || animation.pending) ?? false;
}
export function RiskScreen() {
  const workspace = useRef<HTMLElement>(null);
  const [entryMotion, setEntryMotion] = useState<'initial' | 'local'>('initial');
  const [tab, setTab] = useState<'overview' | 'review' | 'policy'>('overview');
  const [listView, setListView] = useState<'overview' | 'review'>('overview');
  const [policyOpened, setPolicyOpened] = useState(false);
  const [window, setWindow] = useState<RiskWindow>('7d');
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [username, setUsername] = useState('');
  const [level, setLevel] = useState('');
  const [status, setStatus] = useState('');
  const activeLevel = listView === 'overview' ? 'attention' : level;
  const state = useResource(async () => (await read('/api/risk/users', {
    window,
    page,
    page_size: 12,
    username,
    level: activeLevel,
    status
  })) as RiskList, [window, page, username, activeLevel, status]);
  const enterLocalView = () => setEntryMotion(hostRiskEntranceRunning(workspace.current) ? 'initial' : 'local');
  useEffect(() => {
    if (!state.data) return;
    let active = true;
    queueMicrotask(() => { if (active) setEntryMotion(hostRiskEntranceRunning(workspace.current) ? 'initial' : 'local'); });
    return () => { active = false; };
  }, [state.data]);
  const [selected, setSelected] = useState<RiskUserDetail | null>(null);
  const [detailUser, setDetailUser] = useState<RiskUser | null>(null);
  const [detailError, setDetailError] = useState('');
  const [detailBusy, setDetailBusy] = useState(false);
  const detailGeneration = useRef(0);
  const [review, setReview] = useState<{
    user: RiskUser;
    status: RiskStatus;
  } | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [reviewSaved, setReviewSaved] = useState(false);
  const rows = useMemo(() => state.data?.items || [], [state.data?.items]);
  const metrics = useMemo(() => ({
    pending: rows.filter(row => row.status === 'unreviewed').length,
    highest: Math.max(0, ...rows.map(row => score(row.score))),
    requests: rows.reduce((total, row) => total + row.request_count, 0)
  }), [rows]);
  const distribution = useMemo(() => signals.map(([key, title, description]) => ({
    key,
    title,
    description,
    value: rows.length ? Math.round(rows.reduce((total, row) => total + score(row.signals?.[key]), 0) / rows.length) : 0
  })).sort((a, b) => b.value - a.value), [rows]);
  const detail = async (user: RiskUser) => {
    const current = ++detailGeneration.current;
    setDetailUser(user);
    setSelected(null);
    setDetailError('');
    setDetailBusy(true);
    try {
      const result = (await read(`/api/risk/users/${user.user_id}`, {
        window
      })) as RiskUserDetail;
      if (current === detailGeneration.current) setSelected(result);
    } catch (cause) {
      if (current === detailGeneration.current) setDetailError(message(cause));
    } finally {
      if (current === detailGeneration.current) setDetailBusy(false);
    }
  };
  const closeDetail = () => {
    detailGeneration.current++;
    setDetailUser(null);
    setSelected(null);
    setDetailBusy(false);
  };
  useEffect(() => () => {
    detailGeneration.current++;
  }, []);
  const saveReview = async () => {
    if (!review || busy || reviewSaved) return;
    setBusy(true);
    setError('');
    try {
      await write(`/api/risk/users/${review.user.user_id}/status`, 'PATCH', {
        status: review.status
      });
      toast.success(`「${review.user.username}」已标记为${statuses[review.status]}`);
      setReviewSaved(true);
      void state.reload();
    } catch (cause) {
      setError(message(cause));
      toast.error('复核结果未保存，请重试。');
    } finally {
      setBusy(false);
    }
  };
  const changeTab = (next: typeof tab) => {
    if (next === tab) return;
    enterLocalView();
    setTab(next);
    if (next === 'policy') setPolicyOpened(true);else {
      setListView(next);
      if (listView !== next) setPage(1);
    }
    setError('');
  };
  return <section ref={workspace} className='frisk-workspace' data-risk-entry={entryMotion} data-risk-view={tab}>
    <header className='frisk-hero'><div className='frisk-hero-main'><span className='frisk-kicker'><ShieldCheck size={16} />further-api / 账户防护</span><h1>风险预警与反测活</h1><p>读懂异常的来由，给每次复核一个清晰结论。</p><div className='frisk-hero-note'><span />真实请求证据 · 北京时间</div></div><div className='frisk-workflow'><span><Eye size={17} /><b>01</b>发现异常</span><i /><span><Fingerprint size={17} /><b>02</b>核对证据</span><i /><span><CheckCheck size={17} /><b>03</b>记录结论</span><small><Sparkles size={13} />复核状态和检测策略分别保存</small></div></header>
    <nav className='frisk-tabs' role='tablist' aria-label='风险管理视图'>{([{
        key: 'overview',
        title: '风险总览',
        icon: ShieldAlert
      }, {
        key: 'review',
        title: '反测活复核',
        icon: Fingerprint
      }, {
        key: 'policy',
        title: '检测策略',
        icon: SlidersHorizontal
      }] as const).map(({
        key,
        title,
        icon: Icon
      }) => <button id={`frisk-tab-${key}`} key={key} type='button' role='tab' aria-selected={tab === key} aria-controls={`frisk-${key}`} tabIndex={tab === key ? 0 : -1} onKeyDown={event => {
        const tabs = ['overview', 'review', 'policy'] as const;
        if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault();
        const index = tabs.indexOf(key);
        const next = (() => {
          if (event.key === 'Home') {
            return 0;
          } else {
            if (event.key === 'End') {
              return tabs.length - 1;
            } else {
              return (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
            }
          }
        })();
        changeTab(tabs[next]);
        (event.currentTarget.parentElement?.children[next] as HTMLElement)?.focus();
      }} onClick={() => changeTab(key)}><Icon size={18} /><span>{title}</span></button>)}</nav>
    <div id={`frisk-${listView}`} role='tabpanel' aria-labelledby={`frisk-tab-${listView}`} className='frisk-content' key={listView} hidden={tab === 'policy'} aria-busy={state.loading || undefined}>
      <div className='frisk-metrics'>{[{
            title: '匹配账户',
            value: count(state.data?.total),
            hint: window === '7d' ? '最近 7 天 · 当前筛选' : '最近 3 天 · 当前筛选',
            icon: Users
          }, {
            title: '待复核',
            value: count(metrics.pending),
            hint: '本页账户',
            icon: Clock3
          }, {
            title: '最高风险分',
            value: count(metrics.highest),
            hint: '本页账户 · 满分 100',
            icon: Activity
          }, {
            title: '关联请求',
            value: count(metrics.requests),
            hint: '本页账户累计',
            icon: Layers3
          }].map(({
            title,
            value,
            hint,
            icon: Icon
          }, index) => <article className='frisk-enter' style={{ '--risk-delay': `${index * 24}ms` } as CSSProperties} key={title}><div><span>{title}</span><Icon size={19} /></div><strong>{state.loading && !state.data ? '—' : value}</strong><small>{hint}</small></article>)}</div>
      {tab === 'overview' && <section className='frisk-overview frisk-enter'><div><span className='frisk-kicker'>本页行为信号</span><h2>从异常规律开始复核</h2><p>优先查看多种信号同时出现的账户，再结合业务用途判断。</p><button type='button' className='frisk-text-button' onClick={() => changeTab('review')}>查看全部账户 <ArrowRight size={16} /></button></div><div className='frisk-signal-summary'>{distribution.slice(0, 4).map(signal => <div key={signal.key}><span>{signal.title}</span><div><i style={{
                  width: `${signal.value}%`
                }} /></div><b>{signal.value}<small> / 100</small></b></div>)}<p>信号平均分仅统计当前页，分数不等于违规结论。</p></div></section>}
      <section className='frisk-queue'><div className='frisk-section-heading frisk-enter'><div><span className='frisk-kicker'>{tab === 'overview' ? '需要关注' : '账户复核队列'}</span><h2>{tab === 'overview' ? '风险账户动态' : '逐项查看，留存判断'}</h2></div><button type='button' className='frisk-button' disabled={state.loading} onClick={() => { enterLocalView(); void state.reload(); }}><RefreshCw size={16} className={state.loading ? 'frisk-spinning' : ''} />刷新记录</button></div>
        <form className='frisk-filters frisk-enter' onSubmit={event => {
            event.preventDefault();
            enterLocalView();
            setUsername(search.trim());
            setPage(1);
          }}><div className='frisk-search'><Search size={17} /><input aria-label='搜索风险账户' placeholder='搜索用户名' value={search} onChange={event => setSearch(event.target.value)} /><button type='submit'>搜索</button></div><label><span><Filter size={14} />复核状态</span><select value={status} onChange={event => {
                enterLocalView();
                setStatus(event.target.value);
                setPage(1);
              }}><option value=''>全部状态</option>{Object.entries(statuses).map(([key, title]) => <option key={key} value={key}>{title}</option>)}</select></label>{tab === 'review' && <label><span>风险等级</span><select value={level} onChange={event => {
                enterLocalView();
                setLevel(event.target.value);
                setPage(1);
              }}><option value=''>全部等级</option>{Object.entries(levels).map(([key, title]) => <option key={key} value={key}>{title}</option>)}</select></label>}<label><span>采样窗口</span><select value={window} onChange={event => {
                enterLocalView();
                setWindow(event.target.value as RiskWindow);
                setPage(1);
              }}><option value='7d'>最近 7 天</option><option value='3d'>最近 3 天</option></select></label></form>
        <Notice error={state.error} />{state.loading && state.data && <div className='rfw-refresh-indicator' aria-label='正在更新风险记录' />}{(() => {
            if (state.loading && !state.data) {
              return <div className='frisk-skeleton-grid' aria-label='正在读取风险记录' role='status'>{[1, 2, 3].map(item => <div key={item}><i /><i /><i /></div>)}</div>;
            } else {
              if (rows.length) {
                return <div className='frisk-account-grid'>{rows.map((user, index) => <article className={`frisk-account frisk-${user.level}`} key={user.user_id} style={{
                    '--risk-delay': `${Math.min(index, 5) * 35}ms`
                  } as CSSProperties}><div className='frisk-account-top'><div className='frisk-identity'><span className='frisk-avatar'>{(user.username || '#').slice(0, 1)}</span><div><h3>{user.username || `用户 ${user.user_id}`}</h3><small>账户 #{user.user_id}</small></div></div><span className='frisk-level'>{levels[user.level] || '待评估'}</span></div><div className='frisk-account-body'><RiskScore value={user.score} level={user.level} /><dl><div><dt>请求次数</dt><dd>{count(user.request_count)}</dd></div><div><dt>来源 IP</dt><dd>{count(user.ip_count)}</dd></div><div><dt>最近请求</dt><dd>{moment(user.last_request_at)}</dd></div></dl></div><div className='frisk-tags'>{signals.filter(([key]) => Number(user.signals?.[key] || 0) > 0).sort((a, b) => Number(user.signals[b[0]]) - Number(user.signals[a[0]])).slice(0, 3).map(([key, title]) => <span key={key}>{title}</span>)}{!signals.some(([key]) => Number(user.signals?.[key] || 0) > 0) && <span>暂无明显异常信号</span>}</div><footer><span className={`frisk-review-state frisk-state-${user.status}`}><i />{statuses[user.status] || '待复核'}</span><div><button type='button' aria-label={`查看 ${user.username} 的证据`} disabled={state.loading} onClick={() => void detail(user)}><Eye size={15} />证据</button><button type='button' aria-label={`复核 ${user.username}`} disabled={state.loading} onClick={() => {
                          setReview({
                            user,
                            status: user.status === 'unreviewed' ? 'watching' : user.status
                          });
                          setReviewSaved(false);
                          setError('');
                        }}><CheckCheck size={15} />复核</button></div></footer></article>)}</div>;
              } else {
                return <div className='frisk-empty'><ShieldCheck size={36} /><h3>{state.error ? '暂时无法读取风险记录' : '当前没有待查看的风险账户'}</h3><p>{state.error ? '请刷新重试，现有复核记录不会因此改变。' : '可以调整筛选条件，或切换到反测活复核查看全部账户。'}</p></div>;
              }
            }
          })()}
        <div className='frisk-pagination frisk-enter'><span>共 {count(state.data?.total)} 个账户 · 第 {page} 页</span><div><button type='button' className='frisk-button' disabled={page === 1 || state.loading} onClick={() => { enterLocalView(); setPage(current => current - 1); }}>上一页</button><button type='button' className='frisk-button' disabled={page * 12 >= (state.data?.total || 0) || state.loading} onClick={() => { enterLocalView(); setPage(current => current + 1); }}>下一页</button></div></div>
      </section><p className='frisk-footnote'><LockKeyhole size={14} />仅展示聚合行为证据。复核标记会保存到服务器，不会自动封禁账户。</p>
    </div>
    <div id='frisk-policy' role='tabpanel' aria-labelledby='frisk-tab-policy' className='frisk-content' hidden={tab !== 'policy'}>{policyOpened && <DetectionPolicy />}</div>
    {detailUser && <Modal title={`行为证据 · ${detailUser.username}`} close={closeDetail}><div className='frisk-detail'><Notice error={detailError} />{(() => {
          if (detailBusy) {
            return <div className='frisk-loading' role='status'><RefreshCw size={20} />正在读取聚合证据…</div>;
          } else {
            if (selected) {
              return <><div className='frisk-detail-lead'><RiskScore value={selected.score} level={selected.level} /><div><span className={`frisk-level frisk-${selected.level}`}>{levels[selected.level]}</span><h3>{statuses[selected.status]}</h3><p>{count(selected.request_count)} 次请求 · {count(selected.ip_count)} 个来源 IP</p></div></div><h3>九类行为信号</h3><div className='frisk-detail-signals'>{signals.map(([key, title, description]) => <div key={key}><div><strong>{title}</strong><small>{description}</small></div><progress max={100} value={score(selected.signals?.[key])} aria-label={title} /><b>{score(selected.signals?.[key])}</b></div>)}</div><h3>采样证据</h3><dl className='frisk-evidence'>{Object.entries(selected.evidence || {}).filter(([key]) => key in evidenceLabels).map(([key, value]) => <div key={key}><dt>{evidenceLabels[key]}</dt><dd>{(() => {
                        if (typeof value === 'boolean') {
                          if (value) {
                            return '是';
                          } else {
                            return '否';
                          }
                        } else {
                          if (typeof value === 'number') {
                            return count(value);
                          } else {
                            return String(value ?? '—');
                          }
                        }
                      })()}</dd></div>)}</dl><p className='frisk-footnote'>不展示请求原文与原始 IP。请结合账户用途判断风险。</p></>;
            } else {
              return <button type='button' className='frisk-button' onClick={() => void detail(detailUser)}>重新读取证据</button>;
            }
          }
        })()}</div></Modal>}
    {review && <Modal title='保存账户复核' busy={busy} dismiss={reviewSaved} close={() => {
      setReview(null);
      setReviewSaved(false);
    }}><div className='frisk-review'><Notice error={error} success={reviewSaved ? '复核结果已保存。' : ''} /><div className='frisk-review-account'><Fingerprint size={26} /><div><strong>{review.user.username}</strong><span>账户 #{review.user.user_id} · 风险评分 {score(review.user.score)}</span></div></div><p>选择本次复核结论，保存后将更新账户的风险处理状态。</p><div className='frisk-review-options'>{Object.entries(statuses).map(([key, title]) => <label key={key} className={review.status === key ? 'selected' : ''}><input type='radio' name='risk-review-status' value={key} checked={review.status === key} disabled={busy || reviewSaved} onChange={() => setReview({
              ...review,
              status: key as RiskStatus
            })} /><span>{title}</span>{review.status === key && <Check size={17} />}</label>)}</div><div className='frisk-savebar'><span>本操作不会封禁账户</span><button type='button' className='frisk-button frisk-button-primary' disabled={busy || reviewSaved} onClick={() => void saveReview()}>{busy ? '正在保存…' : '保存复核结果'}</button></div></div></Modal>}
  </section>;
}
