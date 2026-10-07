import type { Field } from './catalog';

export function validateSetting(field: Field, value: string): string {
  if (field.kind === 'number' && (!value.trim() || !Number.isFinite(Number(value)))) {
    return `${field.label}必须是有效数字。`;
  }
  if (field.kind === 'number' && field.min !== undefined && Number(value) < field.min) {
    return `${field.label}不能小于 ${field.min}。`;
  }
  if (field.format === 'json' && value.trim()) {
    try {
      const parsed = JSON.parse(value);
      if (parsed === null || typeof parsed !== 'object') {
        return `${field.label}需要填写 JSON 对象或数组。`;
      }
      if (['fetch_setting.domain_list', 'fetch_setting.ip_list', 'fetch_setting.allowed_ports', 'global.thinking_model_blacklist'].includes(field.key) && (!Array.isArray(parsed) || !parsed.every(item => typeof item === 'string'))) {
        return `${field.label}需要填写字符串数组，例如 ["443"]。`;
      }
      if (field.key === 'claude.default_max_tokens' && (Array.isArray(parsed) || !Object.values(parsed).every(value => Number.isSafeInteger(value) && Number(value) >= 0))) {
        return `${field.label}需要填写模型名称到非负整数的 JSON 对象。`;
      }
    } catch {
      return `${field.label}的 JSON 格式不正确，请检查引号、逗号和括号。`;
    }
  }
  if (['claude.thinking_adapter_budget_tokens_percentage', 'gemini.thinking_adapter_budget_tokens_percentage'].includes(field.key) && Number(value) > 1) {
    return `${field.label}需要填写 0 到 1 的比例。`;
  }
  if (field.format === 'url' && value.trim()) {
    try {
      const url = new URL(value);
      if (!['https:', 'http:'].includes(url.protocol)) {
        return `${field.label}需要使用 http:// 或 https:// 地址。`;
      }
    } catch {
      return `${field.label}需要填写完整的网址。`;
    }
  }
  if (['AutomaticRetryStatusCodes', 'AutomaticDisableStatusCodes'].includes(field.key) && value.trim() && !value.split(',').every(code => /^\d{3}$/.test(code.trim()) && Number(code) >= 100 && Number(code) <= 599)) {
    return `${field.label}请用英文逗号分隔 100–599 之间的状态码。`;
  }
  return '';
}
