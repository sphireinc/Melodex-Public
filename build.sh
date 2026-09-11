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
export GOCACHE="${GOCACHE:-${ROOT_DIR}/.melodex/go-build-cache}"

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

RELEASE_CHANNEL="${MELODEX_RELEASE_CHANNEL:-stable}"
RELEASE_TAG="$(git describe --tags --exact-match 2>/dev/null || true)"
if [[ "${RELEASE_CHANNEL}" != "dev" && -z "${RELEASE_TAG}" ]]; then
  printf '[melodex] release builds must be produced from a tagged commit.\n' >&2
  printf '[melodex] use dev.sh for local development or tag the commit before building a release.\n' >&2
  exit 1
fi

APP_VERSION="${MELODEX_VERSION:-${RELEASE_TAG#v}}"
if [[ -z "${APP_VERSION}" ]]; then
  APP_VERSION="${MELODEX_VERSION:-$(git describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null | sed 's/^v//')}"
fi
if [[ -z "${APP_VERSION}" ]]; then
  APP_VERSION="1.0.0"
fi
BUILD_NUMBER="${MELODEX_BUILD_NUMBER:-$(date -u +%Y%m%d%H%M%S)}"
GIT_COMMIT="${MELODEX_GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo dev)}"
BUILD_TIME="${MELODEX_BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
export APP_LDFLAGS="-X main.AppVersion=${APP_VERSION} -X main.BuildNumber=${BUILD_NUMBER} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME} -X main.ReleaseChannel=${RELEASE_CHANNEL}"

BUILD_PLATFORM="${MELODEX_BUILD_PLATFORM:-}"
if [[ -n "${BUILD_PLATFORM}" ]]; then
  BUILD_OS="${BUILD_PLATFORM%%/*}"
  BUILD_ARCH="${BUILD_PLATFORM##*/}"
  export GOOS="${BUILD_OS}"
  if [[ "${BUILD_ARCH}" != "universal" ]]; then
    export ARCH="${BUILD_ARCH}"
  fi
fi

if [[ "${BUILD_PLATFORM}" == "darwin/universal" ]]; then
  # Release workflows consume the standalone application bundle, so package
  # the universal binary here instead of leaving only bin/Melodex behind.
  "${WAILS_BIN}" task darwin:package:universal
elif [[ "${MELODEX_BUILD_INSTALLER:-0}" == "1" ]]; then
  "${WAILS_BIN}" task windows:package
else
  "${WAILS_BIN}" build
fi

if [[ "$(uname -s)" == "Darwin" ]]; then
  APP_BUNDLE="${ROOT_DIR}/bin/Melodex.app"
  APP_PLIST="${APP_BUNDLE}/Contents/Info.plist"
  if [[ -f "${APP_PLIST}" ]]; then
    /usr/libexec/PlistBuddy -c "Set :CFBundleIdentifier com.sphire.melodex" "${APP_PLIST}" 2>/dev/null || true
    /usr/libexec/PlistBuddy -c "Set :CFBundleVersion ${BUILD_NUMBER}" "${APP_PLIST}" 2>/dev/null || true
    /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString ${APP_VERSION}" "${APP_PLIST}" 2>/dev/null || true
    /usr/libexec/PlistBuddy -c "Set :CFBundleGetInfoString Melodex ${APP_VERSION} (${BUILD_NUMBER})" "${APP_PLIST}" 2>/dev/null || true
    /usr/libexec/PlistBuddy -c "Add :NSAppTransportSecurity dict" "${APP_PLIST}" 2>/dev/null || true
    /usr/libexec/PlistBuddy -c "Add :NSAppTransportSecurity:NSAllowsLocalNetworking bool true" "${APP_PLIST}" 2>/dev/null || \
      /usr/libexec/PlistBuddy -c "Set :NSAppTransportSecurity:NSAllowsLocalNetworking true" "${APP_PLIST}" 2>/dev/null || true
    /usr/libexec/PlistBuddy -c "Add :NSAppTransportSecurity:NSAllowsArbitraryLoadsInWebContent bool true" "${APP_PLIST}" 2>/dev/null || \
      /usr/libexec/PlistBuddy -c "Set :NSAppTransportSecurity:NSAllowsArbitraryLoadsInWebContent true" "${APP_PLIST}" 2>/dev/null || true
    /usr/bin/touch "${APP_BUNDLE}"
    # PlistBuddy changes invalidate the signature produced by the Wails
    # packaging task. Re-sign the local bundle after all metadata changes;
    # distribution CI will replace this ad-hoc signature with Developer ID.
    if command -v codesign >/dev/null 2>&1; then
      codesign --force --deep --sign - "${APP_BUNDLE}"
    fi
  fi
fi
