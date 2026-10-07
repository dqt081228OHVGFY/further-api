# further-api

`further-api` はセルフホスト型の AI モデルゲートウェイです。プロバイダーのルーティング、アカウント、API キー、使用量と課金、管理コンソールを提供します。

[简体中文](./README.md) · [English](./README.en.md) · [繁體中文](./README.zh_TW.md) · [Français](./README.fr.md)

## クイックスタート

```powershell
git clone https://github.com/dqt081228OHVGFY/further-api.git
Set-Location further-api
Copy-Item compose.env.example .env
```

`.env` に `POSTGRES_PASSWORD`、`REDIS_PASSWORD`、`SESSION_SECRET`、`CRYPTO_SECRET` を設定し、`docker compose up -d --build` を実行します。<http://localhost:3000> で初期設定を完了してください。本番環境では HTTPS と正確なセッション Cookie のオリジンを設定してください。

ローカル開発には Go 1.27.1 以降の互換バージョン、Bun 1.4.2、Node.js が必要です。`web/` で `bun install --frozen-lockfile` と `bun run build` を実行した後、リポジトリのルートで `go run .` を実行します。

## ソース構成と移行状況

フロントエンドは `further-ui/`（ページ、スクリプト、スタイル）と `web/src/further/`（公開ページ、認証、独立した機能画面、API 接続）で構成されています。`bun run build` はエントリーポイントをコンパイルし、`web/scripts/build-further.mjs` を使ってホーム、ログイン、ユーザーコンソール、管理画面、プリペイドカード画面を `web/dist/` にまとめて出力します。旧ページのルートツリーはビルドしません。`rsbuild build` だけでは完全な配布物になりません。

本リポジトリは further-api の Go サービスを直接実行します。認証とセッション、外部アカウントの連携と安全性の確認、設定のトランザクション、サブスクリプション台帳、リクエストの振り分けとプロトコル変換、各プロバイダーの HTTP/SSE/WebSocket 処理、利用料金の精算、非同期タスク、決済注文と通知の主要な実行処理を再実装し、正式なエントリーポイントに接続しました。公式プロトコルの DTO、サードパーティー SDK、モデルとリージョンの定数、データのエンコード・デコード処理、既存データの移行機能は引き続き使用します。過去の貢献に関する権利とライセンス上の義務も継続します。実装範囲、検証内容、更新時の注意点は [バックエンドの実装と移行](./docs/backend-architecture.md) を参照してください。

過去のオープンソース著作権、ライセンス、表示義務は [LICENSE](./LICENSE)、[NOTICE](./NOTICE)、[サードパーティーライセンス](./THIRD-PARTY-LICENSES.md) を参照してください。製品名は **further-api** です。
