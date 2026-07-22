#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

if ! command -v rg >/dev/null 2>&1; then
  echo "Missing command: rg" >&2
  exit 1
fi

cd "${REPO_ROOT}"

scan_args=(
  --hidden
  --glob '!.git/**'
  --glob '!.tmp/**'
  --glob '!**/node_modules/**'
  --glob '!**/dist/**'
  --glob '!desktop/release/**'
  --glob '!scripts/check-openbrain-public-boundary.sh'
)

global_patterns=(
  'download[.]openbrain[.]io'
  'resource[.]op-agent[.]com'
  'opagent-download[.]oss-[a-z0-9-]+[.]aliyuncs[.]com'
  'OPENBRAIN_OSS_'
  'AWS_SECRET_ACCESS_KEY'
  'api[.]opagent[.]chat'
  'api[.]openbrain[.]chat'
  '/gbrain/mcp'
  'openbrain-dev'
  '/Users/colin'
  'io[.]openbrain[.]ios'
  'OpenBrainMobile'
)

openbrain_patterns=(
  'alidns'
  'openbrain-manager'
  'manager[_ -]?token'
  'tailnet'
  'tailscale'
)

failed=0
for pattern in \
  '^AGENTS[.]md$' \
  '^[.]agent/' \
  '^[.]claude/' \
  '^[.]context/' \
  '^gbrain/' \
  '^mobile/ios/'
do
  if matches="$(git ls-files | rg -n -e "${pattern}")"; then
    echo "Forbidden public tracked path: ${pattern}" >&2
    echo "${matches}" >&2
    failed=1
  fi
done

for pattern in "${global_patterns[@]}"; do
  if matches="$(rg -n -i -S -e "${pattern}" "${scan_args[@]}" .)"; then
    echo "Forbidden public-boundary match: ${pattern}" >&2
    echo "${matches}" >&2
    failed=1
  fi
done

openbrain_paths=()
for path in README.md .github docs desktop opagent-runtime server scripts opagent-protocol agents tools skills; do
  [[ -e "${path}" ]] && openbrain_paths+=("${path}")
done
for pattern in "${openbrain_patterns[@]}"; do
  if matches="$(rg -n -i -S -e "${pattern}" "${scan_args[@]}" "${openbrain_paths[@]}")"; then
    echo "Forbidden OpenBrain public-boundary match: ${pattern}" >&2
    echo "${matches}" >&2
    failed=1
  fi
done

if [[ "${failed}" -ne 0 ]]; then
  echo "OpenBrain public-boundary check failed." >&2
  exit 1
fi

echo "OpenBrain public-boundary check passed."
