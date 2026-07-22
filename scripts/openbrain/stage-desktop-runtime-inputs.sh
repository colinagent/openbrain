#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
DESKTOP_ROOT="${REPO_ROOT}/desktop"
VERSION="${2:-${OPENBRAIN_RELEASE_VERSION:-$(node -p "require('${DESKTOP_ROOT}/package.json').version")}}"
PLATFORM="${1:-}"
MANIFEST_URL="${OPENBRAIN_RUNTIME_MANIFEST_URL:-https://download.op-agent.com/runtime/latest/manifest.json}"

usage() {
  cat <<'EOF'
usage: scripts/openbrain/stage-desktop-runtime-inputs.sh <platform> [version]

Stage runtime bundle/bootstrap inputs required by openbrain-desktop packaging.
This script consumes runtime assets and stages them for OpenBrain desktop
packaging. Published runtime assets are read from the public download endpoint
by default.

Platforms:
  darwin-arm64
  linux-amd64
  windows-amd64

Environment:
  OPENBRAIN_RUNTIME_BUNDLE_PATH   Existing local bundle.tar.gz path.
  OPENBRAIN_BOOTSTRAP_PATH        Existing local bootstrap binary path.
  OPENBRAIN_RUNTIME_VERSION       Runtime version when staging from local files.
  OPENBRAIN_RUNTIME_MANIFEST_URL  Runtime manifest URL. Defaults to the public OpenBrain runtime manifest.
EOF
}

if [[ "${PLATFORM}" == "-h" || "${PLATFORM}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -z "${PLATFORM}" ]]; then
  usage >&2
  exit 1
fi

case "${PLATFORM}" in
  darwin-arm64|linux-amd64|windows-amd64) ;;
  *) echo "unsupported desktop runtime platform: ${PLATFORM}" >&2; exit 1 ;;
esac

BOOTSTRAP_NAME="openbrain-bootstrap"
if [[ "${PLATFORM}" == windows-* ]]; then
  BOOTSTRAP_NAME="openbrain-bootstrap.exe"
fi

STAGE_ROOT="${REPO_ROOT}/.tmp/openbrain-release/${VERSION}/${PLATFORM}"
BUNDLE_OUT="${STAGE_ROOT}/bundle.tar.gz"
BOOTSTRAP_OUT="${STAGE_ROOT}/stage/bin/${BOOTSTRAP_NAME}"
RUNTIME_VERSION_OUT="${STAGE_ROOT}/runtime-version.txt"

mkdir -p "$(dirname "${BUNDLE_OUT}")" "$(dirname "${BOOTSTRAP_OUT}")"

if [[ -n "${OPENBRAIN_RUNTIME_BUNDLE_PATH:-}" || -n "${OPENBRAIN_BOOTSTRAP_PATH:-}" ]]; then
  [[ -n "${OPENBRAIN_RUNTIME_BUNDLE_PATH:-}" ]] || { echo "OPENBRAIN_RUNTIME_BUNDLE_PATH is required when staging from local files" >&2; exit 1; }
  [[ -n "${OPENBRAIN_BOOTSTRAP_PATH:-}" ]] || { echo "OPENBRAIN_BOOTSTRAP_PATH is required when staging from local files" >&2; exit 1; }
  [[ -f "${OPENBRAIN_RUNTIME_BUNDLE_PATH}" ]] || { echo "missing runtime bundle: ${OPENBRAIN_RUNTIME_BUNDLE_PATH}" >&2; exit 1; }
  [[ -f "${OPENBRAIN_BOOTSTRAP_PATH}" ]] || { echo "missing runtime bootstrap: ${OPENBRAIN_BOOTSTRAP_PATH}" >&2; exit 1; }

  cp "${OPENBRAIN_RUNTIME_BUNDLE_PATH}" "${BUNDLE_OUT}"
  cp "${OPENBRAIN_BOOTSTRAP_PATH}" "${BOOTSTRAP_OUT}"
  printf '%s\n' "${OPENBRAIN_RUNTIME_VERSION:-${VERSION}}" > "${RUNTIME_VERSION_OUT}"
else
  node - "${MANIFEST_URL}" "${PLATFORM}" "${BUNDLE_OUT}" "${BOOTSTRAP_OUT}" "${RUNTIME_VERSION_OUT}" <<'NODE'
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { pipeline } = require('node:stream/promises');

const [manifestUrl, platform, bundleOut, bootstrapOut, runtimeVersionOut] = process.argv.slice(2);

async function fetchJson(url) {
  const res = await fetch(url, { headers: { Accept: 'application/json' } });
  if (!res.ok) throw new Error(`failed to fetch ${url}: ${res.status} ${res.statusText}`);
  return await res.json();
}

async function download(url, outPath, expectedSha256) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`failed to download ${url}: ${res.status} ${res.statusText}`);
  if (!res.body) throw new Error(`download has no body: ${url}`);

  fs.mkdirSync(path.dirname(outPath), { recursive: true });
  const hash = crypto.createHash('sha256');
  const writer = fs.createWriteStream(outPath);
  const transform = new TransformStream({
    transform(chunk, controller) {
      hash.update(Buffer.from(chunk));
      controller.enqueue(chunk);
    },
  });
  try {
    await pipeline(res.body.pipeThrough(transform), writer);
  } catch (err) {
    fs.rmSync(outPath, { force: true });
    throw err;
  }
  const actual = hash.digest('hex');
  if (actual.toLowerCase() !== String(expectedSha256 || '').toLowerCase()) {
    fs.rmSync(outPath, { force: true });
    throw new Error(`sha256 mismatch for ${url}: expected ${expectedSha256}, got ${actual}`);
  }
}

(async () => {
  const manifest = await fetchJson(manifestUrl);
  const entry = manifest?.assets?.[platform];
  if (!entry?.bundle?.url || !entry?.bundle?.sha256) {
    throw new Error(`runtime manifest missing bundle for ${platform}`);
  }
  if (!entry?.bootstrap?.url || !entry?.bootstrap?.sha256) {
    throw new Error(`runtime manifest missing bootstrap for ${platform}`);
  }
  if (!manifest?.version) {
    throw new Error('runtime manifest missing version');
  }

  await download(entry.bundle.url, bundleOut, entry.bundle.sha256);
  await download(entry.bootstrap.url, bootstrapOut, entry.bootstrap.sha256);
  fs.writeFileSync(runtimeVersionOut, `${String(manifest.version).trim()}\n`);
})().catch((err) => {
  console.error(err instanceof Error ? err.message : String(err));
  process.exit(1);
});
NODE
fi

chmod +x "${BOOTSTRAP_OUT}" 2>/dev/null || true

echo "[desktop-runtime] staged runtime inputs"
echo "  platform: ${PLATFORM}"
echo "  version: ${VERSION}"
echo "  runtime version: $(cat "${RUNTIME_VERSION_OUT}")"
echo "  bundle: ${BUNDLE_OUT}"
echo "  bootstrap: ${BOOTSTRAP_OUT}"
