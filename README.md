# further-api

`further-api` 是可自行部署的 AI 模型网关。服务端提供统一模型接口、渠道路由、账号与令牌管理、用量计费和管理 API；Web 控制台提供日常运营界面。正式入口是根目录的 Go 服务，前端源码由 `further-ui/` 和 `web/src/further/` 共同组成。

仓库地址：[dqt081228OHVGFY/further-api](https://github.com/dqt081228OHVGFY/further-api)。[English](./README.en.md) · [繁體中文](./README.zh_TW.md) · [Français](./README.fr.md) · [日本語](./README.ja.md)

## 能力

- 统一调用入口：OpenAI 兼容接口、Responses、Anthropic Messages、Gemini、图片、音频、向量与异步任务；实际能力取决于上游渠道。
- 管理页面：统一入口根据账户角色进入账户或管理员工作区；模型配置集中维护价格、倍率、渠道路由与上游别名，并提供一键填写参考价格。支持用户、令牌、订阅与福利、充值、兑换码、日志、插件和站点设置。
- 账户安全：短期访问令牌、HttpOnly 刷新 Cookie、服务端会话及可配置的多因素认证。
- 数据存储：SQLite、MySQL 或 PostgreSQL 作为主数据库；可选 Redis 缓存与会话共享。

## 快速部署

需要 Docker Engine 与 Docker Compose 插件。默认部署使用 PostgreSQL 15、Redis 7，监听本机 `3000` 端口。

```powershell
git clone https://github.com/dqt081228OHVGFY/further-api.git
Set-Location further-api
Copy-Item compose.env.example .env
```

编辑 `.env`，填写 `POSTGRES_PASSWORD`、`REDIS_PASSWORD` 和 `SESSION_SECRET`、`CRYPTO_SECRET`。然后启动：

```powershell
docker compose up -d --build
docker compose ps
docker compose logs -f further-api
```

打开 <http://localhost:3000> 完成首次安装。生产环境应在反向代理上配置 HTTPS，并把 `.env` 中的 `SESSION_COOKIE_SECURE` 设为 `true`、`SESSION_COOKIE_TRUSTED_URL` 设为准确的 HTTPS 来源。

Compose 默认使用 `further_api` 数据库及用户。已有数据库卷如果使用其他账号或库名，可在 `.env` 设置 `POSTGRES_USER`、`POSTGRES_DB` 保持连接，再安排数据迁移。升级前备份数据库卷和 `data/` 目录，切勿直接删除卷。

## 本地开发

### 前端源码与构建

- `further-ui/`：首页、登录、账户与管理员管理页面、额度卡页面及共享脚本和样式。
- `web/src/further/`：公开页面、认证流程、独立功能工作区及 API 接入。`feature-workspace/` 包含账户安全、模型厂商、充值支付、系统设置和运行监控界面。
- `web/src/main.tsx` 与 `web/src/further/bootstrap.ts` 是运行入口；构建不再使用旧页面路由树。
- 在 `web/` 执行 `bun run build` 会先编译运行入口，再由 `scripts/build-further.mjs` 发布所有页面和资源到 `web/dist/`。不要只复制某个页面或只执行 `rsbuild build` 作为完整发布。

### 后端迁移状态

本仓库直接运行 further-api 的 Go 服务。账户认证与会话、身份绑定和安全验证、配置事务、订阅账本、请求调度与协议转换、厂商 HTTP/SSE/WebSocket 执行、计费结算、异步任务及支付订单通知等核心执行流程已重写并接入正式入口。官方协议 DTO、第三方 SDK、模型与区域常量、纯编解码工具及历史数据迁移继续保留；历史贡献与许可要求仍然适用。具体实现边界和验证要求见 [后端实现与迁移](./docs/backend-architecture.md)。

### 启动开发环境

需要 Go 1.27.1 或更高兼容版本、Bun 1.4.2 和 Node.js 24.10.0（见 `web/.node-version`）。Go 会把 `web/dist` 内嵌到程序中，因此直接启动服务前需要构建前端。

```powershell
Set-Location web
bun install --frozen-lockfile
bun run build
Set-Location ..
go run .
```

默认使用当前工作目录的 SQLite 数据库。前端热更新可以另外运行 `bun run dev --port 5173`，把 `/api` 与模型接口代理至本地 Go 服务。开发用 Compose 文件是 `docker-compose.dev.yml`。

## API 示例

在控制台配置渠道、启用模型并创建 API Key 后：

```powershell
$env:FURTHER_API_KEY = '<your-api-key>'
curl.exe http://localhost:3000/v1/models -H "Authorization: Bearer $env:FURTHER_API_KEY"
```

管理 API 位于 `/api`，要求面板身份与相应权限。模型调用 API Key 不授予站点管理权限。

## 验证

```powershell
Set-Location web
bun run typecheck
bun run test
bun run build
Set-Location ..
go test ./...
```

`relaykit/` 是独立 Go 模块，修改其代码后还应在该目录运行 `go test ./...`。真实上游请求、支付通知和具体数据库组合需要在目标部署环境验收。

## 源码发布与版本

服务版本以根目录 `VERSION` 为准；当前值为 `v0.1.0-dev`。`web/package.json` 和 `electron/package.json` 的包版本不替代服务版本。源码仓库保留 `further-ui/`、`web/`、根 Go 模块、独立 `relaykit/` 模块、内置插件、数据库迁移源码、锁文件与许可文档；`web/dist/` 由完整前端构建生成，随后嵌入 Go 程序。

在 Windows PowerShell 中构建带版本标识的可运行程序：

```powershell
Set-Location web
bun install --frozen-lockfile
$env:VITE_REACT_APP_VERSION = (Get-Content -Raw ../VERSION).Trim()
bun run build:check
Set-Location ..
$releaseVersion = (Get-Content -Raw VERSION).Trim()
$env:GOWORK = 'off'
go build -trimpath -ldflags "-s -w -X github.com/dqt081228OHVGFY/further-api/common.Version=$releaseVersion" -o further-api.exe .
.\further-api.exe --version
```

完整验证仍按上面的验证流程执行；发布时还可在 `web/` 运行 `node --test scripts/workspace-motion-css.test.mjs scripts/home-motion.test.mjs scripts/native-features-css.test.mjs` 检查页面、弹窗和下拉动画的样式约束。桌面安装包另按 [Electron 构建说明](./electron/README.md) 打包和验收。

提交源码时排除依赖安装、构建产物、`.agent-work/`、`work/`、数据库、日志、真实 `.env` 配置和签名密钥。保留 `.env.example` 与 `compose.env.example` 作为空配置模板。升级使用同一源码版本的所有节点：先备份数据库并停止旧节点，启动主节点完成表结构和历史选项迁移，再启动其余节点；迁移由启动流程执行，具体兼容规则见 [后端实现与迁移](./docs/backend-architecture.md)。

## 文档与许可

- [认证与会话](./docs/authentication.md)
- [后端实现与迁移](./docs/backend-architecture.md)
- [订阅与福利](./docs/benefits.md)
- [风险控制](./docs/risk-control.md)
- [任务插件接口](./docs/plugin-api/README.md)
- [许可](./LICENSE)、[来源和附加署名要求](./NOTICE)、[第三方许可](./THIRD-PARTY-LICENSES.md)

本项目包含历史开源贡献。更改品牌不改变原作者的版权或许可要求；发行与界面署名请遵守 `NOTICE`。本项目的产品名称统一为 **further-api**。
