#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
GBRAIN_SOURCE_ROOT="${OPENBRAIN_GBRAIN_SOURCE_ROOT:-${REPO_ROOT}/../gbrain}"
GBRAIN_RELEASE_TAG="${OPENBRAIN_GBRAIN_RELEASE_TAG:-}"
GBRAIN_RELEASE_BASE_URL="${OPENBRAIN_GBRAIN_RELEASE_BASE_URL:-}"
OUT_DIR="${OPENBRAIN_CLI_RELEASE_DIR:-${REPO_ROOT}/.tmp/openbrain-release-artifacts/cli}"

usage() {
  cat <<'EOF'
usage: scripts/openbrain/build-gbrain-cli-release.sh [platform...]

Build root gbrain CLI release binaries for OpenBrain releases.

Platforms:
  darwin-arm64
  linux-x64
  windows-x64

Environment:
  OPENBRAIN_CLI_RELEASE_DIR  Output directory.
  OPENBRAIN_GBRAIN_SOURCE_ROOT
                             GBrain fork checkout used when no release tag is set. Default: ../gbrain.
  OPENBRAIN_GBRAIN_RELEASE_TAG
                             Optional colinagent/gbrain release tag to download instead of building.
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
  PLATFORMS=(darwin-arm64 linux-x64 windows-x64)
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

target_for_platform() {
  case "$1" in
    darwin-arm64) echo "bun-darwin-arm64" ;;
    linux-x64) echo "bun-linux-x64" ;;
    windows-x64) echo "bun-windows-x64" ;;
    *) echo "unsupported gbrain CLI platform: $1" >&2; exit 1 ;;
  esac
}

artifact_for_platform() {
  case "$1" in
    darwin-arm64) echo "gbrain-darwin-arm64" ;;
    linux-x64) echo "gbrain-linux-x64" ;;
    windows-x64) echo "gbrain-windows-x64.exe" ;;
    *) echo "unsupported gbrain CLI platform: $1" >&2; exit 1 ;;
  esac
}

mkdir -p "${OUT_DIR}"

if [[ -n "${GBRAIN_RELEASE_TAG}" && -z "${GBRAIN_RELEASE_BASE_URL}" ]]; then
  GBRAIN_RELEASE_BASE_URL="https://github.com/colinagent/gbrain/releases/download/${GBRAIN_RELEASE_TAG}"
fi

for platform in "${PLATFORMS[@]}"; do
  target="$(target_for_platform "${platform}")"
  artifact="$(artifact_for_platform "${platform}")"
  out="${OUT_DIR}/${artifact}"

  if [[ -n "${GBRAIN_RELEASE_TAG}" ]]; then
    require_cmd curl
    url="${GBRAIN_RELEASE_BASE_URL%/}/${artifact}"
    checksum_tmp="${out}.sha256"
    echo "[gbrain-cli] platform=${platform} download=${url}"
    curl -fsSL --retry 3 --retry-delay 2 "${url}" -o "${out}"
    curl -fsSL --retry 3 --retry-delay 2 "${url}.sha256" -o "${checksum_tmp}"
    expected="$(awk '{print $1}' "${checksum_tmp}")"
    actual="$(sha256_file "${out}")"
    if [[ -z "${expected}" || "${expected}" != "${actual}" ]]; then
      echo "gbrain checksum mismatch for ${artifact}: expected ${expected:-<empty>} got ${actual}" >&2
      exit 1
    fi
  else
    require_cmd bun
    [[ -f "${GBRAIN_SOURCE_ROOT}/src/cli.ts" ]] || {
      echo "GBrain source not found: ${GBRAIN_SOURCE_ROOT}/src/cli.ts" >&2
      echo "Set OPENBRAIN_GBRAIN_SOURCE_ROOT or clone https://github.com/colinagent/gbrain next to this repo." >&2
      exit 1
    }
    echo "[gbrain-cli] platform=${platform} target=${target} source=${GBRAIN_SOURCE_ROOT} out=${out}"
    (
      cd "${GBRAIN_SOURCE_ROOT}"
      bun build --compile --target="${target}" --outfile "${out}" src/cli.ts
    )
  fi

  if [[ "${platform}" != windows-* ]]; then
    chmod +x "${out}" 2>/dev/null || true
  fi
done

echo "[gbrain-cli] artifacts:"
find "${OUT_DIR}" -maxdepth 1 -type f -print | sort
