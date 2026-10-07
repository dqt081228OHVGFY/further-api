import { useState } from 'react';
import { toast } from 'sonner';
import { asRow, asRows, label, read, write, type Row } from './api';
import { Details, ReadableValue, Status } from './operation-ui';
import { bytes, dateTime } from "./operation-values";
import { Empty, Modal, Notice, Page } from './ui';
import { message, useResource } from "./workspace-resource";

const taskTypes: Record<string, string> = {
  log_cleanup: '日志清理',
  channel_test: '渠道测试',
  model_update: '模型同步',
  midjourney_poll: '绘图任务轮询',
  async_task_poll: '异步任务轮询'
};
function ResourceMeter({
  name,
  value,
  description
}: {
  name: string;
  value: unknown;
  description?: string;
}) {
  const amount = Number(value);
  const available = value !== undefined && Number.isFinite(amount);
  return <div className='rfw-op-resource'>
      <div>
        <span>{name}</span>
        <strong>{available ? `${amount.toFixed(1)}%` : '未上报'}</strong>
      </div>
      {available && <meter min={0} max={100} value={Math.max(0, Math.min(100, amount))} aria-label={`${name}使用率`} />}
      {description && <small>{description}</small>}
    </div>;
}
function InstanceDetails({
  row,
  close
}: {
  row: Row;
  close: () => void;
}) {
  const info = asRow(row.info);
  const runtime = asRow(info.runtime);
  const resources = asRow(info.resources);
  const disk = asRow(resources.storage);
  return <Modal title={`实例详情 · ${label(row.node_name)}`} close={close}>
      <Details items={[['实例名称', label(row.node_name)], ['在线状态', <Status key='status' value={row.status} />], ['节点角色', asRow(info.role).is_master ? '主节点' : '工作节点'], ['主机名', label(asRow(info.host).hostname)], ['应用版本', label(runtime.version)], ['操作系统', label(runtime.goos)], ['处理器架构', label(runtime.goarch)], ['启动时间', dateTime(row.started_at ?? runtime.started_at)], ['最近心跳', dateTime(row.last_seen_at)], ['离线判定时间', `${label(row.stale_after_seconds)} 秒未上报`]]} />
      <div className='rfw-op-resource-grid'>
        <ResourceMeter name='处理器' value={asRow(resources.cpu).usage_percent} />
        <ResourceMeter name='内存' value={asRow(resources.memory).usage_percent} />
        <ResourceMeter name='磁盘' value={disk.used_percent} description={`${bytes(disk.used_bytes)} 已用 / ${bytes(disk.total_bytes)} 总量`} />
      </div>
      {Object.keys(asRow(info.extra)).length > 0 && <>
          <h3>附加运行信息</h3>
          <ReadableValue value={info.extra} />
        </>}
    </Modal>;
}
function TaskMonitor({
  mode,
  close
}: {
  mode: 'current' | 'history';
  close: () => void;
}) {
  const [type, setType] = useState(mode === 'current' ? 'log_cleanup' : '');
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const [detail, setDetail] = useState<Row | null>(null);
  const state = useResource(() => mode === 'current' ? read('/api/system-task/current', {
    type
  }) : read('/api/system-task/list', {
    offset: (page - 1) * 20,
    limit: 20,
    ...(type ? {
      type
    } : {}),
    ...(status ? {
      status
    } : {})
  }), [mode, type, status, page]);
  const rows = (() => {
    if (mode === 'current') {
      if (state.data) {
        return [asRow(state.data)];
      } else {
        return [];
      }
    } else {
      return asRows(state.data);
    }
  })();
  return <Modal title={mode === 'current' ? '运行中的维护任务' : '维护任务历史'} close={close}>
      <div className='rfw-toolbar rfw-op-main-toolbar'>
        <select aria-label='任务类型' value={type} onChange={event => {
        setType(event.target.value);
        setPage(1);
        setDetail(null);
      }}>
          {mode === 'history' && <option value=''>全部类型</option>}
          {Object.entries(taskTypes).map(([key, name]) => <option key={key} value={key}>
              {name}
            </option>)}
        </select>
        {mode === 'history' && <select aria-label='任务状态' value={status} onChange={event => {
        setStatus(event.target.value);
        setPage(1);
        setDetail(null);
      }}>
            <option value=''>全部状态</option>
            <option value='pending'>排队中</option>
            <option value='running'>运行中</option>
            <option value='succeeded'>已完成</option>
            <option value='failed'>失败</option>
          </select>}
        <button type='button' className='rfw-secondary' disabled={state.loading} onClick={() => void state.reload()}>
          刷新任务
        </button>
      </div>
      <Notice error={state.error} />
      {(() => {
      if (detail) {
        return <div>
          <button type='button' className='rfw-secondary' onClick={() => setDetail(null)}>
            返回任务列表
          </button>
          <h3>{taskTypes[String(detail.type)] || label(detail.type)}</h3>
          <Details items={[['任务编号', label(detail.task_id)], ['执行状态', <Status key='status' value={detail.status} />], ['执行节点', label(detail.locked_by)], ['创建时间', dateTime(detail.created_at)], ['最近更新', dateTime(detail.updated_at)]]} />
          <Notice error={detail.error ? String(detail.error) : ''} />
          <h3>处理进度</h3>
          <ReadableValue value={detail.state} />
          <h3>执行结果</h3>
          <ReadableValue value={detail.result} />
          <details className='rfw-op-advanced'>
            <summary>任务参数</summary>
            <ReadableValue value={detail.payload} />
          </details>
        </div>;
      } else {
        if (state.loading && !state.data) {
          return <div className='rfw-loading' role='status'>
          正在读取任务…
        </div>;
        } else {
          if (rows.length) {
            return <div className='rfw-op-stack'>
          {rows.map(task => <article className='rfw-card' key={String(task.task_id)}>
              <div className='rfw-op-card-head'>
                <h3>{taskTypes[String(task.type)] || label(task.type)}</h3>
                <Status value={task.status} />
              </div>
              <Details items={[['执行节点', label(task.locked_by)], ['最近更新', dateTime(task.updated_at)], ['任务编号', label(task.task_id)]]} />
              <Notice error={task.error ? String(task.error) : ''} />
              <button type='button' className='rfw-secondary' onClick={() => setDetail(task)}>
                查看处理进度与结果
              </button>
            </article>)}
        </div>;
          } else {
            return <Empty>
          {mode === 'current' ? '这个类型当前没有运行中的任务。' : '没有符合条件的任务记录。'}
        </Empty>;
          }
        }
      }
    })()}
      {mode === 'history' && !detail && <div className='rfw-pagination'>
          <button type='button' className='rfw-secondary' disabled={page === 1 || state.loading} onClick={() => setPage(page - 1)}>
            上一页
          </button>
          <span>第 {page} 页</span>
          <button type='button' className='rfw-secondary' disabled={rows.length < 20 || state.loading} onClick={() => setPage(page + 1)}>
            下一页
          </button>
        </div>}
    </Modal>;
}
export function MonitorOperations() {
  const state = useResource(() => read('/api/system-info/instances'));
  const rows = asRows(state.data);
  const [detail, setDetail] = useState<Row | null>(null);
  const [tasks, setTasks] = useState<'current' | 'history' | null>(null);
  const [remove, setRemove] = useState<Row | 'all' | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [dismissRemove, setDismissRemove] = useState(false);
  const online = rows.filter(row => row.status === 'online').length;
  const stale = rows.filter(row => row.status === 'stale').length;
  const confirmRemove = async () => {
    if (!remove) return;
    setBusy(true);
    setError('');
    try {
      const response = asRow(await write(remove === 'all' ? '/api/system-info/stale-instances' : `/api/system-info/instances/${encodeURIComponent(String(remove.node_name))}`, 'DELETE'));
      const message = `已移除 ${Number(response.deleted_count || 0)} 条离线实例记录。`;
      setSuccess(message);
      toast.success(message);
      setDismissRemove(true);
      void state.reload();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  return <Page title='运行监控' description='查看网关节点心跳、资源负载与维护任务执行情况。' loading={state.loading} error={state.error} reload={state.reload}>
      <Notice success={success} />
      <div className='rfw-op-summary'>
        <article>
          <span>已注册实例</span>
          <strong>{rows.length}</strong>
        </article>
        <article>
          <span>在线实例</span>
          <strong>{online}</strong>
        </article>
        <article>
          <span>离线实例</span>
          <strong>{stale}</strong>
        </article>
      </div>
      <div className='rfw-toolbar rfw-op-main-toolbar'>
        <button type='button' className='rfw-secondary' onClick={() => setTasks('current')}>
          运行中的任务
        </button>
        <button type='button' className='rfw-secondary' onClick={() => setTasks('history')}>
          任务历史
        </button>
        <button type='button' className='rfw-danger' disabled={!stale || busy} onClick={() => {
        setError('');
        setDismissRemove(false);
        setRemove('all');
      }}>
          清理离线实例
        </button>
      </div>
      {rows.length ? <div className='rfw-cards'>
          {rows.map(row => {
        const info = asRow(row.info);
        const resources = asRow(info.resources);
        return <article className='rfw-card' key={String(row.node_name)}>
                <div className='rfw-op-card-head'>
                  <h2>{label(row.node_name)}</h2>
                  <Status value={row.status} />
                </div>
                <p>
                  {asRow(info.role).is_master ? '主节点' : '工作节点'} ·{' '}
                  {label(asRow(info.runtime).version)}
                </p>
                <div className='rfw-op-resource-grid'>
                  <ResourceMeter name='处理器' value={asRow(resources.cpu).usage_percent} />
                  <ResourceMeter name='内存' value={asRow(resources.memory).usage_percent} />
                  <ResourceMeter name='磁盘' value={asRow(resources.storage).used_percent} />
                </div>
                <Details items={[['最近心跳', dateTime(row.last_seen_at)], ['启动时间', dateTime(row.started_at)]]} />
                <div className='rfw-toolbar'>
                  <button type='button' className='rfw-secondary' onClick={() => setDetail(row)}>
                    运行详情
                  </button>
                  {row.status === 'stale' && <button type='button' className='rfw-danger' onClick={() => {
              setError('');
              setDismissRemove(false);
              setRemove(row);
            }}>
                      移除离线记录
                    </button>}
                </div>
              </article>;
      })}
        </div> : <Empty>尚无实例上报。实例启动后会定期上报心跳与资源使用情况。</Empty>}
      {detail && <InstanceDetails row={detail} close={() => setDetail(null)} />}
      {tasks && <TaskMonitor mode={tasks} close={() => setTasks(null)} />}
      {remove && <Modal title='移除离线实例记录' close={() => {
      setRemove(null);
      setDismissRemove(false);
    }} busy={busy} dismiss={dismissRemove}>
          <Notice error={error} />
          <p>
            {remove === 'all' ? `将移除当前 ${stale} 个离线实例的心跳记录。` : `将移除「${label(remove.node_name)}」的离线心跳记录。`}
            这只会清理监控记录，服务器再次上报时会自动重新出现。
          </p>
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
              取消
            </button>
            <button type='button' className='rfw-danger' disabled={busy} onClick={() => void confirmRemove()}>
              {busy ? '清理中…' : '确认移除'}
            </button>
          </div>
        </Modal>}
    </Page>;
}
