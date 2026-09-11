#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUTPUT_NAME="melodex-package-${STAMP}.zip"
TMP_DIR="$(mktemp -d)"
TMP_ZIP="${TMP_DIR}/${OUTPUT_NAME}"

cleanup() {
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

cd "${ROOT_DIR}"

if ! command -v zip >/dev/null 2>&1; then
  echo "zip is required but was not found in PATH" >&2
  exit 1
fi

zip -r "${TMP_ZIP}" . \
  -x ".git/*" \
  -x "frontend/node_modules/*" \
  -x "frontend/node_modules/**"

mv "${TMP_ZIP}" "${ROOT_DIR}/${OUTPUT_NAME}"
echo "Created ${ROOT_DIR}/${OUTPUT_NAME}"
