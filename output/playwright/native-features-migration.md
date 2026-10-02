# RelayFor 同文档功能迁移 · 2026-10-01

根目录正式源码已把管理设置、模型扩展、日志扩展、账户安全等原有 iframe 承载改为直接挂载在 RelayFor 内容区。`gateway/` 旧副本没有参与修改。自有侧栏、顶栏、HTML 业务模块继续负责页面结构，复杂功能复用现有真实 React feature、路由权限和请求逻辑。

## 挂载接口

```js
const handle = await window.RelayForFeatures.mount(hostElement, { path: '/wallet' });
await handle.navigate('/security');
handle.destroy();
```

接口由 `web/src/relayfor/native-features.tsx` 安装到 `relayfor-bridge`。`entries` 提供按实际角色过滤的完整功能目录。路径规范函数拒绝跨域、协议地址、API 路径和编码路径逃逸。内部使用 memory router，外部旧页面路径由 `main.tsx` 与 `original-shell-target.ts` 导向自有 shell；已有原生页面优先使用 `#tokens`、`#profile`、`#channels` 等入口，更具体的功能使用 `#native/<编码路径>`。

## 已迁移的入口

| 自有入口 | 复用的真实功能 |
| --- | --- |
| 管理概览、用户分析 | Dashboard 各分析 section |
| 模型扩展 | 元数据、厂商、部署与相关操作 |
| 账户操作、订阅扩展 | 用户管理、订阅计划和成员操作 |
| 调用与任务日志 | 消费日志、绘图、任务、审计 |
| 系统设置、更多功能 | 站点、认证、安全、请求策略、模型、计费、内容、运维八组全部 section |
| 用户 Key、安全、付款 | API Key 扩展、密码/绑定/会话/Passkey/MFA、Wallet 支付与订单 |
| 更多功能 | 已恢复的任务插件、Playground 功能及内部路由 |

`relayfor-ui/assets/extensions.js` 与用户安全区域不再创建 iframe。VIP 可通过上述接口挂载 `/wallet`。钱包余额订阅支付传递用户确认时的 `expected_price_amount`、`expected_currency`；失败不会当作购买成功。签到展示归主任务的活动单页，账户扩展只保留前往活动与签到的入口。

## 样式与会话边界

- 同文档 ShadowRoot 隔离 legacy 全局 reset 与 Tailwind；弹窗、下拉、菜单和自定义组合框使用现有 PortalContainerContext，显式容器仍优先。
- 独立构建生成 `native-features-manifest.json`。普通功能 CSS 只进入 ShadowRoot；document.head 仅加载含 `@font-face`、`@property` 的 foundations 文件，静态测试禁止其中出现普通选择器。
- Theme/Font/Direction/ThemeCustomization 的 isolated 模式保留上下文，避免重写外层 shell；持续同步节日、配色、深浅主题和减少动效状态。
- 功能内 SearchProvider 不注册全局 Ctrl K；快捷键由自有页面搜索统一处理。
- 保留真实认证解析、route beforeLoad、RBAC、服务器鉴权及敏感操作验证。未完成认证不会创建功能 root，CSS/manifest 失败允许重试，未合成会话或成功结果。
- 本次前端鉴权接线参考 [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 与 [Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)；未据此宣称完整 ASVS 合规。

## 验证

- `npm run typecheck`：通过。
- `npm run build`：通过，Rsbuild 编译 36.6 秒，随后真实发布脚本生成自有页面及 native CSS/manifest。
- Portal ShadowRoot 测试：9 项通过。
- Provider 外层样式与 Ctrl K 隔离测试：2 项通过。
- 功能路径/目录与钱包报价测试：已通过，目录测试对照实际八组 section registry。
- native runtime 集成测试：通过，涵盖鉴权失败、网络/空 manifest、global/shadow/通知样式失败重试、同 host 重复导航、主题同步、销毁及重新挂载。
- `node --test scripts/native-features-css.test.mjs`：3 项通过。
- `node scripts/test-relayfor-extensions.mjs`：通过，同文档菜单、支付目录、用户权限过滤与无 iframe 断言通过。
- `extensions.js`、`console.js` 的 Node 语法检查通过。

主任务负责最后的 Go 内嵌重编译、启动与真实浏览器验收。本任务没有使用旧 3000/3002 构建宣称最新版视觉通过，也未发起真实付款或上游调用。原生登录加密/MFA 的追加修复及最终重建结果将补记于下方。
