# 独立前端审查（2026-10-01）

范围：只读检查 `relayfor-ui/`、根目录 `web/src/relayfor/` 与 `ai-hybgzs-rebuild/mirror/` 的相关页面；查看现有截图与 `workspace-smoke-report.json`。没有修改业务源码、数据库或服务，也没有把 3000/3002 的旧构建当作最新源码。以下仅列可由源码确认的问题。

## 优先处理

1. **[P0] 原生 New API 页面仍是大量功能的主体。** [`extensions.js`](../../relayfor-ui/assets/extensions.js) 第 7–29 行把管理员模型分析、模型资料、用户操作、订阅、任务日志，以及支付、计费、Passkey、SSRF 等设置映射到 New API 路由；第 44–59、125–177 行以 iframe 注入。用户控制台的 Key 扩展和用量分析也沿用此方式；[`console/index.html`](../../relayfor-ui/console/index.html) 第 302 行的安全中心亦是 iframe。当前方案与“以 RelayFor 自有页面为主，合并真实功能并去除原生 New API 页面”的新要求冲突。需要逐页迁移实际表单/API/权限，再删除对应 iframe 和原生页面入口，不能只隐藏 tab。
2. **[P1] VIP“更多支付方式”跳错页。** [`vip-workspace.js`](../../relayfor-ui/assets/vip-workspace.js) 第 90 行将 iframe 指向 `/wallet?relayfor_extension=1`，而 [`wallet/index.tsx`](../../web/src/routes/_authenticated/wallet/index.tsx) 第 21–24 行无条件重定向 `/dashboard`。按钮不会展示在线支付方式。应在自有 VIP 页面接入真实支付选择与订单状态，保持服务端支付合规校验。
3. **[P1] 管理员可见筛选项直接报“不支持”。** [`admin/index.html`](../../relayfor-ui/admin/index.html) 第 199–201 行显示审计员角色及“沉睡用户”筛选；[`admin-adapter.ts`](../../web/src/relayfor/admin-adapter.ts) 第 125–129、343–348 行拒绝这两种输入。应实现相应查询或从表单移除/禁用并说明。
4. **[P1] 模型弹窗暴露未接通的管理字段。** [`admin/index.html`](../../relayfor-ui/admin/index.html) 第 527–550 行允许编辑长上下文、data-train、会话限制、模型启停和渠道绑定；[`admin-adapter.ts`](../../web/src/relayfor/admin-adapter.ts) 第 632–689 行遇到多项非默认值或绑定变化即抛 `unsupported`。保存会失败。应将能映射的设置接入真实模型/渠道 API，其余在页面明确不可编辑。
5. **[P1] 渠道 Key“立即复查余额”不可用。** [`channel-keys.js`](../../relayfor-ui/assets/admin/channel-keys.js) 第 128–135 行发起单 Key 探测，但 [`admin-adapter.ts`](../../web/src/relayfor/admin-adapter.ts) 第 991–993 行直接拒绝；批量复查按钮调用的 `/api/admin/channel-keys/refresh-balances` 也没有对应路由。应接真实探测能力或移除这两个操作。

## 页面与体验

6. **[P2] 已领取福利缺少稳定入口。** [`console/index.html`](../../relayfor-ui/console/index.html) 第 23–32 行侧栏没有“我的福利”；唯一入口是第 80 行概览横幅，而 [`console.js`](../../relayfor-ui/assets/console.js) 第 468–486 行在无待领取福利且词元贷不可见时隐藏它。此时用户无法从界面查看已领取记录。新的“活动与签到”单页可承载待领/已领/签到入口。
7. **[P2] 活动和签到目前分散。** [`console/index.html`](../../relayfor-ui/console/index.html) 第 64、233–258 行分别在概览与福利中呈现“今日活动”和词元贷签到；管理员订阅/注册福利设置及 [`admin.js`](../../relayfor-ui/assets/admin/admin.js) 的活动逻辑也分散。新的单页需要保留真实活动与 `/api/user/checkin`、现有福利请求的状态/失败提示，再移除旧入口，避免两处同时提供可变更操作。
8. **[P2] “白色”配色实际被蓝色覆盖。** [`gateway-theme.css`](../../relayfor-ui/assets/gateway-theme.css) 第 2 行设置灰白主色；后加载的 [`material.css`](../../relayfor-ui/assets/material.css) 第 2–10 行用同等特异性改回深蓝。选择“白色”仅写 `data-relayfor-palette=white`，没有覆盖规则。浅蓝、浅黄也应各自有独立变量，按钮与卡片维持可读对比度。
9. **[P2] 两处动效设置不同步。** [`appearance.js`](../../relayfor-ui/assets/appearance.js) 第 42–46 行写入显式 `relayfor-motion=on/off`；[`console.js`](../../relayfor-ui/assets/console.js) 第 1346–1375 行只保存 `reducedMotion`；[`app.js`](../../relayfor-ui/assets/app.js) 第 44–45 行优先采用显式状态。因此点过顶栏动效按钮后，账户设置里的“减少动效”可能保存成功却不改变动画。
10. **[P2] 多用户统计导出可能与当前表单不符。** [`multi-user-stats.js`](../../relayfor-ui/assets/admin/multi-user-stats.js) 第 48–72 行保存上次查询快照，第 128–139 行始终导出该快照；用户修改用户名或日期后，导出按钮仍可点，文件反映旧条件。建议条件变化后禁用导出直到重新查询，或明确标注“导出上次查询”。
11. **[P2] 调用费用列表固定使用 500000 换算。** [`console.js`](../../relayfor-ui/assets/console.js) 第 903–905 行没有使用服务端提供的 `quota_per_unit`；[`bootstrap.ts`](../../web/src/relayfor/bootstrap.ts) 第 43 行已读取此值。站点管理员调整额度换算后，列表金额将与明细及账户余额不一致。
12. **[P2] 首页在账户查询失败时显示虚构登录状态。** [`app.js`](../../relayfor-ui/assets/app.js) 第 676–687 行对非 401/403 的 `/api/user/self` 错误填入 `{username:'已登录', role:1}`，随后渲染用户入口。网络失败应显示可重试状态，不能用合成用户代替真实会话结果。

## 需在最新构建中复核

- 当前 `web/dist` 缺少最新 VIP/多用户统计资源，Go 正式入口嵌入的是 `web/dist`。必须重新运行前端构建，再测页面；现有 3000/3002 截图不代表最新源码。
- 旧 `workspace-smoke-report.json` 的用户端检查遭遇 HTTP 429，未完成用户控制台和移动布局验收；不能据此声称这些页面通过。
- 管理员概览的 `admin-minute-grid` 在 390px 仍为三列（[`admin-workspace.css`](../../relayfor-ui/assets/admin-workspace.css) 第 2、43 行），长标签/金额有溢出风险；现有移动截图只覆盖渠道页，应在最新构建实际测概览。
- 已核对支付设置目录：`extensions.js` 会动态创建 `[data-settings-page="payment"]`；不能因为静态 HTML 中没有该按钮就判定 VIP“前往支付设置”失效。

静态校验：对 `relayfor-ui/assets` 下 22 个 JavaScript 文件执行 `node --check`，均无语法错误。未启动服务、发起支付或请求上游。
