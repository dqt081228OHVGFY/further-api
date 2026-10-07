import { resources } from "./records-config";
import { useState } from 'react';
import { label, pageData, read, write, type Row } from './api';
import { Empty, Modal, Notice, Page } from './ui';
import { message, useResource } from "./workspace-resource";

export function RecordsScreen({
  pathname
}: {
  pathname: string;
}) {
  const resource = resources[pathname];
  const [page, setPage] = useState(1);
  const [filter, setFilter] = useState('');
  const state = useResource(() => read(resource.endpoint, {
    p: page,
    page_size: 25
  }), [resource.endpoint, page]);
  const data = pageData(state.data);
  const [editing, setEditing] = useState<Row | null>(null);
  const [draft, setDraft] = useState<Row>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [deleting, setDeleting] = useState<Row | null>(null);
  const [dismissEditing, setDismissEditing] = useState(false);
  const [dismissDeleting, setDismissDeleting] = useState(false);
  const open = (row: Row) => {
    setEditing(row);
    setDismissEditing(false);
    setDraft({
      ...row
    });
    setError('');
    setSuccess('');
  };
  const save = async () => {
    if (busy || dismissEditing) return;
    setBusy(true);
    setError('');
    try {
      const payload = {
        ...draft
      };
      for (const field of resource.fields) {
        if (field.required && !String(payload[field.key] ?? '').trim()) {
          throw new Error(`请填写${field.title}。`);
        }
      }
      await write(resource.endpoint, editing?.id ? 'PUT' : 'POST', payload);
      setDismissEditing(true);
      setSuccess('已保存');
      void state.reload();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    if (busy || dismissDeleting) return;
    setBusy(true);
    setError('');
    try {
      await write(resource.endpoint + encodeURIComponent(String(deleting?.id)), 'DELETE');
      setDismissDeleting(true);
      setSuccess('已删除');
      void state.reload();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setBusy(false);
    }
  };
  const allRows = data.items.filter(row => !filter || resource.columns.some(([key]) => label(row[key]).toLowerCase().includes(filter.toLowerCase())));
  const clientPages = Array.isArray(state.data);
  const total = clientPages ? allRows.length : data.total;
  const rows = clientPages ? allRows.slice((page - 1) * 25, page * 25) : allRows;
  return <Page title={resource.title} description='集中维护厂商信息，查看模型关联情况。' loading={state.loading} error={state.error} reload={state.reload} actions={<button type='button' className='rfw-primary' onClick={() => open(resource.seed)}>
          新建{resource.title}
        </button>}>
      <Notice error={!editing && !deleting ? error : ''} success={success} />
      <div className='rfw-toolbar rfw-record-toolbar'>
        <input aria-label={clientPages ? '筛选厂商' : '筛选当前页'} placeholder={clientPages ? '搜索厂商名称或说明' : '筛选当前页'} value={filter} onChange={event => {
        setFilter(event.target.value);
        if (clientPages) setPage(1);
      }} />
      </div>
      {rows.length ? <div className='rfw-record-grid'>
          {rows.map((row, index) => <article className='rfw-record-card' key={String(row.id ?? index)}>
              <header>
                <span className='rfw-record-number'>#{label(row.id)}</span>
              </header>
              <h2>{label(row[resource.columns[0][0]])}</h2>
              {row.description ? <p>{label(row.description)}</p> : <p>
                  尚未填写说明
                </p>}
              <dl>
                {resource.columns.slice(1).filter(([key]) => key !== 'description').map(([key, title]) => <div key={key}>
                      <dt>{title}</dt>
                      <dd>{label(row[key])}</dd>
                    </div>)}
              </dl>
              <footer>
                <button type='button' className='rfw-secondary' aria-label={`编辑${label(row[resource.columns[0][0]])}`} onClick={() => open(row)}>
                  编辑资料
                </button>
                <button type='button' className='rfw-danger' aria-label={`删除${label(row[resource.columns[0][0]])}`} onClick={() => {
            setDeleting(row);
            setError('');
          }}>
                  删除
                </button>
              </footer>
            </article>)}
        </div> : <Empty>当前筛选下没有记录，可新建或调整筛选。</Empty>}
      <div className='rfw-pagination'>
        <button type='button' className='rfw-secondary' disabled={page === 1} onClick={() => setPage(page - 1)}>
          上一页
        </button>
        <span>
          第 {page} 页 · 共 {total} 条
        </span>
        <button type='button' className='rfw-secondary' disabled={page * 25 >= total} onClick={() => setPage(page + 1)}>
          下一页
        </button>
      </div>
      {editing && <Modal busy={busy} dismiss={dismissEditing} title={editing.id ? `编辑${resource.title}` : `新建${resource.title}`} close={() => {
      setEditing(null);
      setDismissEditing(false);
    }}>
          <Notice error={error} />
          <form onSubmit={event => {
        event.preventDefault();
        void save();
      }}>
            <div className='rfw-settings-grid'>
              {resource.fields.map(field => <div className='rfw-field' key={field.key}>
                  <label htmlFor={`rfw-record-${field.key}`}>
                    {field.title}
                  </label>
                  {field.type === 'long' ? <textarea id={`rfw-record-${field.key}`} rows={4} value={String(draft[field.key] ?? '')} onChange={event => setDraft({
                    ...draft,
                    [field.key]: event.target.value
                  })} disabled={busy} /> : <input id={`rfw-record-${field.key}`} type='text' value={String(draft[field.key] ?? '')} onChange={event => setDraft({
                    ...draft,
                    [field.key]: event.target.value
                  })} required={field.required} disabled={busy} autoComplete='off' />}
                </div>)}
            </div>
            <div className='rfw-actions'>
              <button type='button' className='rfw-secondary' disabled={busy} data-modal-close>
                取消
              </button>
              <button type='submit' className='rfw-primary' disabled={busy}>
                {busy ? '保存中…' : '保存'}
              </button>
            </div>
          </form>
        </Modal>}
      {deleting && <Modal title='删除配置' busy={busy} dismiss={dismissDeleting} close={() => {
      setDeleting(null);
      setDismissDeleting(false);
    }}>
          <Notice error={error} />
          <p>
            确认删除「{label(deleting[resource.columns[0][0]])}
            」？此操作无法撤销。
          </p>
          <div className='rfw-actions'>
            <button type='button' className='rfw-secondary' disabled={busy} data-modal-close>
              取消
            </button>
            <button type='button' className='rfw-danger' disabled={busy} onClick={() => void remove()}>
              确认删除
            </button>
          </div>
        </Modal>}
    </Page>;
}
