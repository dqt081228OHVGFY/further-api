# further-api

`further-api` 是自托管 AI 模型网关，提供模型转发、渠道路由、用户与令牌管理、用量计费及管理控制台。

完整的中文部署、开发和许可说明见 [README.md](./README.md)。[English](./README.en.md) · [繁體中文](./README.zh_TW.md) · [Français](./README.fr.md) · [日本語](./README.ja.md)

## 快速开始

```powershell
git clone https://github.com/dqt081228OHVGFY/further-api.git
Set-Location further-api
Copy-Item compose.env.example .env
```

在 `.env` 中填写 `POSTGRES_PASSWORD`、`REDIS_PASSWORD`、`SESSION_SECRET`、`CRYPTO_SECRET`，然后执行 `docker compose up -d --build`。访问 <http://localhost:3000> 完成初始安装。生产部署应启用 HTTPS 并正确设置会话 Cookie 来源。

源码开发需要 Go 1.27.1 或更高兼容版本、Bun 1.4.2 和 Node.js；先在 `web/` 执行 `bun install --frozen-lockfile` 与 `bun run build`，然后在仓库根目录执行 `go run .`。

## 源码与迁移状态

前端源码位于 `further-ui/`（页面文档、脚本与样式）和 `web/src/further/`（公开页面、认证、独立功能工作区与 API 接入）。`bun run build` 会编译入口并运行 `web/scripts/build-further.mjs`，把首页、登录、账户与管理员管理页面及额度卡页面统一发布到 `web/dist/`，不再构建旧页面路由树。仅运行 `rsbuild build` 不等于完整发布。

服务端直接运行 further-api 的 Go 程序。账户认证与会话、身份绑定和安全验证、配置事务、订阅账本、请求调度与协议转换、厂商 HTTP/SSE/WebSocket 执行、计费结算、异步任务及支付订单通知等核心执行流程已重写并接入正式入口。官方协议 DTO、第三方 SDK、模型与区域常量、纯编解码工具和历史数据迁移继续保留。实现边界、验证要求与升级说明见 [后端实现与迁移](./docs/backend-architecture.md)。

项目的历史开源版权、许可和界面署名要求见 [LICENSE](./LICENSE)、[NOTICE](./NOTICE) 和 [第三方许可](./THIRD-PARTY-LICENSES.md)。产品名称统一为 **further-api**。
