# further-api 工作区

此目录保存 further-api 的首页、登录、账户与管理员管理页面和额度卡页面。

- `assets/app.js`：导航、焦点管理、弹窗、通知和基本交互。
- `assets/console.js`：用户账户、API Key、用量和接入文档。
- `assets/admin/`：渠道、用户、订阅福利和站点管理。
- `assets/*-workspace.css`：桌面与手机的页面布局。
- `../web/src/further/feature-workspace/`：账户安全、模型厂商、充值支付、管理配置与运行监控等独立组件。

执行 `web/` 中的 `npm run build` 或 `bun run build`，会把这些页面和应用模块一起发布到 `web/dist/`。所有业务请求连接本仓库 Go 服务；构建不包含已撤下的旧路由树和页面组件。

页面显示品牌固定为 **further-api**。资源命名空间为 `/further-assets/`，业务扩展接口为 `/api/further/`。部署前必须重新构建前端和 Go 二进制，避免内嵌旧资源。

模型页集中管理价格、倍率、渠道路由与上游别名；参考价格可以一键填写并在核对后保存。通用登录后入口统一显示为“进入管理页面”，由账户角色决定目标工作区。账户验证在账户设置中维护，敏感操作审计从日志页进入；设置目录仅保留独立配置。

页面与设置子页切换使用平滑淡入淡出，自定义下拉菜单和折叠内容使用短距离展开与收起动画。原生 `select` 的弹出列表由系统绘制。

可复现安装使用 `bun install --frozen-lockfile`，发布前执行 `bun run build:check`。版本标识和前端先构建、Go 后嵌入的完整顺序见根目录 [源码发布与版本](../README.md#源码发布与版本)。
