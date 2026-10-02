import {readFile, writeFile} from 'node:fs/promises';
const edits = {
  'relayfor-ui/assets/appearance.js': [
    ['<p class="md-dialog-content">选择你喜欢的浅色配色，节日期间自动换上节日主题。</p>', ''],
  ],
  'relayfor-ui/assets/admin/admin-models.js': [
    ['在这里配置真实模型定价与订阅访问规则。渠道及上游模型别名在「高级路由」中统一管理。', '渠道和上游模型别名可在「高级路由」中设置。'],
  ],
  'relayfor-ui/assets/admin/admin-token-stats.js': [
    ['按用户与令牌汇总真实调用日志，可勾选多个用户的令牌并复制统计。', ''],
  ],
  'relayfor-ui/assets/admin/multi-user-stats.js': [
    ['从想了解的用户开始', '暂无查询结果'],
    ['填写用户名和日期，查询所有令牌的真实使用情况。', '请选择用户和日期。'],
  ],
  'relayfor-ui/assets/admin/admin.js': [
    ['统一兑换码管理', '兑换码'],
    ['。批次与明细在同一列表操作。', ''],
  ],
};
for (const [file, pairs] of Object.entries(edits)) {
  let source = await readFile(file, 'utf8');
  for (const [before, after] of pairs) source = source.replaceAll(before, after);
  await writeFile(file, source);
}
