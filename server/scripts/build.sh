#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PLATFORM="${1:-${OPENBRAIN_SERVER_PLATFORM:-$(go env GOOS)-$(go env GOARCH)}}"
VERSION="${VERSION:-${OPENBRAIN_SERVER_VERSION:-$(git -C "${APP_ROOT}" describe --tags --always --dirty 2>/dev/null || echo dev)}}"

case "${PLATFORM}" in
  darwin-arm64)
    GOOS_VALUE="darwin"
    GOARCH_VALUE="arm64"
    EXE=""
    ;;
  darwin-amd64)
    GOOS_VALUE="darwin"
    GOARCH_VALUE="amd64"
    EXE=""
    ;;
  linux-amd64)
    GOOS_VALUE="linux"
    GOARCH_VALUE="amd64"
    EXE=""
    ;;
  windows-amd64)
    GOOS_VALUE="windows"
    GOARCH_VALUE="amd64"
    EXE=".exe"
    ;;
  *)
    echo "unsupported platform: ${PLATFORM}" >&2
    echo "supported: darwin-arm64 darwin-amd64 linux-amd64 windows-amd64" >&2
    exit 1
    ;;
esac

OUT_ROOT="${OPENBRAIN_SERVER_DIST_ROOT:-${APP_ROOT}/dist/${PLATFORM}}"
OUT_DIR="${OUT_ROOT}/agents/openbrain-server/bin"
OUT_PATH="${OUT_DIR}/openbrain-server${EXE}"

mkdir -p "${OUT_DIR}"

(
  cd "${APP_ROOT}"
  GOOS="${GOOS_VALUE}" GOARCH="${GOARCH_VALUE}" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" \
    -o "${OUT_PATH}" ./cmd/openbrain-server
)

chmod +x "${OUT_PATH}" 2>/dev/null || true
echo "Built ${OUT_PATH}"
