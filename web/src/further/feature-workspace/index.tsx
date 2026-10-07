import { isRecordsPath } from "./records-config";
import './workspace.css';
import './account-settings.css';
import { useState } from 'react';
import { api } from '@/lib/api';
import { useAuthStore } from '@/stores/auth-store';
import { AnalyticsScreen, LogsScreen } from './analysis';
import { label } from './api';
import { ENTRY_BY_PATH, settingsPageFor } from './catalog';
import { OperationsScreen } from './operations';
import { PaymentsScreen } from './payments';
import { RecordsScreen } from './records';
import { SecurityScreen } from './security';
import { RiskScreen } from './risk';
import { managementDestination, workspaceShellTarget } from '../workspace-target';
import { RETIRED_FEATURE_PATHS } from '../native-feature-paths';
import { OAuthServicesScreen, SettingsScreen, UnsupportedScreen } from './settings';
import { Notice, Page } from './ui';
import { message } from "./workspace-resource";

const localPages: Record<string, {
  title: string;
  href: string;
  description: string;
}> = {
  '/channels': {
    title: '渠道管理',
    href: '/admin/#channels',
    description: '配置上游渠道、独立 Key、路由与健康检测。'
  },
  '/keys': {
    title: 'API Key',
    href: '/console/#tokens',
    description: '创建令牌、调整访问范围与安全设置。'
  },
  '/users': {
    title: '用户管理',
    href: '/admin/#users',
    description: '管理用户、角色、权限、余额和订阅。'
  },
  '/redemption-codes': {
    title: '兑换码',
    href: '/admin/#redemptions',
    description: '生成、管理和导出兑换码批次。'
  },
  '/risk-control': {
    title: '风险预警与反测活',
    href: '/admin/#risk',
    description: '检查异常调用与账户风险状态。'
  },
  '/subscriptions': {
    title: '订阅与福利',
    href: '/admin/#subscriptions/paid',
    description: '在统一工作区管理付费订阅和福利发放。'
  },
  '/system-settings/billing/payment': {
    title: '支付网关',
    href: '/admin/#settings/payment',
    description: '配置支付渠道和收款信息。'
  },
  '/system-settings/site/notice': {
    title: '站点公告',
    href: '/admin/#settings/announcements',
    description: '统一维护面向用户展示的站点公告。'
  },
  '/usage-logs': {
    title: '调用记录',
    href: '/admin/#logs',
    description: '在统一日志页面查看调用记录和请求追踪。'
  },
  '/usage-logs/common': {
    title: '调用记录',
    href: '/admin/#logs',
    description: '在统一日志页面查看调用记录和请求追踪。'
  }
};
function PlaygroundScreen() {
  const [model, setModel] = useState('');
  const [key, setKey] = useState('');
  const [input, setInput] = useState('');
  const [result, setResult] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const send = async () => {
    setBusy(true);
    setError('');
    setResult('');
    try {
      const response = await api.post('/v1/chat/completions', {
        model,
        messages: [{
          role: 'user',
          content: input
        }],
        stream: false
      }, {
        headers: {
          Authorization: `Bearer ${key}`
        },
        skipAuthRefresh: true
      });
      setResult(JSON.stringify(response.data, null, 2));
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  return <Page title='请求游乐场' description='使用自己的 API Key 验证模型接入；请求会按正常调用计费。'><Notice error={error} /><form onSubmit={event => {
      event.preventDefault();
      void send();
    }}><div className='rfw-settings-grid'><div className='rfw-field'><label htmlFor='rfw-playground-model'>模型标识</label><input id='rfw-playground-model' value={model} onChange={event => setModel(event.target.value)} required /></div><div className='rfw-field'><label htmlFor='rfw-playground-key'>API Key</label><input id='rfw-playground-key' type='password' value={key} onChange={event => setKey(event.target.value)} autoComplete='off' required /></div></div><div className='rfw-field' style={{
        marginTop: 18
      }}><label htmlFor='rfw-playground-input'>输入</label><textarea id='rfw-playground-input' rows={5} value={input} onChange={event => setInput(event.target.value)} required /></div><div className='rfw-actions'><span>Key 仅用于本次页面请求</span><button type='submit' className='rfw-primary' disabled={busy}>{busy ? '请求中…' : '发送请求'}</button></div></form>{result && <pre className='rfw-code'>{result}</pre>}</Page>;
}
export function FeatureWorkspace({
  path
}: {
  path: string;
}) {
  const user = useAuthStore(state => state.auth.user);
  const pathname = path.split('?')[0].replace(/\/+$/, '');
  const entry = ENTRY_BY_PATH.get(pathname);
  const role = entry?.role ?? (() => {
    if (pathname.startsWith('/system-settings') || pathname === '/system-info' || pathname === '/task-plugins') {
      return 100;
    } else {
      if (pathname.startsWith('/models') || ['/users', '/channels', '/subscriptions', '/redemption-codes', '/risk-control'].includes(pathname)) {
        return 10;
      } else {
        return 1;
      }
    }
  })();
  if (!user || Number(user.role) < role) return <Page title='无法访问此功能'><div className='rfw-empty' role='alert'>当前账户没有访问权限。<p><a href={managementDestination(user?.role)}>进入管理页面</a></p></div></Page>;
  if (RETIRED_FEATURE_PATHS[pathname]) return <Page title='此功能已合并'><div className='rfw-card'><p>请使用当前管理页面中的统一入口。</p><a className='rfw-primary' href={workspaceShellTarget(pathname, Number(user.role))}>进入管理页面</a></div></Page>;
  if (pathname === '/security' || pathname === '/profile') return <SecurityScreen />;
  if (pathname === '/risk-control') return <RiskScreen />;
  if (pathname === '/wallet') return <PaymentsScreen />;
  if (pathname === '/system-settings/auth/custom-oauth') return <OAuthServicesScreen />;
  if (isRecordsPath(pathname)) return <RecordsScreen key={pathname} pathname={pathname} />;
  if (pathname.startsWith('/dashboard')) return <AnalyticsScreen pathname={pathname} />;
  if (['/usage-logs/drawing', '/usage-logs/task', '/usage-logs/audit'].includes(pathname)) return <LogsScreen pathname={pathname} />;
  if (pathname === '/task-plugins' || pathname === '/system-info') return <OperationsScreen pathname={pathname} />;
  if (pathname === '/playground') return <PlaygroundScreen />;
  const local = localPages[pathname];
  if (local) return <Page title={local.title} description={local.description}><div className='rfw-card'><h2>{local.title}</h2><p>此功能已接入 further-api 的统一工作区。</p><div className='rfw-toolbar'><a className='rfw-primary' href={local.href}>打开{local.title}</a></div></div></Page>;
  const settings = settingsPageFor(pathname);
  if (settings) return <SettingsScreen key={pathname} page={settings} pathname={pathname} />;
  if (pathname === '/models' || pathname === '/system-settings') return <Page title='管理页面'><a className='rfw-primary' href={workspaceShellTarget(pathname, Number(user.role))}>进入管理页面</a></Page>;
  return <UnsupportedScreen path={label(path)} />;
}
