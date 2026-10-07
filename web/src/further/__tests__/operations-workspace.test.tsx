import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { MonitorOperations } from '../feature-workspace/monitor-operations';
import { PluginOperations } from '../feature-workspace/plugin-operations';
import { ArgumentEditor, type ArgumentValue } from '../feature-workspace/operation-ui';

const request = vi.hoisted(() => ({
  read: vi.fn(),
  write: vi.fn(),
  toast: vi.fn()
}));
vi.mock('@/lib/api', () => ({
  api: {}
}));
vi.mock('../feature-workspace/api', async original => ({
  ...(await original<typeof import('../feature-workspace/api')>()),
  read: request.read,
  write: request.write
}));
vi.mock('sonner', () => ({
  toast: {
    success: request.toast,
    error: request.toast
  }
}));

const plugin = {
  meta: {
    key: 'task/demo',
    name: '演示任务',
    version: '1.0'
  },
  source: 'override',
  enabled: true,
  runtime_status: 'registered',
  channel_count: 0,
  in_flight_count: 0
};
beforeEach(() => {
  vi.resetAllMocks();
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
    configurable: true,
    value() {
      this.setAttribute('open', '');
    }
  });
  Object.defineProperty(HTMLDialogElement.prototype, 'close', {
    configurable: true,
    value() {
      this.removeAttribute('open');
    }
  });
  request.write.mockResolvedValue(undefined);
  request.read.mockImplementation(async (path: string) => {
    if (path === '/api/plugin/task') return [plugin];
    if (path === '/api/plugin/task/task%2Fdemo/versions') {
      return [{
        id: 1,
        version: '1.0',
        active: true
      }, {
        id: 2,
        version: '2.0',
        active: false
      }];
    }
    if (path === '/api/system-info/instances') {
      return [{
        node_name: 'gateway-a',
        status: 'online',
        info: {
          runtime: {
            version: '1.0',
            goos: 'linux'
          },
          resources: {
            cpu: {
              usage_percent: 23
            },
            memory: {
              usage_percent: 48
            },
            storage: {
              used_percent: 34
            }
          }
        }
      }];
    }
    throw new Error(`Unexpected request ${path}`);
  });
});
describe('localized operations workspace', () => {
  it('preserves a later argument draft and focus when an earlier list item is removed', () => {
    function Editor() {
      const [value, setValue] = useState<ArgumentValue>(['first', {}]);
      return <ArgumentEditor value={value} onChange={setValue} />;
    }
    render(<Editor />);
    const field = screen.getByLabelText('新字段名称');
    fireEvent.change(field, { target: { value: 'unfinished-field' } });
    field.focus();
    fireEvent.click(screen.getAllByRole('button', { name: '移除此项' })[0]);
    expect(screen.getByLabelText('新字段名称')).toBe(field);
    expect(field).toHaveValue('unfinished-field');
    expect(field).toHaveFocus();
    fireEvent.click(screen.getByRole('button', { name: '添加字段' }));
    expect(screen.getByLabelText('unfinished-field的内容')).toBeVisible();
  });
  it('requires a successful plugin-source read before offering writes and lets the user retry safely', async () => {
    request.read.mockImplementation(async (path: string) => {
      if (path === '/api/plugin/task') return [plugin];
      if (path === '/api/plugin/task/marketplace/sources') {
        throw new Error('插件源读取暂时失败');
      }
      throw new Error(`Unexpected request ${path}`);
    });
    render(<PluginOperations />);
    fireEvent.click(await screen.findByRole('button', {
      name: '管理插件源'
    }));
    expect(await screen.findByRole('alert')).toHaveTextContent('插件源读取暂时失败');
    expect(screen.queryByRole('button', {
      name: '保存插件源'
    })).toBeNull();
    expect(request.write).not.toHaveBeenCalled();
    request.read.mockResolvedValue([{
      name: '现有插件源',
      index_url: 'https://plugins.example/index.json'
    }]);
    fireEvent.click(screen.getByRole('button', {
      name: '重新读取插件源'
    }));
    expect(await screen.findByDisplayValue('现有插件源')).toBeVisible();
    fireEvent.click(screen.getByRole('button', {
      name: '保存插件源'
    }));
    await waitFor(() => expect(request.write).toHaveBeenCalledExactlyOnceWith('/api/plugin/task/marketplace/sources', 'PUT', [{
      name: '现有插件源',
      index_url: 'https://plugins.example/index.json'
    }]));
  });
  it('renders instance details without issuing the nonexistent detail GET', async () => {
    render(<MonitorOperations />);
    fireEvent.click(await screen.findByRole('button', {
      name: '运行详情'
    }));
    const modal = screen.getByRole('dialog', {
      name: '实例详情 · gateway-a'
    });
    expect(within(modal).getByText('linux')).toBeVisible();
    expect(within(modal).getByRole('meter', {
      name: '处理器使用率'
    })).toHaveAttribute('value', '23');
    expect(request.read.mock.calls.map(([path]) => path)).toEqual(['/api/system-info/instances']);
  });
  it('switches to an installed plugin version after a visible confirmation', async () => {
    render(<PluginOperations />);
    fireEvent.click(await screen.findByRole('button', {
      name: '版本管理'
    }));
    const buttons = await screen.findAllByRole('button', {
      name: '使用此版本'
    });
    fireEvent.click(buttons.find(button => !button.hasAttribute('disabled'))!);
    expect(request.write).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', {
      name: '确认操作'
    }));
    await waitFor(() => expect(request.write).toHaveBeenCalledWith('/api/plugin/task/task%2Fdemo/activate', 'POST', {
      version: '2.0'
    }));
  });
  it('sends dry-run arguments from typed fields and renders the response as readable fields', async () => {
    request.write.mockResolvedValue({
      status: 'success',
      message: '已完成解析'
    });
    render(<PluginOperations />);
    fireEvent.click(await screen.findByRole('button', {
      name: '请求验证'
    }));
    fireEvent.click(screen.getByRole('button', {
      name: '添加一项'
    }));
    fireEvent.change(screen.getByLabelText('参数 1的内容'), {
      target: {
        value: 'sample-input'
      }
    });
    fireEvent.click(screen.getByRole('button', {
      name: '运行验证'
    }));
    expect(await screen.findByText('已完成解析')).toBeVisible();
    expect(request.write).toHaveBeenCalledWith('/api/plugin/task/task%2Fdemo/dryrun', 'POST', {
      hook: 'decode',
      member: '',
      args: ['sample-input']
    });
    expect(screen.queryByLabelText('配置（JSON）')).not.toBeInTheDocument();
  });
});
