import { editableIdentity, replaceEditable } from "./editable-identity";
import { useState } from 'react';
import { toast } from 'sonner';
import { asRow, asRows, label, read, write, type Row } from './api';
import { ArgumentEditor, Details, Field, ReadableValue, Status, type ArgumentValue } from './operation-ui';
import { dateTime } from "./operation-values";
import { Empty, Modal, Notice, Page } from './ui';
import { message, useResource } from "./workspace-resource";

function localized(value: unknown) {
  if (typeof value === 'string') return value;
  const data = asRow(value);
  return String(data['zh-CN'] ?? data.zh ?? data.en ?? '');
}
function pluginKey(row: Row) {
  return String(asRow(row.meta).key ?? row.key ?? '');
}
function pluginPath(row: Row) {
  return `/api/plugin/task/${encodeURIComponent(pluginKey(row))}`;
}
function PluginUpload({
  close,
  saved
}: {
  close: () => void;
  saved: () => void;
}) {
  const [source, setSource] = useState('');
  const [remark, setRemark] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [completed, setCompleted] = useState(false);
  const upload = async () => {
    if (busy || completed) return;
    setBusy(true);
    setError('');
    try {
      if (!source.trim()) throw new Error('请选择或粘贴插件源码。');
      if (new Blob([source]).size > 8 * 1024 * 1024) {
        throw new Error('插件源码不能超过 8 MB。');
      }
      await write('/api/plugin/task', 'POST', {
        source,
        remark: remark.trim(),
        enabled
      });
      toast.success('插件已上传并通过编译。');
      setCompleted(true);
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  return <Modal title='上传任务插件' close={completed ? saved : close} busy={busy} dismiss={completed}>
      <p>
        上传 JavaScript
        插件后，服务器会校验插件声明并编译。相同插件的不同版本会保留为版本记录。
      </p>
      <Notice error={error} success={completed ? '插件已上传并通过编译。' : ''} />
      <form onSubmit={event => {
      event.preventDefault();
      void upload();
    }}>
        <fieldset className='rfw-operation-form-content' disabled={busy || completed}>
          <Field title='选择源码文件'>
            <input aria-label='选择源码文件' type='file' accept='.js,.mjs,text/javascript,application/javascript' onChange={async event => {
            const file = event.target.files?.[0];
            if (!file) return;
            if (file.size > 8 * 1024 * 1024) {
              setError('插件源码不能超过 8 MB。');
              return;
            }
            try {
              setSource(await file.text());
              setError('');
            } catch {
              setError('无法读取文件，请重新选择。');
            }
          }} />
          </Field>
          <Field title='插件源码'>
            <textarea aria-label='插件源码' className='rfw-op-source' rows={15} spellCheck={false} value={source} onChange={event => setSource(event.target.value)} required disabled={busy} />
          </Field>
          <Field title='版本备注'>
            <input aria-label='版本备注' value={remark} onChange={event => setRemark(event.target.value)} placeholder='例如：修复任务轮询与计费解析' />
          </Field>
          <label className='rfw-switch'>
            <input type='checkbox' checked={enabled} onChange={event => setEnabled(event.target.checked)} />
            上传后立即启用
          </label>
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
              取消
            </button>
            <button type='submit' className='rfw-primary' disabled={busy}>
              {busy ? '校验并上传中…' : '校验并上传'}
            </button>
          </div>
        </fieldset>
      </form>
    </Modal>;
}
function PluginDetails({
  row,
  close
}: {
  row: Row;
  close: () => void;
}) {
  const state = useResource(() => read(pluginPath(row)));
  const data = asRow(state.data);
  const meta = asRow(data.meta);
  const author = asRow(meta.author);
  return <Modal title='插件详情' close={close}>
      <Notice error={state.error} />
      {state.loading && !state.data ? <div className='rfw-loading'>正在读取插件…</div> : <>
          <h3>{localized(meta.name) || pluginKey(row)}</h3>
          <p>{localized(meta.description) || '此插件尚未填写说明。'}</p>
          <Details items={[['插件标识', label(meta.key)], ['当前版本', label(meta.version)], ['接口规范版本', label(meta.apiVersion)], ['作者', label(author.name || meta.author)], ['来源', data.layer === 'factory' ? '内置插件' : '上传的插件版本'], ['关联渠道', label(row.channel_count)], ['进行中的任务', label(row.in_flight_count)]]} />
          <h3>支持的模型</h3>
          <div className='rfw-op-tags'>
            {Array.isArray(meta.models) ? meta.models.map(model => <span key={String(model)}>{String(model)}</span>) : '未声明模型'}
          </div>
          <h3>可访问的上游主机</h3>
          <div className='rfw-op-tags'>
            {Array.isArray(meta.allowedHosts) ? meta.allowedHosts.map(host => <span key={String(host)}>{String(host)}</span>) : '未声明主机'}
          </div>
          <h3>公开路由</h3>
          {asRows(meta.routes).length ? <Details items={asRows(meta.routes).map((route, index) => [`路由 ${index + 1}`, `${label(route.method)} ${label(route.path)}`])} /> : <p>此插件没有声明公开路由。</p>}
          <details className='rfw-op-advanced'>
            <summary>查看已安装源码</summary>
            <pre className='rfw-code rfw-op-source'>
              {String(data.source || '')}
            </pre>
          </details>
        </>}
    </Modal>;
}
function PluginVersions({
  row,
  close,
  changed
}: {
  row: Row;
  close: () => void;
  changed: () => void;
}) {
  const state = useResource(() => read(`${pluginPath(row)}/versions`));
  const versions = asRows(state.data);
  const [pending, setPending] = useState<{
    row: Row;
    mode: 'activate' | 'delete';
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const commit = async () => {
    if (!pending) return;
    setBusy(true);
    setError('');
    try {
      await write(pluginPath(row) + (pending.mode === 'activate' ? '/activate' : '/versions/' + encodeURIComponent(String(pending.row.version))), pending.mode === 'activate' ? 'POST' : 'DELETE', pending.mode === 'activate' ? {
        version: String(pending.row.version)
      } : undefined);
      toast.success(pending.mode === 'activate' ? '插件版本已切换。' : '插件版本已删除。');
      setPending(null);
      void state.reload();
      changed();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  return <Modal title={`版本管理 · ${pluginKey(row)}`} close={close} busy={busy}>
      <Notice error={error || state.error} />
      <p>切换后，新请求将使用选定版本。内置版本由应用提供，不能在此删除。</p>
      {(() => {
      if (pending) {
        return <div className='rfw-op-confirm'>
          <h3>
            {pending.mode === 'activate' ? '切换到' : '删除'}版本{' '}
            {label(pending.row.version)}
          </h3>
          <p>
            {pending.mode === 'activate' ? '服务器会重新编译并发布此版本。' : '删除无法恢复；仍被渠道或任务使用的版本会由服务器阻止删除。'}
          </p>
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' disabled={busy} onClick={() => setPending(null)}>
              返回版本列表
            </button>
            <button type='button' className={pending.mode === 'activate' ? 'rfw-primary' : 'rfw-danger'} disabled={busy} onClick={() => void commit()}>
              {busy ? '正在处理…' : '确认操作'}
            </button>
          </div>
        </div>;
      } else {
        if (state.loading && !state.data) {
          return <div className='rfw-loading'>正在载入版本…</div>;
        } else {
          if (versions.length) {
            return <div className='rfw-op-stack'>
          {versions.map(version => <article className='rfw-card' key={String(version.id || version.version)}>
              <div className='rfw-op-card-head'>
                <h3>{label(version.version)}</h3>
                {Boolean(version.active) && <Status value='active' />}
              </div>
              <p>{label(version.remark)}</p>
              <Details items={[['上传时间', dateTime(version.created_at)], ['启用状态', version.enabled ? '启用' : '停用']]} />
              <div className='rfw-toolbar'>
                <button type='button' className='rfw-secondary' disabled={Boolean(version.active)} onClick={() => setPending({
                    row: version,
                    mode: 'activate'
                  })}>
                  使用此版本
                </button>
                <button type='button' className='rfw-danger' onClick={() => setPending({
                    row: version,
                    mode: 'delete'
                  })}>
                  删除版本
                </button>
              </div>
            </article>)}
        </div>;
          } else {
            return <Empty>没有上传版本；当前使用内置插件。</Empty>;
          }
        }
      }
    })()}
    </Modal>;
}
function PluginDryRun({
  row,
  close
}: {
  row: Row;
  close: () => void;
}) {
  const [hook, setHook] = useState('decode');
  const [member, setMember] = useState('');
  const [args, setArgs] = useState<ArgumentValue[]>([]);
  const [result, setResult] = useState<unknown>();
  const [hasResult, setHasResult] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  return <Modal title={`验证插件请求 · ${pluginKey(row)}`} close={close} busy={busy}>
      <p>
        选择要调用的插件函数，逐项填写参数，结果会按字段展示。请按插件函数要求设置参数顺序与类型。
      </p>
      <Notice error={error} />
      <form onSubmit={async event => {
      event.preventDefault();
      setBusy(true);
      setError('');
      setHasResult(false);
      try {
        setResult(await write(`${pluginPath(row)}/dryrun`, 'POST', {
          hook: hook.trim(),
          member: member.trim(),
          args
        }));
        setHasResult(true);
      } catch (cause) {
        setError(message(cause));
      } finally {
        setBusy(false);
      }
    }}>
        <div className='rfw-settings-grid'>
          <Field title='函数名称' hint='例如 decode；对象方法可在右侧补充成员名称。'>
            <input aria-label='函数名称' value={hook} onChange={event => setHook(event.target.value)} required />
          </Field>
          <Field title='成员名称（可选）'>
            <input aria-label='成员名称（可选）' value={member} onChange={event => setMember(event.target.value)} />
          </Field>
        </div>
        <ArgumentEditor name='调用参数（按顺序）' fixedType value={args} onChange={value => {
        if (Array.isArray(value)) setArgs(value);
      }} />
        <div className='rfw-actions'>
          <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
            关闭
          </button>
          <button type='submit' className='rfw-primary' disabled={busy}>
            {busy ? '正在运行…' : '运行验证'}
          </button>
        </div>
      </form>
      {hasResult && <section className='rfw-op-result'>
          <h3>验证结果</h3>
          <ReadableValue value={result} />
        </section>}
    </Modal>;
}
function PluginSources({
  close
}: {
  close: () => void;
}) {
  const [sources, setSources] = useState<{
    name: string;
    index_url: string;
  }[]>([]);
  const state = useResource(async () => {
    const rows = asRows(await read('/api/plugin/task/marketplace/sources'));
    setSources(rows.map(row => ({
      name: String(row.name || ''),
      index_url: String(row.index_url || '')
    })));
    return rows;
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  return <Modal title='插件源管理' close={close} busy={busy}>
      <p>
        管理插件目录的名称与索引地址。这里保存目录配置，不会自动下载或安装插件。
      </p>
      <Notice error={error || state.error} success={success} />
      {(() => {
      if (state.loading && !state.data) {
        return <div className='rfw-loading' role='status'>
          正在载入插件源…
        </div>;
      } else {
        if (!state.data) {
          return <div className='rfw-empty'>
          <p>插件源尚未读取成功，原有配置已保留。</p>
          <button type='button' className='rfw-secondary' onClick={() => void state.reload()}>
            重新读取插件源
          </button>
        </div>;
        } else {
          return <form onSubmit={async event => {
            event.preventDefault();
            if (busy || !state.data || state.error || state.loading) return;
            setBusy(true);
            setError('');
            setSuccess('');
            try {
              await write('/api/plugin/task/marketplace/sources', 'PUT', sources.map(source => ({
                name: source.name.trim(),
                index_url: source.index_url.trim()
              })));
              setSuccess('插件源配置已保存。');
              toast.success('插件源配置已保存。');
            } catch (cause) {
              setError(message(cause));
            } finally {
              setBusy(false);
            }
          }}>
          <fieldset className='rfw-operation-form-content' disabled={busy}>
            <div className='rfw-op-stack'>
              {sources.map((source, index) => <div className='rfw-card' key={editableIdentity(source)}>
                  <Field title={`插件源 ${index + 1} 名称`}>
                    <input aria-label={`插件源 ${index + 1} 名称`} value={source.name} required onChange={event => setSources(sources.map((old, i) => i === index ? replaceEditable(old, {
                      name: event.target.value
                    }) : old))} />
                  </Field>
                  <Field title='索引地址'>
                    <input aria-label={`插件源 ${index + 1} 索引地址`} type='url' pattern='https?://.+' value={source.index_url} required placeholder='https://example.com/plugins/index.json' onChange={event => setSources(sources.map((old, i) => i === index ? replaceEditable(old, {
                      index_url: event.target.value
                    }) : old))} />
                  </Field>
                  <button type='button' className='rfw-danger' onClick={() => setSources(sources.filter((_, i) => i !== index))}>
                    移除此源
                  </button>
                </div>)}
            </div>
            <div className='rfw-toolbar'>
              <button type='button' className='rfw-secondary' onClick={() => setSources([...sources, {
                  name: '',
                  index_url: ''
                }])}>
                添加插件源
              </button>
            </div>
            <div className='rfw-actions'>
              <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
                关闭
              </button>
              <button type='submit' className='rfw-primary' disabled={busy}>
                {busy ? '保存中…' : '保存插件源'}
              </button>
            </div>
          </fieldset>
        </form>;
        }
      }
    })()}
    </Modal>;
}
export function PluginOperations() {
  const state = useResource(() => read('/api/plugin/task'));
  const rows = asRows(state.data);
  const [view, setView] = useState<{
    type: 'details' | 'versions' | 'dryrun';
    row: Row;
  } | null>(null);
  const [upload, setUpload] = useState(false);
  const [sources, setSources] = useState(false);
  const [toggle, setToggle] = useState<Row | null>(null);
  const [dismissToggle, setDismissToggle] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const filtered = rows.filter(row => `${localized(asRow(row.meta).name)} ${pluginKey(row)}`.toLowerCase().includes(search.toLowerCase()));
  return <Page title='任务插件' description='管理任务接入能力、插件版本和运行验证。' loading={state.loading} error={state.error} reload={state.reload} actions={<button type='button' className='rfw-primary' onClick={() => setUpload(true)}>
          上传插件
        </button>}>
      <div className='rfw-toolbar rfw-op-main-toolbar'>
        <input aria-label='搜索任务插件' placeholder='按插件名称或标识搜索' value={search} onChange={event => setSearch(event.target.value)} />
        <span>
          {rows.length} 个插件 · {rows.filter(row => row.enabled).length}{' '}
          个已启用
        </span>
        <button type='button' className='rfw-secondary' onClick={() => setSources(true)}>
          管理插件源
        </button>
      </div>
      {filtered.length ? <div className='rfw-cards'>
          {filtered.map(row => {
        const meta = asRow(row.meta);
        return <article className='rfw-card' key={pluginKey(row)}>
                <div className='rfw-op-card-head'>
                  <h2>{localized(meta.name) || pluginKey(row)}</h2>
                  <Status value={row.enabled ? row.runtime_status : 'disabled'} />
                </div>
                <p className='rfw-op-id'>
                  {pluginKey(row)} · {label(meta.version)}
                </p>
                <p>
                  {localized(meta.description) || '此插件为网关提供任务处理能力。'}
                </p>
                <Notice error={row.runtime_error ? String(row.runtime_error) : ''} />
                <Details items={[['来源', (() => {
            if (row.source === 'factory') {
              return '内置插件';
            } else {
              if (row.source === 'override_over_factory') {
                return '自定义版本（覆盖内置）';
              } else {
                return '上传的插件';
              }
            }
          })()], ['关联渠道', label(row.channel_count)], ['进行中的任务', label(row.in_flight_count)]]} />
                <div className='rfw-toolbar'>
                  <button type='button' className='rfw-secondary' onClick={() => setView({
              type: 'details',
              row
            })}>
                    详情与源码
                  </button>
                  <button type='button' className='rfw-secondary' onClick={() => setView({
              type: 'versions',
              row
            })}>
                    版本管理
                  </button>
                  <button type='button' className='rfw-secondary' onClick={() => setView({
              type: 'dryrun',
              row
            })}>
                    请求验证
                  </button>
                  <button type='button' className={row.enabled ? 'rfw-danger' : 'rfw-primary'} onClick={() => {
              setError('');
              setDismissToggle(false);
              setToggle(row);
            }}>
                    {row.enabled ? '停用插件' : '启用插件'}
                  </button>
                </div>
              </article>;
      })}
        </div> : <Empty>
          {rows.length ? '没有匹配的插件。' : '还没有任务插件，可上传源码安装。'}
        </Empty>}
      {upload && <PluginUpload close={() => setUpload(false)} saved={() => {
      setUpload(false);
      void state.reload();
    }} />}
      {sources && <PluginSources close={() => setSources(false)} />}
      {view?.type === 'details' && <PluginDetails row={view.row} close={() => setView(null)} />}
      {view?.type === 'versions' && <PluginVersions row={view.row} close={() => setView(null)} changed={() => void state.reload()} />}
      {view?.type === 'dryrun' && <PluginDryRun row={view.row} close={() => setView(null)} />}
      {toggle && <Modal title={toggle.enabled ? '停用任务插件' : '启用任务插件'} close={() => {
      setToggle(null);
      setDismissToggle(false);
    }} busy={busy} dismiss={dismissToggle}>
          <Notice error={error} />
          <p>
            {toggle.enabled ? `将停用 ${pluginKey(toggle)} 的当前插件版本。关联渠道 ${label(toggle.channel_count)} 个，进行中的任务 ${label(toggle.in_flight_count)} 个。` : `将启用 ${pluginKey(toggle)}，服务器会校验并加载插件。`}
          </p>
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' data-modal-close disabled={busy}>
              取消
            </button>
            <button type='button' className={toggle.enabled ? 'rfw-danger' : 'rfw-primary'} disabled={busy} onClick={async () => {
          setBusy(true);
          setError('');
          try {
            await write(`${pluginPath(toggle)}/status`, 'POST', {
              enabled: !toggle.enabled
            });
            toast.success(toggle.enabled ? '插件已停用。' : '插件已启用。');
            setDismissToggle(true);
            void state.reload();
          } catch (cause) {
            setError(message(cause));
          } finally {
            setBusy(false);
          }
        }}>
              {busy ? '正在处理…' : '确认'}
            </button>
          </div>
        </Modal>}
    </Page>;
}
