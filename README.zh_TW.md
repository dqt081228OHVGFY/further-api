# further-api

`further-api` 是可自行部署的 AI 模型閘道，提供模型轉送、通道路由、使用者與金鑰管理、用量計費及管理主控台。

[简体中文](./README.md) · [English](./README.en.md) · [Français](./README.fr.md) · [日本語](./README.ja.md)

## 快速開始

```powershell
git clone https://github.com/dqt081228OHVGFY/further-api.git
Set-Location further-api
Copy-Item compose.env.example .env
```

在 `.env` 填入 `POSTGRES_PASSWORD`、`REDIS_PASSWORD` 和 `SESSION_SECRET`、`CRYPTO_SECRET`，執行 `docker compose up -d --build`，然後開啟 <http://localhost:3000> 完成初次設定。正式環境應設定 HTTPS 與正確的 Session Cookie 來源。

本機開發需要 Go 1.27.1 或相容新版本、Bun 1.4.2 及 Node.js。先在 `web/` 執行 `bun install --frozen-lockfile`、`bun run build`，再於專案根目錄執行 `go run .`。

## 原始碼與遷移狀態

前端原始碼位於 `further-ui/`（頁面文件、指令碼與樣式）和 `web/src/further/`（公開頁面、認證、獨立功能工作區及 API 存取）。`bun run build` 會編譯入口並執行 `web/scripts/build-further.mjs`，將首頁、登入、使用者主控台、管理後台及額度卡頁面統一發佈至 `web/dist/`，不再建置舊頁面路由樹。僅執行 `rsbuild build` 並非完整發佈。

服務端直接執行 further-api 的 Go 程式。帳戶認證與工作階段、身分綁定與安全驗證、設定交易、訂閱帳本、請求調度與協定轉換、供應商 HTTP/SSE/WebSocket 執行、計費結算、非同步任務及支付訂單通知等核心流程已重新實作並接入正式入口。官方協定 DTO、第三方 SDK、模型與區域常數、純編解碼工具及歷史資料遷移繼續保留。實作範圍、驗證要求與升級說明請參閱 [後端實作與遷移](./docs/backend-architecture.md)。

歷史開源著作權、授權及介面署名要求請參閱 [LICENSE](./LICENSE)、[NOTICE](./NOTICE) 與 [第三方授權](./THIRD-PARTY-LICENSES.md)。產品名稱統一為 **further-api**。
