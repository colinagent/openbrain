#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"

if ! command -v rg >/dev/null 2>&1; then
  echo "Missing command: rg" >&2
  exit 1
fi

cd "${REPO_ROOT}"

desktop_auth_paths=(
  desktop/src/main/auth
  desktop/src/main/main.ts
  desktop/src/main/preload.ts
  desktop/src/renderer/store/authStore.ts
  desktop/src/renderer/components/Sidebar/OpenBrainSidebar.tsx
  desktop/electron-builder.yml
)

failed=0
scan_args=(
  -n
  -S
  --glob
  '!*.test.*'
  --glob
  '!*.source.test.*'
)

if matches="$(rg "${scan_args[@]}" -e '[.]opagent' "${desktop_auth_paths[@]}")"; then
  echo "Desktop auth must not read legacy ~/.opagent state:" >&2
  echo "${matches}" >&2
  failed=1
fi

if matches="$(rg "${scan_args[@]}" -e 'opagent://auth/callback' "${desktop_auth_paths[@]}")"; then
  echo "Desktop auth must not accept legacy opagent:// callbacks:" >&2
  echo "${matches}" >&2
  failed=1
fi

if matches="$(rg "${scan_args[@]}" -e "const PROTOCOL_NAMES = \\['openbrain', 'opagent'\\]" desktop/src/main/main.ts)"; then
  echo "Desktop protocol registration must only include openbrain:" >&2
  echo "${matches}" >&2
  failed=1
fi

if rg "${scan_args[@]}" -e 'invalidateAuthSessionForOpenBrainResponse' desktop/src/main >/dev/null; then
  echo "OpenBrain Cloud business auth failures must not clear global desktop auth." >&2
  failed=1
fi

if [[ "${failed}" -ne 0 ]]; then
  echo "Desktop auth boundary check failed." >&2
  exit 1
fi

echo "Desktop auth boundary check passed."
