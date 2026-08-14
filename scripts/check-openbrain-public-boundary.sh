#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/.." && pwd)"
cd "${repo_root}"

command -v rg >/dev/null 2>&1 || { echo "Missing command: rg" >&2; exit 1; }

forbidden_paths=(
  AGENTS.md
  opagent-runtime
  opagent-protocol
  openbrain-server
  server
  desktop
  apps/openbrain-desktop
  apps/openbrain-ios
  agents/coder
  agents/simple-memory
  skills/word-document
  tools/rg-search
  tools/systool
)
for path in "${forbidden_paths[@]}"; do
  if [[ -e "${path}" ]]; then
    echo "forbidden OpenBrain public path: ${path}" >&2
    exit 1
  fi
done

scan_args=(
  --hidden
  --glob '!.git/**'
  --glob '!.tmp/**'
  --glob '!**/node_modules/**'
  --glob '!**/dist/**'
  --glob '!scripts/check-openbrain-public-boundary.sh'
)

patterns=(
  'github[.]com/colinagent/openbrain/opagent-(runtime|protocol)'
  'github[.]com/op-agent/OpAgent'
  'OPENBRAIN_BASE_DIR'
  'OPENBRAIN_HOME'
  'OP_HOME'
  '[.]openbrain'
  'api[.]opagent[.]chat'
  'api[.]openbrain[.]chat'
  'download[.]openbrain[.]io'
  'resource[.]op-agent[.]com'
  'OPENBRAIN_OSS_'
  'AWS_SECRET_ACCESS_KEY'
  'openbrain-dev'
  '/Users/colin'
  'ssh dev'
)

failed=0
for pattern in "${patterns[@]}"; do
  if matches="$(rg -n -i -S -e "${pattern}" "${scan_args[@]}" .)"; then
    echo "forbidden public-boundary match: ${pattern}" >&2
    echo "${matches}" >&2
    failed=1
  fi
done

if [[ "${failed}" -ne 0 ]]; then
  echo "OpenBrain public-boundary check failed." >&2
  exit 1
fi

echo "OpenBrain public-boundary check passed."
