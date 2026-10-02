You are implementing an authorized software task in D:\test. Do the work to completion, not a plan or a proposal. The human has answered all scope questions. Do not ask those questions again.

CONFIRMED USER REQUEST (Chinese):
1. 把 relayfor-rebuild 改成完整网关，合并本地 newapi1 的全部功能。
2. 保留 relayfor 现有后台布局和同款动画，把 New API 新功能接入该后台。
3. 去除现在 relayfor 后台旧商业站的全部逻辑与页面配置。
4. The user explicitly clarified the preceding point: 保留 New API 的充值、订阅、支付等商业功能实现，仅删除 relayfor 原来的商业逻辑。Do NOT remove New API billing, payment, subscriptions, redemption, affiliate, or quota accounting.
5. 首页重做；网站要有图标；网站名字可以修改。
6. User asked to be informed if a real unresolved conflict requires a decision; the choices above are settled. Use reasonable defaults for technical implementation.

WORKSPACE FACTS:
- D:\test\relayfor-rebuild is NOT a git repo. Treat its current files as user work. Back up any existing runtime files you replace to a clearly excluded archival location under .agent-work. Do not destroy or rewrite the evidence directories port/, admin-mirror/, or probe/ unnecessarily.
- D:\test\newapi1 IS the selected full Go/Gin + React19/TypeScript/Rsbuild New API source, currently clean. D:\test\newapi is NOT the selected source. Leave both source directories unchanged.
- Read D:\test\NEWAPI_RELAYFOR_COMPARISON.md for useful existing investigation, but verify current actual files because some later branding/home work already exists.
- Relay source includes admin-site/admin/index.html, admin-site/app.js, app.css, admin.js, admin-overview.js, admin-settings.js, admin-models.js, admin-routing.js, channel-keys.js, disabled-channels.js, site-settings.js, runtime-auth-config.js, site-branding-store.mjs, site-branding.test.mjs, and Node preview server.mjs. admin-fixture.js and runtime-fixtures are demo interceptors.
- mirror/ has user console, auth, app.css/app.js, console.js, site.js, and more recent home.css, home.js, index.html, site-branding.css, site-branding.js. Read the actual existing changes before replacing anything.
- src/ was previously only a planning README. The old preview server is not a gateway: no actual relay, database, or real admin auth.
- newapi1 includes full real relay protocols, task plugins, RBAC, OAuth/Passkey/MFA, billing/payment/subscription/account/user/admin APIs, React feature screens, tests, and deployment files. Preserve the complete implementation and license/copyright/source attribution.
- newapi1 AGENTS.md and web/AGENTS.md give conventions. Read applicable complete files, and newapi1/.agents skills if modifying its copied frontend. User instructions take priority over lower-priority policy preferences, but retain source licensing/attributions. Configurable operator site name is a normal option; do not mass replace New API project identifiers.
- node/npm/npx/python/git/codex are on PATH. go and bun were not found on PATH in an earlier inspection; look for usable bundled/cached runtimes and Docker before assuming available. No dependencies have been installed yet.
- Writable root is D:\test. Do not edit global config or credentials. No public deployment, payment transaction, external message, push, or commit requested.
- Current date 2026-09-24.

ARCHITECTURAL TARGET:
Create an independently buildable/deployable product in relayfor-rebuild, with full selected newapi1 gateway source vendored under relayfor-rebuild/gateway/ (exclude .git, private .env files, databases, credentials, node_modules, build output, cache; retain source, package locks, tests, plugins, relaykit, public assets, licenses). Do not just point production at the sibling newapi1 folder.
Preserve all real New API backend behavior/routes/auth/accounting/plugin capabilities. Prefer minimal host wiring over reimplementing any proxy, auth, payment, or accounting logic.
Retain RelayFor's actual visual layout, navigation regions, sidebar/header/content geometry, theme, buttons, dialogs, and animations. Reusing complete New API React feature pages inside a faithfully ported RelayFor shell is acceptable and preferable to rewriting thousands of functional controls. Do NOT simply ship unmodified New API UI or replace the RelayFor layout with the New API layout. Do NOT deliver fake endpoints, fixture data, an iframe-based facade, or merely link users to a second New API backend for missing features.
Every existing New API functional page/route must remain reachable in the integrated RelayFor shell with real API calls and original authorization. Add necessary navigation groups into the existing layout. Maintain original /admin and /console entry paths via proper redirects or routes without breaking New API deep links.
Production must stop executing old admin-fixture.js, runtime-fixtures, synthetic login, localStorage synthetic admin/business state, fake model/channel/user lists, legacy prepaid/registration-card/token-loan/loss-pool/promotion logic, and old payment/configuration contracts. Archive evidence if needed but ensure it is not copied into public builds/served by production and document the archive boundary. Remove obsolete commercial menu/settings entries; use the original New API feature implementations for corresponding retained commerce.
Production should have a single clear launch path, real backend and frontend build, environment example, and Docker/Compose if available. Existing preview launcher should guide or invoke the new gateway, not silently boot the obsolete commercial demo.

ANIMATION CONTRACT:
Read relayfor-rebuild/ANIMATION_CHECKLIST.md and actual app.css/app.js. Preserve the actual original implementation timings and layout behavior; do not guess or redesign animation constants:
--md-sys-motion-easing-standard: cubic-bezier(.2,0,0,1)
standard-decelerate: cubic-bezier(0,0,0,1)
standard-accelerate: cubic-bezier(.3,0,1,1)
emphasized-decelerate: cubic-bezier(.05,.7,.1,1)
emphasized-accelerate: cubic-bezier(.3,0,.8,.15)
duration-short-2:100ms, short-4:200ms, medium-1:250ms, medium-2:300ms,long-1:450ms
Use click-position Material ripple with cleanup, correct staged panel transitions, theme transition, toast and dialog motion, mobile sidebar and loading states. Match observed original values if the actual CSS differs from old documentation. Respect reduced-motion. Port original code faithfully into framework hooks where necessary.

HOME / BRAND:
Read and apply frontend-design skill at C:\Users\Administrator\.agents\skills\frontend-design\SKILL.md for the home redesign while matching the existing RelayFor visual identity and motion (not a generic unrelated design).
Build a complete responsive new homepage appropriate for this real unified gateway, with actual navigation to model catalog, console, account/setup, docs/capabilities. No fabricated uptime, usage numbers, provider coverage counts, testimonials, pricing, or working claims.
Use an actual usable vector website icon/favicon (include local fallback; no missing icon-font glyphs). Use New API's existing server-side site-branding/options infrastructure for a persistently configurable website name/logo/favicon displayed consistently in home, login, console, browser title, mobile navigation; keep source author attribution. Do not make it just an unprotected localStorage field. Prefer reuse of New API existing settings and admin permissions instead of inventing a weaker branding API.
Preserve existing custom homepage/branding support where appropriate.

SECURITY / VERIFICATION:
- Read applicable actual authentication code before touching. Prefer keeping it untouched. Never transplant the synthetic Bearer auth, generate hardcoded root passwords/API keys, weaken RBAC, make protected options public, store real secrets in fixtures, or mask network failure with dummy success.
- If changes affect auth flows, read official OWASP Authentication and Session Management Cheat Sheets as required by the source repository; preserve backend-enforced permissions/session and CSRF assurance. Test applicable failure behavior.
- Do not change DB schemas/driver versions just for integration. If database behavior changes, source AGENTS requires real SQLite/MySQL/PostgreSQL verification; otherwise keep backend database code byte-identical and state no migration added.
- Read the Playwright skill at C:\Users\Administrator\.codex\skills\playwright\SKILL.md if using it. Test actual visible navigation, homepage, responsive/mobile menu, editable branding path, original-style motion, deep links and not-logged-in/permission behavior. Use available browser tooling; don't claim screenshot/visual QA without running it.
- Install required toolchains/dependencies only within authorized boundaries, following sandbox approvals. Never use --dangerously-bypass-approvals-and-sandbox or equivalent. If an unavoidable external restriction prevents a check, still complete code and all feasible checks and report exact blocker. Do not call source import alone complete or claim unrun tests.
- Run frontend typecheck, focused meaningful tests, production build, backend build and affected tests where possible. Independently build relaykit if modified; don't modify it unnecessarily.
- Ensure original source newapi1 stays unchanged. Inspect final output for active old fixture imports, invalid routes, missing favicon/title, known commercial-demo endpoints and fake success data.
- Add focused regression tests for route/nav reachability, complete imported feature coverage, removal of runtime fixtures, branding propagation/persistence and requested UI behavior. Existing backend tests can be retained. Avoid vacuous string-snapshot-only checks for interactive behavior.
- Write a concise root README with actual startup/build commands and a truthful integration/verification report in relayfor-rebuild/.agent-work or root (don't add docs/ under vendored source unless requested). Include real missing/unverified features rather than silently hiding them.

WORKFLOW:
Inspect, implement, verify, fix until a concrete reviewable result exists. This is substantial work; use available subagents for bounded nonoverlapping work if you have such tools, otherwise work locally. Avoid unrelated changes. Work autonomously within the user's confirmed scope, don't stop with a plan or another high-level question.
Your final message (saved by CLI) must be in Chinese and include:
1. What actually changed and where.
2. Confirmation of which old commercial demo functionality was removed and that real New API commerce is retained.
3. Exact real startup command(s), site name/icon configuration location.
4. Tests/builds actually run and results; concrete unresolved blockers or feature gaps.
5. Important modified file paths for the calling agent.
Do not assert "all features complete" without checking complete feature/route reachability. Do not include secrets.
