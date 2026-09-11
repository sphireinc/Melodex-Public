#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${ROOT_DIR}/env.sh"
FRONTEND_DIR="${ROOT_DIR}/frontend"
WAILS_VERSION="v3.0.0-beta.2"

melodex_enable_debug_logging

info() {
  printf '[melodex] %s\n' "$1"
}

have() {
  command -v "$1" >/dev/null 2>&1
}

require_cmd() {
  if ! have "$1"; then
    printf '[melodex] missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

info "checking toolchain"
require_cmd go
require_cmd node
require_cmd npm

if ! node -e 'const [major, minor] = process.versions.node.split(".").map(Number); process.exit(major > 20 || (major === 20 && minor >= 19) ? 0 : 1)'; then
  printf '[melodex] Node.js 20.19.0 or newer is required by the frontend toolchain.\n' >&2
  printf '[melodex] use nvm use 24, then rerun ./install.sh.\n' >&2
  exit 1
fi

if ! have wails3; then
  info "installing Wails CLI ${WAILS_VERSION}"
  GO111MODULE=on go install "github.com/wailsapp/wails/v3/cmd/wails3@${WAILS_VERSION}"
  export PATH="${PATH}:$(go env GOPATH)/bin"
fi

if ! have yt-dlp; then
  info "yt-dlp is not installed; Melodex can still run, but URL imports will be unavailable until it is installed"
fi

if ! have ffmpeg; then
  info "ffmpeg is not installed; Melodex can still run, but URL imports will be unavailable until it is installed"
fi

info "installing frontend dependencies"
cd "${FRONTEND_DIR}"
npm ci

info "verification"
cd "${ROOT_DIR}"
go test ./...
cd "${FRONTEND_DIR}"
npm run build

info "installation complete"
info "next: run ./dev.sh"
