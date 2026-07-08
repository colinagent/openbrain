#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-artifacts}"

if [[ ! -d "${ROOT}" ]]; then
  echo "desktop update metadata root not found: ${ROOT}" >&2
  exit 1
fi

python3 - "${ROOT}" <<'PY'
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
errors = []

def referenced_names(path: Path) -> list[str]:
    names = []
    for line in path.read_text(encoding="utf-8").splitlines():
        match = re.match(r"\s*(?:-\s*)?url:\s*(.+?)\s*$", line)
        if not match:
            match = re.match(r"\s*path:\s*(.+?)\s*$", line)
        if match:
            value = match.group(1).strip().strip("'\"")
            if value and "://" not in value:
                names.append(value.split("?", 1)[0].split("#", 1)[0])
    return names

for metadata in sorted(root.rglob("latest*.yml")):
    names = referenced_names(metadata)
    for name in names:
        if not (metadata.parent / name).is_file():
            errors.append(f"{metadata}: references missing file {name}")

if errors:
    raise SystemExit("\n".join(errors))
PY
