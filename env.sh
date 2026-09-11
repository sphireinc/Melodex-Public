#!/usr/bin/env bash

if [[ -z "${ROOT_DIR:-}" ]]; then
  ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

export MELODEX_DEBUG="${MELODEX_DEBUG:-0}"
export MELODEX_LOG_DIR="${MELODEX_LOG_DIR:-${ROOT_DIR}/.melodex/logs}"
export MELODEX_LOG_FILE="${MELODEX_LOG_FILE:-${MELODEX_LOG_DIR}/dev.log}"

melodex_debug_enabled() {
  local normalized
  normalized="$(printf '%s' "${MELODEX_DEBUG}" | tr '[:upper:]' '[:lower:]')"

  case "${normalized}" in
    1 | true | yes | on | debug)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

melodex_enable_debug_logging() {
  if ! melodex_debug_enabled; then
    return 0
  fi

  mkdir -p "${MELODEX_LOG_DIR}"
  touch "${MELODEX_LOG_FILE}"
  printf '[melodex] debug logging enabled; teeing output to %s\n' "${MELODEX_LOG_FILE}" >&2
  exec > >(tee -a "${MELODEX_LOG_FILE}") 2>&1
}
