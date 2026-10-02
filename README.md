# Further API · RelayFor Gateway

仓库地址：**[dqt081228OHVGFY/further-api](https://github.com/dqt081228OHVGFY/further-api)**。`further-api` 是本仓库名称，页面与已有程序配置沿用 RelayFor / New API 的标识。

RelayFor Gateway 是基于 [New API](https://github.com/QuantumNous/new-api) 的 AI 模型网关和管理控制台。本项目保留 New API 的真实账号、渠道、令牌、模型转发、计费和持久化能力，并接入 RelayFor 的页面、品牌资源及业务扩展。生产入口为 Go 服务：同一进程提供管理 API、模型转发接口和内嵌的 Web 页面。

首页、认证页、管理员及用户工作台已更新为蓝白分层的 Material 风格，包含响应式布局、分组导航、页面搜索、波纹与渐进动画。兑换码批次与明细、多用户令牌统计、模型健康、模型资料、别名路由和站点设置等页面连接真实网关 API。旧版 RelayFor 的 Node 预览服务、浏览器合成数据和“公益站／商业站”双站业务不属于当前运行架构。

> **部署前请注意：** 部分旧 RelayFor 高级业务规则尚未等价迁移。请阅读下文的[功能范围与已知限制](#功能范围与已知限制)及 [交付状态](./docs/DELIVERY_STATUS.md)，尤其是独立 Key 限额、异步任务折扣和高级兑换码类型。

## 目录

- [功能范围与已知限制](#功能范围与已知限制)
- [技术架构与代码目录](#技术架构与代码目录)
- [环境要求](#环境要求)
- [获取源码](#获取源码)
- [Docker Compose 部署](#docker-compose-部署)
- [配置说明](#配置说明)
- [本地开发与预览](#本地开发与预览)
- [API 使用示例](#api-使用示例)
- [检查与验证](#检查与验证)
- [常见问题](#常见问题)
- [文档索引](#文档索引)
- [仓库内容与本地文件](#仓库内容与本地文件)
- [来源、许可与署名](#来源许可与署名)

## 功能范围与已知限制

### 已接入的能力

| 领域 | 当前能力 |
| --- | --- |
| 网关转发 | OpenAI Chat Completions、Responses、Anthropic Messages、Gemini，以及图片、音频、向量和异步任务等接口；实际可用能力取决于上游渠道与模型。 |
| 管理控制台 | 账号、角色、渠道、模型、令牌、价格、订阅、充值、支付、兑换码、日志、插件和系统设置使用 New API 的真实 API 与数据库。 |
| RelayFor 界面 | `relayfor-ui/` 提供工作中的 HTML、布局、动画和品牌资源；`web/` 提供 React 控制台及桥接逻辑。首页和控制台可切换白、蓝、黄配色，浏览器分别保存配色和深浅模式。 |
| 站点品牌 | 站点名称和 Logo 使用持久化系统选项 `SystemName`、`Logo`；首页与控制台读取同一设置。默认图标为项目内置的握手 SVG。 |
| 管理扩展 | 兑换码批次与单码明细、按周期汇总的多用户令牌统计、用户可见模型健康、模型资料、别名路由、管理员风险识别。普通管理员可只读查看价格，超级管理员可修改定价。 |
| 原页面业务 | 词元贷、注册码、预付卡、订阅与福利已接入真实数据；具体规则及边界见 [原页面业务说明](./docs/relayfor-benefits.md)。 |
| 活动与节日主题 | 提供用户活动列表、领取、活动订阅、管理员活动与签到配置接口；页面可按服务端活动状态应用端午、七夕、中秋、重阳、跨年及国庆主题。 |
| LDC 支付接入 | 保留 LDC 配置、报价、充值与订阅支付、订单查询和支付通知接口；需要部署者配置服务商信息，真实支付回调仍需验收。 |
| 账号安全 | 面板鉴权使用短期 Access Token、HttpOnly Refresh Cookie 和服务端 Session；配置及多节点语义见 [认证与会话说明](./docs/authentication.md)。 |

旧演示页面在浏览器 `localStorage` 中保存的合成账户、渠道、Key 和订单不能转换为真实网关数据。首次启动后请通过安装向导创建管理员，并重新配置真实上游渠道。

### 尚未完整迁移的旧业务

下表说明旧 RelayFor 业务规则的边界，不代表 New API 自身对应功能不可用。具体情况以 [交付状态](./docs/DELIVERY_STATUS.md) 为准。

| 旧业务规则 | 当前边界 |
| --- | --- |
| 渠道及上游 Key 的独立额度、RPM、并发和权重 | 已有部分数据模型；非默认配置可能明确拒绝，不能视为完整执行链。 |
| 非 1 倍率套餐的异步任务折扣 | 尚未覆盖提交、轮询和结算的完整生命周期；相关请求在提交上游前拒绝并退回预扣。 |
| 原高级路由、别名及独立渠道权重／优先级 | 部分旧表单参数尚不能等价执行；New API 自身的高级管理功能仍可用。 |
| 原公共多次、订阅型、展期型及独立额度组兑换码 | 普通额度兑换已实现；这些高级类型尚未闭合。 |
| 部分旧安全操作表单 | 某些旧表单路径尚未逐项映射；可使用现有 New API 账号安全功能。 |
| 预付卡逐 Key 资金隔离 | 限时额度复用订阅资金源和到期逻辑，尚无与账户其他 Key 完全隔离的资金系统。 |
| 外部环境验收 | 真实上游、支付回调以及 MySQL／PostgreSQL／Redis 的完整组合尚未全部验收。 |

## 技术架构与代码目录

生产构建先用 Bun 和 Rsbuild 生成 `web/dist`，再编译 Go 程序。Go 通过 `//go:embed web/dist` 内嵌前端；Docker Compose 使用本仓库的 [Dockerfile](./Dockerfile) 从本地源码构建应用镜像。Compose 同时启动 PostgreSQL 和 Redis。主数据库也支持 SQLite、MySQL、PostgreSQL；日志库可单独使用 ClickHouse，具体配置见 [.env.example](./.env.example)。

| 路径 | 用途 |
| --- | --- |
| `main.go`、`router/`、`middleware/`、`controller/` | 服务入口、HTTP 路由、鉴权与管理 API。 |
| `relay/`、`relaykit/`、`plugins/tasks/` | 上游渠道适配、协议转换、流式转发及 JavaScript 任务插件；`relaykit/` 为本地 Go 子模块。 |
| `model/`、`service/`、`setting/` | 数据持久化、业务服务、计费、任务和系统设置。 |
| `web/` | React、TypeScript、Rsbuild 控制台与 RelayFor 桥接代码。 |
| `relayfor-ui/` | RelayFor 页面、布局、动画、资源及恢复来源记录。 |
| `electron/` | New API 桌面端工程；生产 Compose 不依赖它。 |
| `docs/` | 交付边界、鉴权、风险控制及业务说明。 |
| `bin/` | Windows 本地构建与预览脚本。 |
| `gateway/` | 项目中保留的网关源码目录；根目录 `main.go` 与 `Dockerfile` 是本文部署命令使用的入口。 |

## 环境要求

| 使用方式 | 需要的软件 |
| --- | --- |
| Docker 部署 | Docker Engine 与 Docker Compose 插件；构建时会拉取 Dockerfile 中指定的 Bun、Go 和运行时基础镜像。 |
| 本地源码运行 | Go 1.25.1 或兼容的更新版本、Bun（Docker 与 CI 使用 1.4.0）、Node.js（`web/.node-version` 为 24.10.0）；Web 构建脚本使用 Node.js。 |
| Windows 预览脚本 | PowerShell 7（`pwsh`）、Go、Node.js／npm，以及已安装的前端依赖；脚本通过 `npm run build` 构建前端。 |

默认服务端口为 `3000`，前端开发服务器通常为 `5173`。首次 Docker 构建会下载依赖；请为前端依赖、Go 模块和镜像层留出足够磁盘空间。

## 获取源码

```bash
git clone https://github.com/dqt081228OHVGFY/further-api.git
cd further-api
```

下文使用仓库根目录作为工作目录。根目录 Go 模块通过 `replace` 引用本地 `relaykit/`，因此应保留该子目录。`go.mod` 延续上游的模块路径 `github.com/QuantumNous/new-api`，克隆到 `further-api` 目录不影响本地构建。

## Docker Compose 部署

以下命令在仓库根目录执行。默认 Compose 文件会启动 `new-api`、PostgreSQL 15 和 Redis 7，发布 `3000:3000`。网关数据保存在 `./data`，日志保存在 `./logs`，数据库保存在 `pg_data` 命名卷。

1. 复制配置模板：

   ```powershell
   Copy-Item compose.env.example .env
   ```

   Linux / macOS 使用 `cp compose.env.example .env`。已有 `.env` 时直接编辑现有文件，保留原密钥和数据库配置。

2. 编辑 `.env`，填写 `POSTGRES_PASSWORD`、`REDIS_PASSWORD` 和 `SESSION_SECRET`。这三项为空时 Compose 会拒绝启动。数据库与 Redis 密码会进入连接 URL，建议使用 URL 安全的随机字符，例如十六进制字符串。多节点服务必须使用相同的 `SESSION_SECRET`。`.env` 按现有 `.gitignore` 不纳入 Git。

3. 构建并启动：

   ```powershell
   docker compose up -d --build
   docker compose ps
   docker compose logs -f new-api
   ```

4. 打开 <http://localhost:3000>，按首次安装向导创建管理员；然后在控制台配置上游渠道、模型、价格，并为用户创建 API Key。接口状态可用 `curl.exe http://localhost:3000/api/status` 检查。

升级时可在保留 `.env`、`./data`、`./logs` 和 `pg_data` 的前提下重新执行 `docker compose up -d --build`。升级前请备份数据库卷和 `data` 目录。需要保留数据时不要运行 `docker compose down -v`。

### HTTPS 与反向代理

本地 HTTP 可使用模板中的 `SESSION_COOKIE_SECURE=false`。公开 HTTPS 站点应将其设为 `true`，并在 `SESSION_COOKIE_TRUSTED_URL` 写入用户实际访问的完整 HTTPS Origin；多项以英文逗号分隔，不支持通配符或路径。反向代理需支持 SSE 流式响应和 WebSocket 升级。

Compose 默认 `TRUSTED_PROXIES=none`，不信任客户端提供的代理头。部署在反向代理后时，应填写代理自身的 IP 或 CIDR，而不是访问者的地址范围。会话、Cookie 和多节点 Redis 的详细语义见 [认证与会话说明](./docs/authentication.md)。

## 配置说明

| 变量 | 用途与默认值 | 适用方式 |
| --- | --- | --- |
| `POSTGRES_PASSWORD` | Compose 中 PostgreSQL 的密码；必填。 | Docker Compose |
| `REDIS_PASSWORD` | Compose 中 Redis 的密码；必填。 | Docker Compose |
| `SESSION_SECRET` | 登录与安全流程的高强度随机密钥；Compose 必填，更换会使现有登录会话失效。 | Docker Compose／直接运行 |
| `SESSION_COOKIE_SECURE` | 本地 HTTP 为 `false`；公开 HTTPS 站点设为 `true`。 | Docker Compose／直接运行 |
| `SESSION_COOKIE_TRUSTED_URL` | Secure 模式下允许的完整 HTTPS Origin，多个用逗号分隔。 | Docker Compose／直接运行 |
| `TRUSTED_PROXIES` | Compose 默认 `none`；有反向代理时显式列出代理 IP／CIDR。 | Docker Compose／直接运行 |
| `SQL_DSN` | 主数据库连接串；Compose 已指向 PostgreSQL，不设置时可使用本地 SQLite。 | 直接运行时按需设置 |
| `REDIS_CONN_STRING` | Redis 连接串；Compose 已设置。 | 直接运行时按需设置 |
| `BIND_HOST` | 本机预览时可设为 `127.0.0.1`，只监听回环地址。 | 直接运行 |
| `PORT` | HTTP 监听端口，默认 `3000`。 | 直接运行 |

更多数据库、缓存、任务、超时、日志、登录和支付设置见 [.env.example](./.env.example)。[compose.env.example](./compose.env.example) 是当前 Compose 所需配置的精简模板；Compose 从根目录 `.env` 或当前 shell 环境中读取其文件引用的变量，应用的其他变量应按需加入服务的 `environment` 配置。不要把真实上游密钥或支付凭据写入 README、示例文件或提交记录。

## 本地开发与预览

### 构建前端并运行 Go

直接运行 Go 前必须先生成 `web/dist`，因为 Go 服务会内嵌该目录：

```powershell
Set-Location web
bun install --frozen-lockfile
bun run build
Set-Location ..
go run .
```

不设置 `SQL_DSN` 时使用当前工作目录下的 SQLite 文件。打开 <http://localhost:3000>。仅在本机访问时可设置 `BIND_HOST=127.0.0.1`。`bun run build` 同时发布 `relayfor-ui/` 的页面和资源到 `web/dist`。

### 前端热更新

可先启动开发 Compose 后端，再启动 Rsbuild：

```powershell
docker compose -f docker-compose.dev.yml up -d --build
Set-Location web
bun install --frozen-lockfile
bun run dev --port 5173
```

上述命令将开发页面设在 <http://localhost:5173>；端口被占用时以终端输出为准。Rsbuild 将 `/api`、`/v1`、`/mj`、`/pg` 代理至默认的 `http://localhost:3000`；要改目标，可设置 `VITE_REACT_APP_SERVER_URL`。开发 Compose 使用独立的 `dev_data` 和 `dev_pg_data` 卷，并通过本地源码构建后端。首次使用开发 Compose 前，同样需要按部署章节准备 `.env`。

### Windows 本地预览脚本

在已安装依赖的工作目录中，以下脚本构建前端和 Go，再启动完整预览：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File .\bin\start-local-preview.ps1
```

默认地址为 <http://127.0.0.1:3000/>；可使用 `-Port 3001` 指定端口，或用 `-Foreground` 让终端持续等待服务退出。脚本默认在后台启动服务，本地 SQLite 与会话密钥保存在 `.agent-work/local-preview/`。已有构建时，可运行 `pwsh -NoProfile -ExecutionPolicy Bypass -File .\bin\start-built-preview.ps1` 选择现有程序。预览脚本不会替代生产 Compose 部署。

## API 使用示例

安装完成、配置上游渠道并创建 API Key 后，可从根目录执行：

```powershell
# 公开状态接口
curl.exe http://localhost:3000/api/status

# 把控制台创建的 API Key 放到当前终端环境变量中
$env:GATEWAY_API_KEY = '<your-api-key>'
curl.exe http://localhost:3000/v1/models -H "Authorization: Bearer $env:GATEWAY_API_KEY"
```

最小 Chat Completions 请求示例，`<model-id>` 应替换为已配置且可用的模型：

```powershell
$body = @{ model = '<model-id>'; messages = @(@{ role = 'user'; content = '你好' }) } | ConvertTo-Json -Depth 5
Invoke-RestMethod -Uri 'http://localhost:3000/v1/chat/completions' `
  -Method Post -Headers @{ Authorization = "Bearer $env:GATEWAY_API_KEY" } `
  -ContentType 'application/json' -Body $body
```

常用转发入口还包括 `POST /v1/responses`、`POST /v1/messages` 和 Gemini 的 `/v1beta/models/{model}:{action}`。管理 API 位于 `/api`，使用面板登录会话及角色权限，不能仅凭模型调用 API Key 管理站点。渠道与模型的实际协议能力以所选上游为准。

## 检查与验证

构建或修改后可按需运行下列命令。`make test` 会测试根 Go 模块和 `relaykit/` 子模块；前端测试、类型检查为独立命令。

```powershell
# 在项目根目录；需要 Go、make 及兼容 POSIX 的 shell（Windows 可用 WSL / Git Bash）
make test

# 前端检查
Set-Location web
bun run typecheck
bun run test
bun run build
```

服务启动后，可检查 `/api/status`、`/v1/models`，并在浏览器完成首次安装、登录、渠道配置和实际模型请求。`output/playwright/` 留有本地页面检查记录；[交付状态](./docs/DELIVERY_STATUS.md) 说明哪些路径已验证，以及哪些真实上游、支付回调和数据库组合仍需部署方验收。

上述命令是后续开发和部署的检查方法。本次首次上传主要整理 README 并保存当前项目快照，未重新执行完整构建、测试或外部服务验收；历史检查记录应按其日期和覆盖范围阅读。

## 常见问题

| 现象 | 检查方法 |
| --- | --- |
| Compose 拒绝启动并提示缺少变量 | 确认根目录 `.env` 已由 `compose.env.example` 复制，并填写三项必填密钥。 |
| Go 构建提示找不到 `web/dist` | 先在 `web/` 执行 `bun install --frozen-lockfile` 与 `bun run build`。 |
| 页面可打开但模型调用失败 | 确认安装向导已完成、上游渠道密钥有效、模型已启用且 API Key 有权限与余额；再查 `docker compose logs new-api`。 |
| 前端开发页请求连不到 API | 确认后端监听 `3000`；如后端地址不同，设置 `VITE_REACT_APP_SERVER_URL` 后重启 Rsbuild。 |
| HTTPS 下登录或刷新失败 | 检查 `SESSION_COOKIE_SECURE=true`、精确的 `SESSION_COOKIE_TRUSTED_URL`、浏览器实际 Origin 和反向代理转发。 |
| 反向代理后 IP 判断异常 | 检查 `TRUSTED_PROXIES` 是否只包含实际代理地址，并阅读 [认证与会话说明](./docs/authentication.md)。 |
| 旧演示数据不见了 | 旧 `localStorage` 合成数据不会迁移；通过安装向导及真实管理界面重新配置。 |

## 文档索引

| 文档 | 内容 |
| --- | --- |
| [交付状态](./docs/DELIVERY_STATUS.md) | 已接入能力、旧业务迁移边界和已有验证证据。 |
| [认证与会话说明](./docs/authentication.md) | Access／Refresh Token、Session、Cookie、Origin 与多节点 Redis。 |
| [管理员风险识别](./docs/risk-control.md) | 风险信号、评分、数据窗口及管理员接口。 |
| [RelayFor 原页面业务](./docs/relayfor-benefits.md) | 词元贷、注册码及相关业务规则。 |
| [RelayFor 前端恢复说明](./relayfor-ui/RESTORATION.md) | 原页面源码、资源、桥接方式及恢复记录。 |
| [RelayKit 说明](./relaykit/README.md) | 协议转换子模块。 |
| [管理 API 定义](./docs/openapi/api.json) / [转发 API 定义](./docs/openapi/relay.json) | 已有 OpenAPI 参考；RelayFor 扩展接口以当前 `router/relayfor_*.go` 路由为准。 |
| [任务插件文档](./docs/plugin-api/README.md) | 插件 API、类型定义及使用说明。 |
| [New API 官方文档](https://docs.newapi.ai/en/docs) | 上游网关的完整产品与部署参考。 |

## 仓库内容与本地文件

首次上传保留原项目目录结构，源码、现有文档、`gateway/` 源码副本，以及未被现有忽略规则排除的 `.agent-work/`、`.playwright-cli/` 和 `output/` 记录均随仓库保存。这些归档和检查记录可用于追溯，运行入口仍是仓库根目录。

上传遵循原有 [`.gitignore`](./.gitignore) 及子目录中的忽略规则：`web/node_modules/`、`web/dist/`、本地 `.env`、数据库、日志、可执行文件及 `.agent-work/local-preview/` 等不进入 Git 历史。本地原文件保留，不作清理；新克隆的仓库需要重新安装依赖并构建，也不会带入原机器的预览账户、会话密钥或数据库。`README.en.md` 等其他语言文档保留上游内容，本项目的运行方式和迁移边界以本 README 及 `docs/` 中的说明为准。

## 来源、许可与署名

本项目的后端源码取自 `newapi1`，上游基准提交为 `d04c118c8803f49e0c9bab74dcf5b5efeab9464a`（2026-09-23）。本项目的修改重点是 RelayFor 风格页面、品牌资源和本地构建部署配置；原页面来源及哈希记录见 [`relayfor-ui/source-provenance.json`](./relayfor-ui/source-provenance.json)。

本项目是 New API 的修改版，遵循 [AGPLv3](./LICENSE) 及 [NOTICE](./NOTICE) 的附加条款。界面必须保留署名 **Frontend design and development by New API contributors.** 和指向 [New API 原项目](https://github.com/QuantumNous/new-api) 的可见链接。第三方许可见 [THIRD-PARTY-LICENSES.md](./THIRD-PARTY-LICENSES.md)。
