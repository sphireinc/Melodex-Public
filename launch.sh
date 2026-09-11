#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${ROOT_DIR}/env.sh"
NVM_DIR="${NVM_DIR:-${HOME}/.nvm}"
if [[ -s "${NVM_DIR}/nvm.sh" ]]; then
  source "${NVM_DIR}/nvm.sh"
  nvm use 24 >/dev/null
fi
WAILS_BIN="$(command -v wails3 || true)"

melodex_enable_debug_logging

if [[ -z "${WAILS_BIN}" ]]; then
  GOPATH_BIN="$(go env GOPATH)/bin/wails3"
  if [[ -x "${GOPATH_BIN}" ]]; then
    WAILS_BIN="${GOPATH_BIN}"
  fi
fi

if [[ -z "${WAILS_BIN}" ]]; then
  printf '[melodex] Wails CLI not found. Run ./install.sh first.\n' >&2
  exit 1
fi

export PATH="$(dirname "${WAILS_BIN}"):${PATH}"

cd "${ROOT_DIR}"
"${WAILS_BIN}" build

APP_BUNDLE="${ROOT_DIR}/bin/Melodex.app"
APP_BINARY="${ROOT_DIR}/bin/Melodex"

if [[ "$(uname -s)" == "Darwin" && -d "${APP_BUNDLE}" ]]; then
  open "${APP_BUNDLE}"
  exit 0
fi

if [[ -x "${APP_BINARY}" ]]; then
  "${APP_BINARY}"
  exit 0
fi

printf '[melodex] build completed, but no launchable artifact was found.\n' >&2
exit 1
