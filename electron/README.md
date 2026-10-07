# further-api Desktop

The Electron wrapper runs the local `further-api` Go service and opens its console in a native window with a system tray icon.

## Build

The build script builds the frontend and Go binary before packaging. From the repository root:

```bash
./electron/build.sh
```

The script resolves paths from its own location and can also be run as `./build.sh` from `electron/`. It installs frontend dependencies with `bun install --frozen-lockfile` and Electron dependencies with `npm ci`, builds the complete web application, compiles the Go service as `further-api` (or `further-api.exe` on Windows), and creates the platform package in `electron/dist/`. Run it in Bash (Git Bash on Windows).

Frontend build metadata, the Go service version, and installer metadata are read from the root `VERSION` file. Installer metadata removes the leading `v` while retaining prerelease identifiers; package manifests and lockfiles are unchanged. The Go build uses `GOWORK=off` and `CGO_ENABLED=0`; generated frontend bundles and the gateway binary must come from the same source revision. Docker development builds also disable unrelated local Go workspaces.

The web sources are shared with the browser edition: [`further-ui/`](../further-ui/) contains page documents and assets, and [`web/src/further/`](../web/src/further/) contains public pages and feature workspaces. The `web/` build invokes `scripts/build-further.mjs` to publish all pages into `web/dist/` before Go embeds them. No separate desktop page tree is maintained, and the previous web route tree is no longer built.

For development, run the Go service at port `3000`, run `bun run dev --port 5173` from `web/`, and use `npm run dev-app` from this directory. Production packages include the Go binary and the license files.

The desktop database is stored as `further-api.db` in the app data directory. Back up an existing database before replacing a previous desktop installation. See the root [README](../README.md), [LICENSE](../LICENSE), and [NOTICE](../NOTICE).

The desktop package uses the same rewritten authentication, configuration, subscription, provider execution, billing, task, and payment services as the browser edition. Official protocol DTOs, third-party SDKs, data migrations, and historical license obligations remain part of the project. Packaging is a separate validation step; verify the final installer on its target platform. See [backend implementation and migration](../docs/backend-architecture.md) for the implementation and compatibility boundaries.
