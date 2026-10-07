#!/bin/bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$script_dir/.." && pwd)"
release_version="$(tr -d '\r\n' < "$repo_dir/VERSION")"
if [[ -z "$release_version" ]]; then
    echo "VERSION must contain the application version." >&2
    exit 1
fi
export GOWORK=off

echo "Building further-api desktop app ($release_version)..."

echo "Step 1: Building frontend..."
cd "$repo_dir/web"
bun install --frozen-lockfile
DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION="$release_version" bun run build

echo "Step 2: Building Go backend..."
cd "$repo_dir"
ldflags="-s -w -X github.com/dqt081228OHVGFY/further-api/common.Version=$release_version"
backend_file=further-api
package_script=build
if [[ "$OSTYPE" == "darwin"* ]]; then
    package_script=build:mac
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    package_script=build:linux
elif [[ "$OSTYPE" == "msys"* || "$OSTYPE" == "cygwin"* || "$OSTYPE" == "win32"* ]]; then
    backend_file=further-api.exe
    package_script=build:win
fi
CGO_ENABLED=0 go build -ldflags="$ldflags" -o "$backend_file"

echo "Step 3: Packaging desktop app..."
cd "$script_dir"
npm ci --include=dev
npm run "$package_script" -- "--config.extraMetadata.version=${release_version#v}"

echo "Build complete! Check electron/dist/ for output."
