#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
GBRAIN_SOURCE_ROOT="${OPENBRAIN_GBRAIN_SOURCE_ROOT:-${REPO_ROOT}/../gbrain}"
GBRAIN_RELEASE_TAG="${OPENBRAIN_GBRAIN_RELEASE_TAG:-}"
GBRAIN_RELEASE_BASE_URL="${OPENBRAIN_GBRAIN_RELEASE_BASE_URL:-}"
OUT_DIR="${OPENBRAIN_RUNTIME_RELEASE_DIR:-${REPO_ROOT}/.tmp/openbrain-release-assets/runtime}"
BUILD_ROOT="${OPENBRAIN_RUNTIME_BUILD_ROOT:-${REPO_ROOT}/.tmp/openbrain-runtime-build}"
CHECKSUMS_NAME="runtime-SHA256SUMS"
VERSION="${OPENBRAIN_RELEASE_VERSION:-${GITHUB_REF_NAME:-dev}}"
VERSION="${VERSION#openbrain-v}"
TAG="${OPENBRAIN_RELEASE_TAG:-openbrain-v${VERSION}}"
RELEASE_BASE_URL="${OPENBRAIN_RELEASE_BASE_URL:-https://github.com/colinagent/openbrain/releases/download/${TAG}}"

usage() {
  cat <<'EOF'
usage: scripts/openbrain/build-runtime-release.sh [platform...]

Build OpenBrain runtime release assets for GitHub Releases.

Platforms:
  darwin-arm64
  darwin-amd64
  linux-amd64
  windows-amd64

Environment:
  OPENBRAIN_RELEASE_VERSION       Release version without the openbrain-v prefix.
  OPENBRAIN_RELEASE_TAG           Release tag. Defaults to openbrain-v$OPENBRAIN_RELEASE_VERSION.
  OPENBRAIN_RELEASE_BASE_URL      Base URL used in runtime-manifest.json.
  OPENBRAIN_RUNTIME_RELEASE_DIR   Output directory.
  OPENBRAIN_RUNTIME_BUILD_ROOT    Temporary build root.
  OPENBRAIN_RIPGREP_VERSION       ripgrep release version. Defaults to 15.1.0.
  OPENBRAIN_GBRAIN_SOURCE_ROOT    GBrain fork checkout used when no release tag is set. Default: ../gbrain.
  OPENBRAIN_GBRAIN_RELEASE_TAG    Optional colinagent/gbrain release tag to download instead of building.
  OPENBRAIN_GBRAIN_RELEASE_BASE_URL
                                  Optional GBrain release asset base URL. Defaults to GitHub release URL for the tag.
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

goos_for_platform() {
  case "$1" in
    darwin-*) echo "darwin" ;;
    linux-*) echo "linux" ;;
    windows-*) echo "windows" ;;
    *) echo "unsupported runtime platform: $1" >&2; exit 1 ;;
  esac
}

goarch_for_platform() {
  case "$1" in
    *-amd64) echo "amd64" ;;
    *-arm64) echo "arm64" ;;
    *) echo "unsupported runtime platform: $1" >&2; exit 1 ;;
  esac
}

exe_suffix_for_platform() {
  case "$1" in
    windows-*) echo ".exe" ;;
    *) echo "" ;;
  esac
}

bun_target_for_platform() {
  case "$1" in
    darwin-arm64) echo "bun-darwin-arm64" ;;
    darwin-amd64) echo "bun-darwin-x64" ;;
    linux-amd64) echo "bun-linux-x64" ;;
    windows-amd64) echo "bun-windows-x64" ;;
    *) echo "unsupported gbrain platform: $1" >&2; exit 1 ;;
  esac
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
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
  local asset
  asset="$(ripgrep_asset_name "${version}" "${platform}")"
  local url="https://github.com/BurntSushi/ripgrep/releases/download/${version}/${asset}"
  local tmp_dir archive source
  tmp_dir="$(mktemp -d)"
  archive="${tmp_dir}/${asset}"
  echo "[openbrain-runtime] ripgrep ${platform}: ${asset}"
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

write_manifest() {
  local manifest_path="$1"
  shift

  python3 - "$manifest_path" "$VERSION" "$RELEASE_BASE_URL" "$@" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

manifest_path = Path(sys.argv[1])
version = sys.argv[2]
base_url = sys.argv[3].rstrip("/")
entries = sys.argv[4:]

assets = {}
for entry in entries:
    platform, bundle_name, bundle_sha, bootstrap_name, bootstrap_sha = entry.split("|", 4)
    assets[platform] = {
        "bundle": {
            "url": f"{base_url}/{bundle_name}",
            "sha256": bundle_sha,
        },
        "bootstrap": {
            "url": f"{base_url}/{bootstrap_name}",
            "sha256": bootstrap_sha,
        },
    }

payload = {
    "version": version,
    "generatedAt": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "assets": assets,
}
manifest_path.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

require_cmd go
require_cmd tar
require_cmd python3
require_cmd curl

case "${VERSION}" in
  ""|dev)
    echo "OPENBRAIN_RELEASE_VERSION is required for runtime release builds" >&2
    exit 1
    ;;
esac

mkdir -p "${OUT_DIR}" "${BUILD_ROOT}"
rm -f "${OUT_DIR}/runtime-manifest.json" "${OUT_DIR}/SHA256SUMS" "${OUT_DIR}/${CHECKSUMS_NAME}"
: > "${OUT_DIR}/${CHECKSUMS_NAME}"

if [[ -n "${GBRAIN_RELEASE_TAG}" && -z "${GBRAIN_RELEASE_BASE_URL}" ]]; then
  GBRAIN_RELEASE_BASE_URL="https://github.com/colinagent/gbrain/releases/download/${GBRAIN_RELEASE_TAG}"
fi

gbrain_asset_for_platform() {
  case "$1" in
    darwin-arm64) echo "gbrain-darwin-arm64" ;;
    darwin-amd64) echo "gbrain-darwin-x64" ;;
    linux-amd64) echo "gbrain-linux-x64" ;;
    windows-amd64) echo "gbrain-windows-x64.exe" ;;
    *) echo "unsupported gbrain platform: $1" >&2; exit 1 ;;
  esac
}

stage_gbrain_binary() {
  local platform="$1"
  local bun_target="$2"
  local out_path="$3"
  local asset expected actual checksum_tmp url

  mkdir -p "$(dirname "${out_path}")"
  if [[ -n "${GBRAIN_RELEASE_TAG}" ]]; then
    asset="$(gbrain_asset_for_platform "${platform}")"
    url="${GBRAIN_RELEASE_BASE_URL%/}/${asset}"
    checksum_tmp="${out_path}.sha256"
    echo "[openbrain-runtime] gbrain ${platform}: download ${url}"
    curl -fsSL --retry 3 --retry-delay 2 "${url}" -o "${out_path}"
    curl -fsSL --retry 3 --retry-delay 2 "${url}.sha256" -o "${checksum_tmp}"
    expected="$(awk '{print $1}' "${checksum_tmp}")"
    actual="$(sha256_file "${out_path}")"
    if [[ -z "${expected}" || "${expected}" != "${actual}" ]]; then
      echo "gbrain checksum mismatch for ${asset}: expected ${expected:-<empty>} got ${actual}" >&2
      exit 1
    fi
  else
    require_cmd bun
    [[ -f "${GBRAIN_SOURCE_ROOT}/src/cli.ts" ]] || {
      echo "GBrain source not found: ${GBRAIN_SOURCE_ROOT}/src/cli.ts" >&2
      echo "Set OPENBRAIN_GBRAIN_SOURCE_ROOT or clone https://github.com/colinagent/gbrain next to this repo." >&2
      exit 1
    }
    echo "[openbrain-runtime] gbrain ${platform}: build from ${GBRAIN_SOURCE_ROOT}"
    (
      cd "${GBRAIN_SOURCE_ROOT}"
      bun build --compile --target="${bun_target}" --outfile "${out_path}" src/cli.ts
    )
  fi
}

manifest_entries=()

for platform in "${PLATFORMS[@]}"; do
  case "${platform}" in
    darwin-arm64|darwin-amd64|linux-amd64|windows-amd64) ;;
    *) echo "unsupported runtime platform: ${platform}" >&2; exit 1 ;;
  esac

  goos="$(goos_for_platform "${platform}")"
  goarch="$(goarch_for_platform "${platform}")"
  exe_suffix="$(exe_suffix_for_platform "${platform}")"
  bun_target="$(bun_target_for_platform "${platform}")"
  gbrain_binary_name="gbrain${exe_suffix}"
  platform_root="${BUILD_ROOT}/${platform}"
  stage_root="${platform_root}/stage"
  overlay_root="${platform_root}/overlay"
  bundle_name="runtime-bundle-${platform}.tar.gz"
  bundle_path="${OUT_DIR}/${bundle_name}"
  bootstrap_asset_name="openbrain-bootstrap-${platform}${exe_suffix}"
  bootstrap_asset_path="${OUT_DIR}/${bootstrap_asset_name}"

  rm -rf "${platform_root}" "${bundle_path}" "${bootstrap_asset_path}"
  mkdir -p \
    "${stage_root}/bin" \
    "${overlay_root}/agents/opagent-server/.agent/bin" \
    "${overlay_root}/agents/coder/.agent/bin" \
    "${overlay_root}/skills/openbrain-cloud-sync/bin" \
    "${overlay_root}/tools/rg-search/bin" \
    "${stage_root}/configs"

  echo "[openbrain-runtime] platform=${platform} version=${VERSION}"
  (
    cd "${REPO_ROOT}"
    export CGO_ENABLED=0
    export GOOS="${goos}"
    export GOARCH="${goarch}"
    export GOWORK="${REPO_ROOT}/go.work"
    go build -trimpath -ldflags "-s -w" -o "${stage_root}/bin/opagent-runtime${exe_suffix}" ./opagent-runtime/cmd/opagent-runtime
    go build -trimpath -ldflags "-s -w" -o "${stage_root}/bin/opagent-bootstrap${exe_suffix}" ./opagent-runtime/cmd/opagent-bootstrap
    go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o "${overlay_root}/agents/opagent-server/.agent/bin/openbrain-server${exe_suffix}" ./server/cmd/openbrain-server
    go build -trimpath -ldflags "-s -w" -o "${overlay_root}/agents/coder/.agent/bin/coder${exe_suffix}" ./agents/coder/cmd/coder
    go build -trimpath -ldflags "-s -w" -o "${overlay_root}/skills/openbrain-cloud-sync/bin/openbrain-cloud-sync-helper${exe_suffix}" ./skills/openbrain-cloud-sync/cmd/openbrain-cloud-sync-helper
  )
  stage_gbrain_binary "${platform}" "${bun_target}" "${stage_root}/bin/${gbrain_binary_name}"
  download_ripgrep "${platform}" "${overlay_root}/tools/rg-search/bin/rg${exe_suffix}"

  cp "${stage_root}/bin/opagent-bootstrap${exe_suffix}" "${bootstrap_asset_path}"
  python3 "${REPO_ROOT}/scripts/openbrain/build-builtin-release-assets.py" stage-runtime \
    --repo-root "${REPO_ROOT}" \
    --stage-root "${stage_root}" \
    --overlay-root "${overlay_root}" \
    --platform "${platform}"
  cp "${REPO_ROOT}/opagent-runtime/configs/config.json" "${stage_root}/configs/config.json"

  if [[ "${platform}" != windows-* ]]; then
    chmod +x \
      "${stage_root}/bin/opagent-runtime${exe_suffix}" \
      "${stage_root}/bin/opagent-bootstrap${exe_suffix}" \
      "${stage_root}/bin/${gbrain_binary_name}" \
      "${stage_root}/agents/opagent-server/.agent/bin/openbrain-server${exe_suffix}" \
      "${stage_root}/agents/coder/.agent/bin/coder${exe_suffix}" \
      "${stage_root}/tools/rg-search/bin/rg${exe_suffix}" \
      "${stage_root}/skills/openbrain-cloud-sync/bin/openbrain-cloud-sync-helper${exe_suffix}" \
      "${bootstrap_asset_path}" 2>/dev/null || true
  fi

  tar -czf "${bundle_path}" -C "${stage_root}" .

  bundle_sha="$(sha256_file "${bundle_path}")"
  bootstrap_sha="$(sha256_file "${bootstrap_asset_path}")"
  printf '%s  %s\n' "${bundle_sha}" "${bundle_name}" >> "${OUT_DIR}/${CHECKSUMS_NAME}"
  printf '%s  %s\n' "${bootstrap_sha}" "${bootstrap_asset_name}" >> "${OUT_DIR}/${CHECKSUMS_NAME}"
  manifest_entries+=("${platform}|${bundle_name}|${bundle_sha}|${bootstrap_asset_name}|${bootstrap_sha}")
done

write_manifest "${OUT_DIR}/runtime-manifest.json" "${manifest_entries[@]}"
printf '%s  %s\n' "$(sha256_file "${OUT_DIR}/runtime-manifest.json")" "runtime-manifest.json" >> "${OUT_DIR}/${CHECKSUMS_NAME}"

echo "[openbrain-runtime] artifacts:"
find "${OUT_DIR}" -maxdepth 1 -type f -print | sort
