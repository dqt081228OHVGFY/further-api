
const statuses: Record<string, string> = {
  online: '在线',
  stale: '已离线',
  running: '运行中',
  pending: '排队中',
  succeeded: '已完成',
  failed: '失败',
  completed: '已结束',
  destroyed: '已删除',
  'deployment requested': '正在部署',
  'termination requested': '正在停止',
  registered: '已就绪',
  compile_failed: '编译失败',
  disabled_fallback: '使用内置版本',
  disabled: '已停用',
  not_registered: '尚未注册',
  active: '生效中',
  stopped: '已停止',
  success: '成功',
  error: '错误',
  warning: '警告',
  info: '信息',
  debug: '调试'
};
export function operationStatus(value: unknown) {
  const raw = String(value || '');
  return statuses[raw.toLowerCase()] || raw || '待确认';
}
export function dateTime(value: unknown) {
  if (!value) return '尚无记录';
  const date = new Date(typeof value === 'number' ? value * 1000 : String(value));
  return Number.isNaN(date.getTime()) ? '时间未知' : date.toLocaleString('zh-CN');
}
export function bytes(value: unknown) {
  const amount = Number(value);
  if (!Number.isFinite(amount)) return '—';
  return amount >= 1073741824 ? `${(amount / 1073741824).toFixed(1)} GB` : `${(amount / 1048576).toFixed(1)} MB`;
}
