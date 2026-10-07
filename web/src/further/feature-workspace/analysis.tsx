import { useState } from 'react';
import { useAuthStore } from '@/stores/auth-store';
import { asRow, asRows, label, pageData, read, type Row } from './api';
import { Empty, Modal, Page } from './ui';
import { useResource } from "./workspace-resource";

const day = (offset = 0) => new Date(Date.now() + offset * 86400000 + 8 * 3600000).toISOString().slice(0, 10);
const timestamp = (date: string) => Math.floor(new Date(`${date}T00:00:00+08:00`).getTime() / 1000);
export function AnalyticsScreen({
  pathname
}: {
  pathname: string;
}) {
  const role = Number(useAuthStore(state => state.auth.user?.role));
  const [start, setStart] = useState(day(-6));
  const [end, setEnd] = useState(day());
  const [range, setRange] = useState({
    start: day(-6),
    end: day()
  });
  const section = pathname.split('/')[2] || 'models';
  const endpoint = (() => {
    if (section === 'users') {
      return '/api/data/users';
    } else {
      if (section === 'flow') {
        return `/api/data/flow${role >= 10 ? '' : '/self'}`;
      } else {
        return `/api/data${role >= 10 ? '' : '/self'}`;
      }
    }
  })();
  const state = useResource(() => read(endpoint, {
    start_timestamp: timestamp(range.start),
    end_timestamp: timestamp(range.end) + 86399
  }), [endpoint, range]);
  const rows = asRows(state.data);
  const totals = rows.reduce<{
    quota: number;
    requests: number;
    tokens: number;
  }>((total, row) => ({
    quota: total.quota + Number(row.quota || 0),
    requests: total.requests + Number(row.count ?? row.request_count ?? row.requests ?? 0),
    tokens: total.tokens + Number(row.token_used ?? row.tokens ?? 0)
  }), {
    quota: 0,
    requests: 0,
    tokens: 0
  });
  const groups = new Map<string, number>();
  rows.forEach(row => {
    const name = String(section === 'users' ? row.username || '未记录用户' : row.model_name || row.channel_name || row.model || '未记录模型');
    groups.set(name, (groups.get(name) || 0) + Number(row.quota || 0));
  });
  const sorted = [...groups].sort((a, b) => b[1] - a[1]);
  const max = sorted[0]?.[1] || 1;
  const title = (() => {
    if (section === 'users') {
      return '用户调用分析';
    } else {
      if (section === 'flow') {
        return '流量分析';
      } else {
        return '模型分析';
      }
    }
  })();
  const unit = Number(window.FurtherQuotaPerUnit || 500000);
  return <Page title={title} description='按日期查看真实调用数据、费用及模型分布。' loading={state.loading} error={state.error} reload={state.reload}>
      <form className='rfw-toolbar' onSubmit={event => {
      event.preventDefault();
      if (start <= end) {setRange({
        start,
        end
      });}
    }} style={{
      marginBottom: 20
    }}>
        <label>
          开始{' '}
          <input aria-label='开始日期' type='date' value={start} onChange={event => setStart(event.target.value)} required />
        </label>
        <label>
          结束{' '}
          <input aria-label='结束日期' type='date' value={end} min={start} onChange={event => setEnd(event.target.value)} required />
        </label>
        <button type='submit' className='rfw-primary'>
          查询
        </button>
      </form>
      <div className='rfw-cards'>
        <article className='rfw-card'>
          <h2>消费</h2>
          <strong className='rfw-metric'>
            ${(totals.quota / unit).toFixed(6)}
          </strong>
        </article>
        <article className='rfw-card'>
          <h2>调用次数</h2>
          <strong className='rfw-metric'>
            {totals.requests.toLocaleString()}
          </strong>
        </article>
        <article className='rfw-card'>
          <h2>Token</h2>
          <strong className='rfw-metric'>
            {totals.tokens.toLocaleString()}
          </strong>
        </article>
      </div>
      <article className='rfw-card' style={{
      marginTop: 20
    }}>
        <h2>消费分布</h2>
        {sorted.length ? <div className='rfw-bars'>
            {sorted.map(([name, value]) => <div key={name} className='rfw-bar'>
                <span>{name}</span>
                <div className='rfw-bar-track'>
                  <i style={{
              width: `${value / max * 100}%`
            }} />
                </div>
                <strong>${(value / unit).toFixed(6)}</strong>
              </div>)}
          </div> : <Empty>此日期范围内没有调用记录。</Empty>}
      </article>
    </Page>;
}
const columns: Record<string, [string, string][]> = {
  common: [['created_at', '时间'], ['username', '用户'], ['model_name', '模型'], ['prompt_tokens', '输入'], ['completion_tokens', '输出'], ['quota', '额度'], ['content', '结果']],
  audit: [['created_at', '时间'], ['username', '用户'], ['category', '类别'], ['action', '操作'], ['success', '成功'], ['ip', 'IP']],
  drawing: [['submit_time', '时间'], ['user_id', '用户'], ['action', '任务'], ['status', '状态'], ['progress', '进度']],
  task: [['created_at', '时间'], ['user_id', '用户'], ['task_id', '任务'], ['platform', '平台'], ['status', '状态']]
};
export function LogsScreen({
  pathname
}: {
  pathname: string;
}) {
  const role = Number(useAuthStore(state => state.auth.user?.role));
  const section = pathname.split('/')[2] || 'common';
  const displayColumns = columns[section] || columns.common;
  const [page, setPage] = useState(1);
  const [keyword, setKeyword] = useState('');
  const [filter, setFilter] = useState('');
  const [detail, setDetail] = useState<Row | null>(null);
  const endpoint = (() => {
    if (section === 'audit') {
      return '/api/audit';
    } else {
      if (section === 'drawing') {
        return '/api/mj';
      } else {
        if (section === 'task') {
          return '/api/task';
        } else {
          return '/api/log';
        }
      }
    }
  })();
  const filterLabel = (() => {
    if (section === 'audit') {
      return '用户名';
    } else {
      if (section === 'drawing') {
        return '绘图任务编号';
      } else {
        if (section === 'task') {
          return '任务编号';
        } else {
          return '模型名称';
        }
      }
    }
  })();
  const parameters = (() => {
    if (section === 'drawing') {
      return {
        mj_id: filter
      };
    } else {
      if (section === 'task') {
        return {
          task_id: filter
        };
      } else {
        if (section === 'audit') {
          return {
            username: role >= 10 ? filter : undefined
          };
        } else {
          return {
            model_name: filter
          };
        }
      }
    }
  })();
  const state = useResource(() => read(endpoint + (role >= 10 ? '' : '/self'), {
    p: page,
    page_size: 25,
    ...parameters
  }), [endpoint, role, page, filter]);
  const data = pageData(state.data);
  return <Page title={(() => {
    if (section === 'audit') {
      return '敏感操作审计';
    } else {
      if (section === 'drawing') {
        return '绘图日志';
      } else {
        if (section === 'task') {
          return '任务日志';
        } else {
          return '调用日志';
        }
      }
    }
  })()} description='查看请求记录和对应详情。' loading={state.loading} error={state.error} reload={state.reload}>
      <form className='rfw-toolbar' style={{
      marginBottom: 18
    }} onSubmit={event => {
      event.preventDefault();
      setPage(1);
      setFilter(keyword.trim());
    }}>
        <input aria-label={filterLabel} placeholder={filterLabel} value={keyword} onChange={event => setKeyword(event.target.value)} />
        <button className='rfw-primary' type='submit'>
          查询
        </button>
        <button className='rfw-secondary' type='button' onClick={() => {
        setKeyword('');
        setFilter('');
        setPage(1);
      }}>
          清空筛选
        </button>
      </form>
      {data.items.length ? <div className='rfw-table-wrap'>
          <table className='rfw-table'>
            <thead>
              <tr>
                {displayColumns.map(([key, title]) => <th key={key}>{title}</th>)}
                <th>详情</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((row, index) => <tr key={String(row.id ?? row.event_id ?? index)}>
                  {displayColumns.map(([key]) => <td key={key}>
                      {(() => {
                if (key.endsWith('_at') || key.endsWith('_time')) {
                  if (row[key]) {
                    return new Date(Number(row[key]) * (Number(row[key]) > 1e12 ? 1 : 1000)).toLocaleString();
                  } else {
                    return '—';
                  }
                } else {
                  return label(row[key]);
                }
              })()}
                    </td>)}
                  <td>
                    <button type='button' className='rfw-secondary' onClick={() => setDetail(row)}>
                      查看
                    </button>
                  </td>
                </tr>)}
            </tbody>
          </table>
        </div> : <Empty>当前筛选下没有记录。</Empty>}
      <div className='rfw-pagination'>
        <button type='button' className='rfw-secondary' disabled={page <= 1} onClick={() => setPage(page - 1)}>
          上一页
        </button>
        <span>
          第 {page} 页 · 共 {data.total} 条
        </span>
        <button type='button' className='rfw-secondary' disabled={page * 25 >= data.total} onClick={() => setPage(page + 1)}>
          下一页
        </button>
      </div>
      {detail && <Modal title='记录详情' close={() => setDetail(null)}>
          <pre className='rfw-code'>
            {JSON.stringify({
          ...detail,
          other: typeof detail.other === 'string' ? (() => {
            try {
              return JSON.parse(detail.other) as unknown;
            } catch {
              return detail.other;
            }
          })() : asRow(detail.other)
        }, null, 2)}
          </pre>
        </Modal>}
    </Page>;
}
