---
id: tools-rg-search
name: rg-search
description: Managed ripgrep binary used by OpenBrain search and agent shell commands.
tags:
  - builtin
  - system
---

OpenBrain packages ripgrep as a managed system binary. The runtime projects it
from this tool's `bin/` directory to the user-level OpenBrain bin directory:

- macOS/Linux release packages provide `bin/rg`, which is installed as
  `~/.openbrain/bin/rg`.
- Windows release packages provide `bin/rg.exe`, which is installed as
  `%USERPROFILE%\.openbrain\bin\rg.exe`.

OpenBrain search and agent shell commands can then run `rg` without requiring a
separate ripgrep installation.
