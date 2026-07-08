#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${APP_ROOT}/.." && pwd)"
OPENBRAIN_STAGE="${APP_ROOT}/openbrain"
PLATFORM="${1:-${OPENBRAIN_DESKTOP_BUNDLE_PLATFORM:-darwin-arm64}}"
desktop_package_version() {
  (cd "${APP_ROOT}" && node -p "require('./package.json').version")
}

RELEASE_VERSION="${OPENBRAIN_DESKTOP_BUNDLE_VERSION:-$(desktop_package_version)}"
RUNTIME_RELEASE_DIR="${REPO_ROOT}/.tmp/openbrain-release/${RELEASE_VERSION}/${PLATFORM}"
RUNTIME_BUNDLE_PATH="${OPENBRAIN_RUNTIME_BUNDLE_PATH:-${RUNTIME_RELEASE_DIR}/bundle.tar.gz}"
RUNTIME_VERSION_FILE="${RUNTIME_RELEASE_DIR}/runtime-version.txt"
RUNTIME_VERSION="${OPENBRAIN_RUNTIME_VERSION:-}"
BOOTSTRAP_NAME="openbrain-bootstrap"
if [[ "${PLATFORM}" == windows-* ]]; then
  BOOTSTRAP_NAME="openbrain-bootstrap.exe"
fi
BOOTSTRAP_PATH="${OPENBRAIN_BOOTSTRAP_PATH:-${RUNTIME_RELEASE_DIR}/stage/bin/${BOOTSTRAP_NAME}}"
BOOTSTRAP_ASSET_PATH="${RUNTIME_RELEASE_DIR}/${BOOTSTRAP_NAME}"

rm -rf "${OPENBRAIN_STAGE}" "${APP_ROOT}/agents"
mkdir -p "${OPENBRAIN_STAGE}/bin" "${OPENBRAIN_STAGE}/bundles/${PLATFORM}"

if [[ ! -f "${RUNTIME_BUNDLE_PATH}" ]]; then
  echo "[desktop bundle] missing runtime bundle: ${RUNTIME_BUNDLE_PATH}" >&2
  echo "[desktop bundle] provide OPENBRAIN_RUNTIME_BUNDLE_PATH or stage ${RUNTIME_RELEASE_DIR}/bundle.tar.gz" >&2
  exit 1
fi
if [[ ! -f "${BOOTSTRAP_PATH}" && -f "${BOOTSTRAP_ASSET_PATH}" ]]; then
  BOOTSTRAP_PATH="${BOOTSTRAP_ASSET_PATH}"
fi
if [[ ! -f "${BOOTSTRAP_PATH}" ]]; then
  echo "[desktop bundle] missing runtime bootstrapper: ${BOOTSTRAP_PATH}" >&2
  echo "[desktop bundle] provide OPENBRAIN_BOOTSTRAP_PATH or stage ${RUNTIME_RELEASE_DIR}/${BOOTSTRAP_NAME}" >&2
  exit 1
fi
if [[ -z "${RUNTIME_VERSION}" && -f "${RUNTIME_VERSION_FILE}" ]]; then
  RUNTIME_VERSION="$(tr -d '\r\n' < "${RUNTIME_VERSION_FILE}")"
fi
if [[ -z "${RUNTIME_VERSION}" ]]; then
  RUNTIME_VERSION="${RELEASE_VERSION}"
fi

cp "${BOOTSTRAP_PATH}" "${OPENBRAIN_STAGE}/bin/${BOOTSTRAP_NAME}"
cp "${RUNTIME_BUNDLE_PATH}" "${OPENBRAIN_STAGE}/bundles/${PLATFORM}/bundle.tar.gz"
printf '%s\n' "${RUNTIME_VERSION}" > "${OPENBRAIN_STAGE}/runtime-version.txt"
chmod +x "${OPENBRAIN_STAGE}/bin/${BOOTSTRAP_NAME}" 2>/dev/null || true

echo "Built packaged desktop resources"
echo "  bootstrap: ${OPENBRAIN_STAGE}/bin/${BOOTSTRAP_NAME}"
echo "  release version: ${RELEASE_VERSION}"
echo "  runtime version: ${RUNTIME_VERSION}"
echo "  runtime bundle: ${OPENBRAIN_STAGE}/bundles/${PLATFORM}/bundle.tar.gz"
echo "  settings: ${APP_ROOT}/settings"
