#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="${SCRIPT_DIR}/../settings"
TARGET_DIR="${HOME}/.openbrain/configs/settings"

if ! command -v rsync >/dev/null 2>&1; then
  echo "Error: rsync is not installed or not on PATH." >&2
  exit 1
fi

if [ ! -d "${SOURCE_DIR}" ]; then
  echo "Error: source settings directory not found: ${SOURCE_DIR}" >&2
  exit 1
fi

mkdir -p "${TARGET_DIR}"

echo "Syncing settings:"
echo "  from: ${SOURCE_DIR}/"
echo "  to:   ${TARGET_DIR}/"
echo "  mode: safe-copy (no delete)"

rsync -a "${SOURCE_DIR}/" "${TARGET_DIR}/"

echo "Done."
