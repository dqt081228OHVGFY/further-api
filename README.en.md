# further-api

`further-api` is a self-hosted AI model gateway. The Go service provides model relay, channel routing, accounts, API keys, usage accounting, and management APIs. Its web console is implemented in `further-ui/` and `web/src/further/`.

Repository: [dqt081228OHVGFY/further-api](https://github.com/dqt081228OHVGFY/further-api). [简体中文](./README.md) · [繁體中文](./README.zh_TW.md) · [Français](./README.fr.md) · [日本語](./README.ja.md)

## Deploy

Docker Engine and Docker Compose are required. The default stack starts PostgreSQL 15, Redis 7, and the gateway on port `3000`.

```powershell
git clone https://github.com/dqt081228OHVGFY/further-api.git
Set-Location further-api
Copy-Item compose.env.example .env
```

Set `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, and `SESSION_SECRET`, `CRYPTO_SECRET` in `.env`, then run:

```powershell
docker compose up -d --build
docker compose logs -f further-api
```

Open <http://localhost:3000> to complete initial setup. For HTTPS deployments, configure the reverse proxy and set `SESSION_COOKIE_SECURE=true` and an exact `SESSION_COOKIE_TRUSTED_URL`. Back up the database and `data/` before upgrading. The default database and user are both `further_api`.

## Develop

`further-ui/` contains the home, sign-in, user console, administration, and prepaid-card documents with shared scripts and styles. `web/src/further/` contains public pages, authentication, API integration, and the independent feature workspaces. Runtime entries are `web/src/main.tsx` and `web/src/further/bootstrap.ts`; the previous page route tree is no longer built.

Run the complete `bun run build` command from `web/`. It compiles the runtimes and runs `scripts/build-further.mjs` to publish every page and its assets into `web/dist/`. Running only `rsbuild build` or copying an individual page does not produce a complete release.

The repository runs the further-api Go service. Core execution paths for authentication and sessions, identity binding and security checks, configuration transactions, subscription accounting, request routing and protocol conversion, provider HTTP/SSE/WebSocket handling, billing settlement, asynchronous tasks, and payment orders and notifications have been rewritten and connected to the production entry points. Official protocol DTOs, third-party SDKs, model and region constants, pure codecs, and migrations for existing data remain. Historical contributions and license obligations still apply. See [backend implementation and migration](./docs/backend-architecture.md) for implementation boundaries, validation, and upgrade requirements.

Use a compatible Go 1.27.1+, Bun 1.4.2, and Node.js. Build the frontend before running Go because the service embeds `web/dist`.

```powershell
Set-Location web
bun install --frozen-lockfile
bun run build
Set-Location ..
go run .
```

Run `bun run dev --port 5173` from `web/` for frontend hot reload. The development Compose file is `docker-compose.dev.yml`.

## Verify

Run `bun run typecheck`, `bun run test`, and `bun run build` in `web/`, then `go test ./...` at the repository root. `relaykit/` is an independent Go module and needs its own `go test ./...` after changes.

The management API lives under `/api`; model API keys are for model requests and do not grant console administration. Available model protocols depend on the configured upstream channels.

See [authentication](./docs/authentication.md), [plugin API](./docs/plugin-api/README.md), [LICENSE](./LICENSE), [NOTICE](./NOTICE), and [third-party licenses](./THIRD-PARTY-LICENSES.md). Historical open-source attributions and additional notice obligations in `NOTICE` remain in force. The product name is **further-api**.
