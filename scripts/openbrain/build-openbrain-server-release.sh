#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
APP_ROOT="${REPO_ROOT}/server"
OUT_DIR="${OPENBRAIN_SERVER_RELEASE_DIR:-${REPO_ROOT}/.tmp/openbrain-release-artifacts/server}"
BUILD_ROOT="${OPENBRAIN_SERVER_BUILD_ROOT:-${REPO_ROOT}/.tmp/openbrain-server-build}"
CHECKSUMS_NAME="server-SHA256SUMS"
VERSION="${OPENBRAIN_RELEASE_VERSION:-${GITHUB_REF_NAME:-dev}}"
VERSION="${VERSION#openbrain-v}"

usage() {
  cat <<'EOF'
usage: scripts/openbrain/build-openbrain-server-release.sh [platform...]

Build standalone openbrain-server release archives.

Platforms:
  darwin-arm64
  darwin-amd64
  linux-amd64
  windows-amd64

Environment:
  OPENBRAIN_RELEASE_VERSION      Version embedded into openbrain-server.
  OPENBRAIN_SERVER_RELEASE_DIR   Output directory.
  OPENBRAIN_SERVER_BUILD_ROOT    Temporary build root.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

PLATFORMS=("$@")
if [[ "${#PLATFORMS[@]}" -eq 0 ]]; then
  PLATFORMS=(darwin-arm64 darwin-amd64 linux-amd64 windows-amd64)
fi

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "Missing command: $1" >&2; exit 1; }
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

zip_dir() {
  local src_dir="$1"
  local out_file="$2"
  python3 - "$src_dir" "$out_file" <<'PY'
import os
import sys
import zipfile
from pathlib import Path

src = Path(sys.argv[1])
out = Path(sys.argv[2])
with zipfile.ZipFile(out, "w", compression=zipfile.ZIP_DEFLATED) as zf:
    for path in sorted(p for p in src.rglob("*") if p.is_file()):
        zf.write(path, path.relative_to(src).as_posix())
PY
}

require_cmd go
require_cmd tar
require_cmd python3

mkdir -p "${OUT_DIR}" "${BUILD_ROOT}"
rm -f "${OUT_DIR}/SHA256SUMS" "${OUT_DIR}/${CHECKSUMS_NAME}"
: > "${OUT_DIR}/${CHECKSUMS_NAME}"

for platform in "${PLATFORMS[@]}"; do
  case "${platform}" in
    darwin-arm64|darwin-amd64|linux-amd64|windows-amd64) ;;
    *) echo "unsupported openbrain-server platform: ${platform}" >&2; exit 1 ;;
  esac

  platform_build_root="${BUILD_ROOT}/${platform}"
  rm -rf "${platform_build_root}"

  echo "[openbrain-server] platform=${platform} version=${VERSION}"
  (
    export OPENBRAIN_SERVER_DIST_ROOT="${platform_build_root}"
    export OPENBRAIN_SERVER_VERSION="${VERSION}"
    bash "${APP_ROOT}/scripts/build.sh" "${platform}"
  )

  if [[ "${platform}" == windows-* ]]; then
    artifact="${OUT_DIR}/openbrain-server-${platform}.zip"
    rm -f "${artifact}"
    zip_dir "${platform_build_root}" "${artifact}"
  else
    artifact="${OUT_DIR}/openbrain-server-${platform}.tar.gz"
    rm -f "${artifact}"
    tar -czf "${artifact}" -C "${platform_build_root}" .
  fi

  printf '%s  %s\n' "$(sha256_file "${artifact}")" "$(basename "${artifact}")" >> "${OUT_DIR}/${CHECKSUMS_NAME}"
done

echo "[openbrain-server] artifacts:"
find "${OUT_DIR}" -maxdepth 1 -type f -print | sort
