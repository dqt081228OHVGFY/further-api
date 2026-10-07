import type { Row } from "./api";

export type RecordField = {
  key: string;
  title: string;
  type?: 'long';
  required?: boolean;
};
export type Resource = {
  title: string;
  endpoint: string;
  fields: RecordField[];
  columns: [string, string][];
  seed: Row;
};
const f = (key: string, title: string, type?: RecordField['type'], required?: boolean): RecordField => ({
  key,
  title,
  type,
  required
});
export const resources: Record<string, Resource> = {
  '/models/vendors': {
    title: '模型厂商',
    endpoint: '/api/vendors/',
    columns: [['name', '厂商'], ['description', '说明'], ['model_count', '关联模型']],
    seed: {},
    fields: [f('name', '厂商名称', undefined, true), f('description', '说明', 'long'), f('icon', '图标')]
  },
};
export const isRecordsPath = (path: string) => Boolean(resources[path]);
