#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${ROOT_DIR}/env.sh"
NVM_DIR="${NVM_DIR:-${HOME}/.nvm}"
if [[ -s "${NVM_DIR}/nvm.sh" ]]; then
  source "${NVM_DIR}/nvm.sh"
  nvm use 24 >/dev/null
fi
INSTALL_SH="${ROOT_DIR}/install.sh"
BUILD_DIR="${ROOT_DIR}/build"
FRONTEND_DIST_DIR="${ROOT_DIR}/frontend/dist"
FRONTEND_DIR="${ROOT_DIR}/frontend"
WAILS_BIN="$(command -v wails3 || true)"
VITE_PORT="${VITE_PORT:-9245}"
export GOCACHE="${GOCACHE:-${ROOT_DIR}/.melodex/go-build-cache}"
# Taskfile.yml derives its frontend port from WAILS_VITE_PORT. Export the
# bridge explicitly so `VITE_PORT=... ./dev.sh` keeps Wails and Vite aligned,
# including when the development port is intentionally changed from 9245.
export WAILS_VITE_PORT="${VITE_PORT}"
export VITE_PORT

melodex_enable_debug_logging
mkdir -p "${GOCACHE}"

cleanup_build_artifacts() {
  # Keep the Wails project metadata and platform assets under build/. Wails 3
  # reads build/config.yml before starting dev mode, and deleting that
  # directory makes the next invocation fail before the app can start.
  rm -rf "${BUILD_DIR}/bin" "${BUILD_DIR}/tmp" "${FRONTEND_DIST_DIR}/assets"
}

build_frontend_assets() {
  printf '[melodex] rebuilding frontend assets\n' >&2
  (cd "${FRONTEND_DIR}" && npm run build)
}

format_codebase() {
  printf '[melodex] formatting go sources\n' >&2
  go fmt ./...
  printf '[melodex] formatting frontend sources\n' >&2
  (cd "${FRONTEND_DIR}" && npm run format)
}

typecheck_codebase() {
  printf '[melodex] typechecking frontend\n' >&2
  (cd "${FRONTEND_DIR}" && npm run typecheck)
  printf '[melodex] linting frontend\n' >&2
  (cd "${FRONTEND_DIR}" && npm run lint)
}

if [[ -z "${WAILS_BIN}" ]]; then
  GOPATH_BIN="$(go env GOPATH)/bin/wails3"
  if [[ -x "${GOPATH_BIN}" ]]; then
    WAILS_BIN="${GOPATH_BIN}"
  fi
fi

if [[ -z "${WAILS_BIN}" ]]; then
  if [[ -x "${INSTALL_SH}" ]]; then
    printf '[melodex] Wails CLI not found. Running install bootstrap first.\n' >&2
    "${INSTALL_SH}"
    WAILS_BIN="$(command -v wails3 || true)"
    if [[ -z "${WAILS_BIN}" ]]; then
      GOPATH_BIN="$(go env GOPATH)/bin/wails3"
      if [[ -x "${GOPATH_BIN}" ]]; then
        WAILS_BIN="${GOPATH_BIN}"
      fi
    fi
  fi
fi

if [[ -z "${WAILS_BIN}" ]]; then
  printf '[melodex] Wails CLI still not found after bootstrap.\n' >&2
  exit 1
fi

export PATH="$(dirname "${WAILS_BIN}"):${PATH}"

cd "${ROOT_DIR}"
if [[ ! -f "${BUILD_DIR}/config.yml" ]]; then
  printf '[melodex] missing Wails development config: %s\n' "${BUILD_DIR}/config.yml" >&2
  exit 1
fi
format_codebase
typecheck_codebase
printf '[melodex] cleaning generated build artifacts\n' >&2
cleanup_build_artifacts

build_frontend_assets
"${WAILS_BIN}" dev -config "${BUILD_DIR}/config.yml" -port "${VITE_PORT}"
