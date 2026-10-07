import './public-pages.css';
import { useEffect, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { RichContent } from '@/components/rich-content';
import { statusQueryOptions } from '@/lib/status-query';
import { requireServerSuccess } from '@/lib/server-error-message';
import { useAuthStore } from '@/stores/auth-store';
import { api } from '@/lib/api';
import { resolveAuthentication } from '@/lib/auth-session';
import { loadWorkspaceBootstrap } from './workspace-bootstrap';
import { asRow, asRows, label, read } from './feature-workspace/api';
import { PublicAuthPage } from './public-auth';
import { isPublicAuthPath } from "./auth-navigation";
import { PublicAppearance } from './public-appearance';
import { WelcomePage } from './welcome-page';
import { welcomeDestination } from './welcome-flow';
import { MANAGEMENT_ENTRY_LABEL, managementDestination } from './workspace-target';

const copyrightYear = new Date().getFullYear();

function WelcomeRoute() {
  const user = useAuthStore(state => state.auth.user);
  return <WelcomePage displayName={user?.display_name || user?.username} enterLabel={MANAGEMENT_ENTRY_LABEL} appearanceControls={<PublicAppearance />} onComplete={async () => {
    if (!user) throw new Error('登录已过期，请重新登录后继续。');
    requireServerSuccess((await api.post('/api/further/account/welcome')).data);
    useAuthStore.getState().auth.setUser({ ...user, welcome_completed: true });
  }} onEntered={() => location.replace(welcomeDestination(user || { role: 1 }))} />;
}

function LegalPage() {
  return <section className='further-content further-legal'><span className='further-kicker'>further-api / 开源许可</span><h1>许可与原作署名</h1><p>further-api 的产品品牌、页面和工作区由本项目维护。开源代码的作者归属与许可条款如下。</p><article className='further-model-card'><h2>代码许可</h2><p>本项目依照 GNU Affero General Public License v3.0 分发，并包含历史开源贡献。完整许可证、修改说明和第三方许可随源码与发布程序提供。</p><a href='https://github.com/dqt081228OHVGFY/further-api/blob/main/LICENSE' target='_blank' rel='noreferrer'>查看 LICENSE ↗</a><p>Frontend design and development by New API contributors.</p><a href='https://github.com/QuantumNous/new-api' target='_blank' rel='noreferrer'>原作与作者：QuantumNous / New API ↗</a><a href='https://github.com/dqt081228OHVGFY/further-api/blob/main/NOTICE' target='_blank' rel='noreferrer'>查看完整 NOTICE ↗</a></article></section>;
}
export function FurtherPublicFrame({
  children
}: {
  children: ReactNode;
}) {
  const user = useAuthStore(state => state.auth.user);
  const {
    data: status
  } = useQuery(statusQueryOptions);
  const destination = managementDestination(user?.role);
  return <div className='further-public'><header className='further-public-header'><a className='further-brand' href='/'><img src='/further-api-logo.svg?v=handshake-20261004' width={38} height={38} alt='' /><span>further-api<small>AI API 网关</small></span></a><nav aria-label='站点导航'><a href='/#capabilities'>产品能力</a><a href='/pricing'>模型目录</a><a href='/about'>关于</a></nav><div className='further-public-actions'>{user ? <a className='further-button' href={destination}>{MANAGEMENT_ENTRY_LABEL}</a> : <><a href='/auth/?mode=login'>登录</a>{status?.register_enabled !== false && <a className='further-button' href='/auth/?mode=register'>开始使用</a>}</>}<PublicAppearance /></div></header><main>{children}</main><footer className='further-public-footer'><div><a className='further-brand' href='/'>further-api</a><p>统一模型接入、访问控制与用量管理。</p></div><nav aria-label='页脚'><a href='/console/#docs'>接入文档</a><a href='/about'>关于本站</a>{Boolean(status?.user_agreement_enabled) && <a href='/user-agreement'>用户协议</a>}{Boolean(status?.privacy_policy_enabled) && <a href='/privacy-policy'>隐私政策</a>}</nav><small>© {copyrightYear} further-api</small><a className="further-attribution" href="/legal">开源许可</a></footer></div>;
}
function ContentPage({
  path,
  title,
  fallback
}: {
  path: string;
  title: string;
  fallback: ReactNode;
}) {
  const state = useQuery({
    queryKey: ['further-public-content', path],
    queryFn: () => read(path),
    retry: 1
  });
  const value = state.data;
  const record = asRow(value);
  const content = typeof value === 'string' ? value.trim() : String(record.content || '').trim();
  return <section className='further-content'><span className='further-kicker'>further-api</span><h1>{title}</h1>{(() => {
      if (state.isLoading) {
        return <p role='status'>正在载入…</p>;
      } else {
        if (state.error) {
          return <div className='further-error' role='alert'><p>内容暂时无法载入，请稍后重试。</p><button className='further-button' onClick={() => void state.refetch()} type="button">重新载入</button></div>;
        } else {
          if (content) {
            if (/^https?:\/\//.test(content)) {
              return <><p><a className='further-button' href={content} target='_blank' rel='noopener noreferrer'>查看{title}</a></p><iframe title={title} src={content} sandbox='allow-forms allow-popups allow-scripts' className='further-content-frame' /></>;
            } else {
              return <RichContent content={content} mode={/<[a-z][\s\S]*>/i.test(content) ? 'html' : 'markdown'} htmlVariant='isolated' />;
            }
          } else {
            return fallback;
          }
        }
      }
    })()}</section>;
}
function PublicModels({
  modelId
}: {
  modelId?: string;
}) {
  const state = useQuery({
    queryKey: ['further-public-pricing'],
    queryFn: async () => requireServerSuccess((await api.get('/api/pricing')).data),
    retry: 1
  });
  const [search, setSearch] = useState('');
  const rows = asRows(state.data?.data).filter(row => modelId ? String(row.model_name) === modelId || String(row.id) === modelId : label(row.model_name).toLowerCase().includes(search.toLowerCase()));
  return <section className='further-content'><span className='further-kicker'>模型目录</span><h1>{modelId ? '模型详情' : '为你的应用选择模型'}</h1><p>可用范围与计费规则由当前服务配置决定，实际调用价格以模型页面显示的有效价格为准。</p><div className='further-search'><label htmlFor='further-model-search'>搜索模型</label><input id='further-model-search' value={search} onChange={event => setSearch(event.target.value)} placeholder='输入模型名称' /></div>{(() => {
      if (state.error) {
        return <div className='further-error' role='alert'>模型目录暂时无法载入。<button className='further-button' onClick={() => void state.refetch()} type="button">重试</button></div>;
      } else {
        if (state.isLoading) {
          return <p role='status'>正在载入模型…</p>;
        } else {
          if (rows.length) {
            return <div className='further-model-grid'>{rows.map(row => <article className='further-model-card' key={String(row.id ?? row.model_name)}><span className='further-kicker'>{label(row.vendor_name || '模型')}</span><h2>{label(row.model_name)}</h2><p>{label(row.description || '支持统一网关调用。')}</p><dl><div><dt>计费</dt><dd>{(() => {
                        if (row.billing_mode === 'tiered_expr') {
                          return '按使用量动态计费';
                        } else {
                          if (Number(row.quota_type) === 1) {
                            return `$${Number(row.model_price || 0).toFixed(6)} / 次`;
                          } else {
                            return `输入 $${(Number(row.model_ratio || 0) * 2).toFixed(6)} / 百万 Token`;
                          }
                        }
                      })()}</dd></div><div><dt>协议</dt><dd>{Array.isArray(row.supported_endpoint_types) ? row.supported_endpoint_types.join(' · ') : '查看管理页面'}</dd></div></dl><a href='/console/#models'>在管理页面查看</a></article>)}</div>;
          } else {
            return <div className='further-empty'>暂无符合条件的模型。<a href='/console/#models'>前往管理页面查看账户可用模型</a></div>;
          }
        }
      }
    })()}</section>;
}
function Landing() {
  const user = useAuthStore(state => state.auth.user);
  return <><section className='further-hero'><div><span className='further-kicker'>further-api / 模型接入工作区</span><h1>让每一次模型调用<br /><em>清晰、可控。</em></h1><p>一个 API 地址连接多种模型。在同一个工作区管理访问凭据、费用和调用记录。</p><div className='further-hero-actions'><a className='further-button' href={user ? managementDestination(user.role) : '/auth/?mode=login'}>进入管理页面 ↗</a><a className='further-button secondary' href='/pricing'>浏览模型</a></div></div><div className='further-flow' aria-label='应用通过网关连接不同模型'><span>你的应用</span><i>↓</i><strong><img src='/further-api-logo.svg?v=handshake-20261004' width={48} height={48} alt='' />further-api</strong><i>↓</i><div><span>OpenAI</span><span>Anthropic</span><span>Gemini</span></div></div></section><section id='capabilities' className='further-content'><span className='further-kicker'>从接入到观察</span><h2>为应用提供完整的调用工作区</h2><div className='further-model-grid'><article className='further-model-card'><span>01</span><h3>统一接口</h3><p>支持 Responses、Chat Completions、Anthropic 与 Gemini 接口格式。</p></article><article className='further-model-card'><span>02</span><h3>独立访问控制</h3><p>为每个应用创建 API Key，设置模型范围、有效期与 IP 白名单。</p></article><article className='further-model-card'><span>03</span><h3>真实调用记录</h3><p>查看请求结果、Token 构成、耗时与扣费详情。</p></article></div></section><section id='quick-start' className='further-content further-start'><div><span className='further-kicker'>快速开始</span><h2>创建 Key，选择模型，发送请求。</h2><p>将请求地址替换为当前站点，填入自己的 API Key 与可用模型标识。</p><a className='further-button' href='/console/#docs'>打开接入文档</a></div><pre>{`curl ${location.origin}/v1/responses \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"YOUR_MODEL","input":"Hello"}'`}</pre></section></>;
}
export function FurtherPublicApp({
  pathname = location.pathname
}: {
  pathname?: string;
}) {
  useEffect(() => {
    document.title = pathname.replace(/\/+$/, '') === '/welcome' ? '欢迎加入 · further-api' : 'further-api';
    void resolveAuthentication().catch(() => {});
    if (!pathname.startsWith('/setup')) {void loadWorkspaceBootstrap().then(data => {
      if (!data.setup.status) location.replace('/setup');
    }).catch(() => {});}
  }, [pathname]);
  const path = pathname.replace(/\/+$/, '') || '/';
  const title = path === '/privacy-policy' ? '隐私政策' : '用户协议';
  if (path === '/welcome') return <WelcomeRoute />;
  return <FurtherPublicFrame>{(() => {
      if (isPublicAuthPath(path)) {
        return <PublicAuthPage pathname={path} />;
      } else {
        if (path === '/') {
          return <Landing />;
        } else {
          if (path === '/legal') {
            return <LegalPage />;
          } else {
            if (path === '/about') {
              return <ContentPage path='/api/about' title='关于 further-api' fallback={<><p>further-api 是模型接入与账户管理工作区，提供统一 API、访问控制、用量记录、订阅与福利管理。</p><a className='further-button' href='/console/#docs'>查看本站接入文档</a></>} />;
            } else {
              if (path.startsWith('/pricing')) {
                return <PublicModels modelId={path.split('/')[2] ? decodeURIComponent(path.split('/')[2]) : undefined} />;
              } else {
                if (path === '/privacy-policy' || path === '/user-agreement') {
                  return <ContentPage path={`/api${path}`} title={title} fallback={<div className='further-empty'>本站暂未发布{title}。<a href='/'>返回首页</a></div>} />;
                } else {
                  return <section className='further-content'><h1>此页面不可用</h1><p>请从本站导航选择页面。</p><a className='further-button' href='/'>返回首页</a></section>;
                }
              }
            }
          }
        }
      }
    })()}</FurtherPublicFrame>;
}
