import { appendEditableListItem, editableListEntries, removeEditableListItem, replaceEditableListItem } from "./editable-identity";
import { useState, type ReactNode } from 'react';
import { Empty } from './ui';
import { operationStatus, dateTime } from "./operation-values";

export function Status({
  value
}: {
  value: unknown;
}) {
  return <span className={`rfw-op-status rfw-op-status-${String(value || '').replaceAll(/[^a-z]/gi, '-')}`}>
      {operationStatus(value)}
    </span>;
}
export function Field({
  title,
  hint,
  children
}: {
  title: string;
  hint?: string;
  children: ReactNode;
}) {
  return <div className='rfw-field'>
      <span className='rfw-op-field-title'>{title}</span>
      <div className='rfw-op-control'>{children}</div>
      {hint && <small>{hint}</small>}
    </div>;
}
export function Details({
  items
}: {
  items: [string, ReactNode][];
}) {
  return <dl className='rfw-op-details'>
      {items.map(([name, value]) => <div key={name}>
          <dt>{name}</dt>
          <dd>{value ?? '—'}</dd>
        </div>)}
    </dl>;
}
const captions: Record<string, string> = {
  status: '状态',
  message: '说明',
  id: '标识',
  task_id: '任务编号',
  model: '模型',
  output: '输出',
  input: '输入',
  result: '结果',
  data: '内容',
  error: '错误',
  code: '代码',
  type: '类型',
  progress: '进度',
  total: '总数',
  processed: '已处理',
  deleted_count: '已删除条数',
  deleted: '已删除',
  failed: '失败',
  success: '成功',
  target_timestamp: '清理截止时间',
  checked: '已检查',
  completed: '已完成',
  skipped: '已跳过',
  duration_ms: '耗时（毫秒）',
  channels: '渠道',
  models: '模型',
  url: '地址',
  usage: '用量',
  state: '进度状态',
  payload: '任务参数',
  name: '名称',
  value: '内容',
  token: '令牌',
  count: '数量',
  total_channels: '渠道总数',
  completed_channels: '已完成渠道',
  error_count: '错误次数',
  phase: '当前步骤',
  last_id: '处理位置'
};
export function ReadableValue({
  value,
  depth = 0
}: {
  value: unknown;
  depth?: number;
}) {
  if (value === undefined || value === null) {
    return <span className='rfw-op-muted'>无内容</span>;
  }
  if (typeof value !== 'object') {
    return <span className='rfw-op-value'>
        {(() => {
        if (typeof value === 'boolean') {
          if (value) {
            return '是';
          } else {
            return '否';
          }
        } else {
          return String(value);
        }
      })()}
      </span>;
  }
  const entries = Object.entries(value);
  if (!entries.length) return <Empty>没有附加信息。</Empty>;
  if (depth > 12) return <span>内容层级过深，请使用接口调试工具查看。</span>;
  return <dl className='rfw-op-details rfw-op-nested'>
      {entries.map(([key, entry]) => <div key={key}>
          <dt>
            {Array.isArray(value) ? `第 ${Number(key) + 1} 项` : captions[key] || key.replaceAll('_', ' ')}
          </dt>
          <dd>
            {(() => {
          if (key === 'status') {
            return <Status value={entry} />;
          } else {
            if (key.endsWith('_timestamp')) {
              return dateTime(entry);
            } else {
              return <ReadableValue value={entry} depth={depth + 1} />;
            }
          }
        })()}
          </dd>
        </div>)}
    </dl>;
}
export type ArgumentValue = string | number | boolean | null | ArgumentValue[] | {
  [name: string]: ArgumentValue;
};
function valueKind(value: ArgumentValue) {
  return (() => {
    if (value === null) {
      return 'null';
    } else {
      if (Array.isArray(value)) {
        return 'array';
      } else {
        return typeof value;
      }
    }
  })();
}
export function ArgumentEditor({
  value,
  onChange,
  name = '参数',
  fixedType = false
}: {
  value: ArgumentValue;
  onChange: (value: ArgumentValue) => void;
  name?: string;
  fixedType?: boolean;
}) {
  const kind = valueKind(value);
  const [newKey, setNewKey] = useState('');
  const [error, setError] = useState('');
  return <div className='rfw-op-argument'>
      <div className='rfw-op-argument-head'>
        <span>{name}</span>
        {!fixedType && <select aria-label={`${name}的数据类型`} value={kind} onChange={event => onChange(({
        string: '',
        number: 0,
        boolean: false,
        null: null,
        array: [],
        object: {}
      } as Record<string, ArgumentValue>)[event.target.value])}>
            <option value='string'>文本</option>
            <option value='number'>数字</option>
            <option value='boolean'>是 / 否</option>
            <option value='object'>字段集合</option>
            <option value='array'>列表</option>
            <option value='null'>空值</option>
          </select>}
      </div>
      {(() => {
      if (kind === 'string') {
        return <textarea aria-label={`${name}的内容`} value={String(value)} rows={2} onChange={event => onChange(event.target.value)} />;
      } else {
        if (kind === 'number') {
          return <input aria-label={`${name}的数值`} type='number' step='any' value={Number(value)} onChange={event => onChange(Number(event.target.value))} />;
        } else {
          if (kind === 'boolean') {
            return <select aria-label={`${name}的选项`} value={value ? 'true' : 'false'} onChange={event => onChange(event.target.value === 'true')}>
          <option value='true'>是</option>
          <option value='false'>否</option>
        </select>;
          } else {
            if (kind === 'array') {
              return <>
          {editableListEntries(value as ArgumentValue[]).map((item, index) => <div key={item.id} className='rfw-op-argument-item'>
              <ArgumentEditor name={`参数 ${index + 1}`} value={item.value} onChange={next => onChange(replaceEditableListItem(value as ArgumentValue[], item.id, next))} />
              <button type='button' className='rfw-danger' onClick={() => onChange(removeEditableListItem(value as ArgumentValue[], item.id))}>
                移除此项
              </button>
            </div>)}
          <button type='button' className='rfw-secondary' onClick={() => onChange(appendEditableListItem(value as ArgumentValue[], ''))}>
            添加一项
          </button>
        </>;
            } else {
              if (kind === 'object') {
                return <>
          {Object.entries(value as Record<string, ArgumentValue>).map(([key, item]) => <div className='rfw-op-argument-item' key={key}>
                <ArgumentEditor name={key} value={item} onChange={next => onChange({
                      ...(value as object),
                      [key]: next
                    })} />
                <button type='button' className='rfw-danger' onClick={() => onChange(Object.fromEntries(Object.entries(value as object).filter(([old]) => old !== key)))}>
                  移除字段
                </button>
              </div>)}
          <div className='rfw-toolbar'>
            <input aria-label='新字段名称' placeholder='新字段名称' value={newKey} onChange={event => setNewKey(event.target.value)} />
            <button type='button' className='rfw-secondary' onClick={() => {
                      const key = newKey.trim();
                      if (!key || Object.hasOwn(value as object, key)) {
                        setError('请输入不重复的字段名称。');
                        return;
                      }
                      onChange({
                        ...(value as object),
                        [key]: ''
                      });
                      setNewKey('');
                      setError('');
                    }}>
              添加字段
            </button>
          </div>
          {error && <p role='alert'>{error}</p>}
        </>;
              } else {
                return <p className='rfw-op-muted'>此参数将作为空值提交。</p>;
              }
            }
          }
        }
      }
    })()}
    </div>;
}
