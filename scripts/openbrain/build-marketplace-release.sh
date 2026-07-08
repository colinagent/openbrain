#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
OUT_DIR="${OPENBRAIN_MARKETPLACE_RELEASE_DIR:-${REPO_ROOT}/.tmp/openbrain-release-assets/marketplace}"
BUILD_ROOT="${OPENBRAIN_MARKETPLACE_BUILD_ROOT:-${REPO_ROOT}/.tmp/openbrain-marketplace-build}"
VERSION="${OPENBRAIN_RELEASE_VERSION:-${GITHUB_REF_NAME:-dev}}"
VERSION="${VERSION#openbrain-v}"
BASE_URL="${OPENBRAIN_MARKETPLACE_BASE_URL:-https://download.op-agent.com/marketplace}"

usage() {
  cat <<'EOF'
usage: scripts/openbrain/build-marketplace-release.sh

Build OpenBrain marketplace catalog assets from this repository's public source.

Environment:
  OPENBRAIN_RELEASE_VERSION          Release/catalog version.
  OPENBRAIN_MARKETPLACE_RELEASE_DIR  Output directory.
  OPENBRAIN_MARKETPLACE_BUILD_ROOT   Temporary build root.
  OPENBRAIN_MARKETPLACE_BASE_URL     Public marketplace base URL.
  OPENBRAIN_RIPGREP_VERSION          ripgrep release version. Defaults to 15.1.0.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

case "${VERSION}" in
  ""|dev)
    echo "OPENBRAIN_RELEASE_VERSION is required for marketplace release builds" >&2
    exit 1
    ;;
esac

require_file() {
  if [[ ! -f "$1" ]]; then
    echo "required file not found: $1" >&2
    exit 1
  fi
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "Missing command: $1" >&2; exit 1; }
}

goos_for_platform() {
  case "$1" in
    darwin-*) echo "darwin" ;;
    linux-*) echo "linux" ;;
    windows-*) echo "windows" ;;
    *) echo "unsupported marketplace platform: $1" >&2; exit 1 ;;
  esac
}

goarch_for_platform() {
  case "$1" in
    *-amd64) echo "amd64" ;;
    *-arm64) echo "arm64" ;;
    *) echo "unsupported marketplace platform: $1" >&2; exit 1 ;;
  esac
}

exe_suffix_for_platform() {
  case "$1" in
    windows-*) echo ".exe" ;;
    *) echo "" ;;
  esac
}

ripgrep_asset_name() {
  local version="$1"
  local platform="$2"
  case "${platform}" in
    darwin-arm64) printf 'ripgrep-%s-aarch64-apple-darwin.tar.gz' "${version}" ;;
    darwin-amd64) printf 'ripgrep-%s-x86_64-apple-darwin.tar.gz' "${version}" ;;
    linux-amd64) printf 'ripgrep-%s-x86_64-unknown-linux-musl.tar.gz' "${version}" ;;
    windows-amd64) printf 'ripgrep-%s-x86_64-pc-windows-msvc.zip' "${version}" ;;
    *) echo "unsupported ripgrep platform: ${platform}" >&2; exit 1 ;;
  esac
}

download_ripgrep() {
  local platform="$1"
  local out_path="$2"
  local version="${OPENBRAIN_RIPGREP_VERSION:-15.1.0}"
  local asset url tmp_dir archive source
  asset="$(ripgrep_asset_name "${version}" "${platform}")"
  url="https://github.com/BurntSushi/ripgrep/releases/download/${version}/${asset}"
  tmp_dir="$(mktemp -d)"
  archive="${tmp_dir}/${asset}"
  echo "[openbrain-marketplace] ripgrep ${platform}: ${asset}"
  curl -fsSL --retry 3 --retry-delay 2 "${url}" -o "${archive}"
  case "${asset}" in
    *.zip)
      python3 - "${archive}" "${tmp_dir}/x" <<'PY'
import sys
import zipfile
from pathlib import Path

archive = Path(sys.argv[1])
out = Path(sys.argv[2])
out.mkdir(parents=True, exist_ok=True)
with zipfile.ZipFile(archive) as zf:
    zf.extractall(out)
PY
      source="$(find "${tmp_dir}/x" -type f -name 'rg.exe' -print -quit)"
      ;;
    *.tar.gz)
      mkdir -p "${tmp_dir}/x"
      tar -xzf "${archive}" -C "${tmp_dir}/x"
      source="$(find "${tmp_dir}/x" -type f -name 'rg' -print -quit)"
      ;;
    *) echo "unsupported ripgrep archive: ${asset}" >&2; rm -rf "${tmp_dir}"; exit 1 ;;
  esac
  if [[ -z "${source}" || ! -f "${source}" ]]; then
    echo "ripgrep binary not found in ${asset}" >&2
    rm -rf "${tmp_dir}"
    exit 1
  fi
  mkdir -p "$(dirname "${out_path}")"
  cp "${source}" "${out_path}"
  chmod +x "${out_path}" 2>/dev/null || true
  rm -rf "${tmp_dir}"
}

require_cmd go
require_cmd python3
require_cmd tar
require_cmd curl
require_file "${REPO_ROOT}/scripts/openbrain/build-builtin-release-assets.py"

PLATFORMS=(darwin-arm64 darwin-amd64 linux-amd64 windows-amd64)
OVERLAY_BASE="${BUILD_ROOT}/overlays"

rm -rf "${BUILD_ROOT}"
mkdir -p "${OUT_DIR}" "${OVERLAY_BASE}"

for platform in "${PLATFORMS[@]}"; do
  goos="$(goos_for_platform "${platform}")"
  goarch="$(goarch_for_platform "${platform}")"
  exe_suffix="$(exe_suffix_for_platform "${platform}")"
  overlay_root="${OVERLAY_BASE}/${platform}"
  mkdir -p \
    "${overlay_root}/agents/opagent-server/.agent/bin" \
    "${overlay_root}/agents/coder/.agent/bin" \
    "${overlay_root}/skills/openbrain-cloud-sync/bin" \
    "${overlay_root}/tools/rg-search/bin"

  echo "[openbrain-marketplace] platform=${platform}"
  (
    cd "${REPO_ROOT}"
    export CGO_ENABLED=0
    export GOOS="${goos}"
    export GOARCH="${goarch}"
    export GOWORK="${REPO_ROOT}/go.work"
    go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o "${overlay_root}/agents/opagent-server/.agent/bin/openbrain-server${exe_suffix}" ./server/cmd/openbrain-server
    go build -trimpath -ldflags "-s -w" -o "${overlay_root}/agents/coder/.agent/bin/coder${exe_suffix}" ./agents/coder/cmd/coder
    go build -trimpath -ldflags "-s -w" -o "${overlay_root}/skills/openbrain-cloud-sync/bin/openbrain-cloud-sync-helper${exe_suffix}" ./skills/openbrain-cloud-sync/cmd/openbrain-cloud-sync-helper
  )
  download_ripgrep "${platform}" "${overlay_root}/tools/rg-search/bin/rg${exe_suffix}"
done

python3 "${REPO_ROOT}/scripts/openbrain/build-builtin-release-assets.py" build-marketplace \
  --repo-root "${REPO_ROOT}" \
  --out-dir "${OUT_DIR}" \
  --version "${VERSION}" \
  --base-url "${BASE_URL%/}" \
  --platforms "$(IFS=,; echo "${PLATFORMS[*]}")" \
  --overlay-base "${OVERLAY_BASE}"
