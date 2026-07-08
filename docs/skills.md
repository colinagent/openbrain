# OpenBrain Skills

This document is the public design source for OpenBrain skill packages and how
the runtime discovers, mounts, and exposes them to agents.

## Role

A skill is a reusable instruction package. It is not a separate runtime process
and it does not add new tools by itself.

At scan time, each `SKILL.md` becomes a `kind=skill` `OpNode`. At prompt time,
runtime tells the agent which skills are available and where their files live.
The agent reads the skill body with the normal `read` tool when a task matches.

Skills are the OpenBrain way to ship focused workflows without forking the base
agent prompt for every use case.

## Mental Model

```text
SKILL.md front matter   -> scanner metadata (id, name, description, tags)
SKILL.md markdown body  -> instructions the model reads on demand
AGENT.md skills:        -> which skill nodes this agent may use
Runtime prompt appendix -> short catalog injected at loop start
read tool               -> agent loads full skill instructions when needed
```

Runtime owns discovery and catalog injection. Skill authors own the workflow
text inside `SKILL.md`. Agents own routing logic in their prompt body.

## Package Layout

Each skill lives in its own directory with a single manifest file:

```text
skills/<slug>/
  SKILL.md
  references/          # optional supporting files
  scripts/             # optional helper scripts referenced by the skill
```

Agent-local skills use the same shape under the agent resource directory:

```text
agents/<name>/.agent/
  AGENT.md
  skills/
    my-skill/
      SKILL.md
```

Global skills live under the runtime base directory:

```text
<baseDir>/
  skills/
    search/
      SKILL.md
  agents/
    ...
  tools/
    ...
```

Prefer agent-local skills when a workflow is specific to one product agent.
Prefer global skills when multiple agents or workspaces should share the same
package.

## SKILL.md Manifest

Every skill file uses YAML front matter followed by a Markdown body.

### Required For Scanning

The runtime scanner rejects a skill when either field is missing:

| Field | Purpose |
| --- | --- |
| `name` | Human-readable skill name shown in catalogs |
| `description` | One-line summary used in runtime prompt appendices |

The directory name becomes the skill slug. For `skills/query/SKILL.md`, the
slug is `query`.

### Recommended

| Field | Purpose |
| --- | --- |
| `id` | Stable runtime node id, for example `skill-query`. Use this for built-in or shared packages so moves do not change references. |
| `tags` | Free-form labels used by product UI and filtering |

Explicit ids must use the `skill-` prefix.

### Optional Authoring Fields

OpenBrain runtime ignores extra front matter keys, but agent authors often use
them for routing and documentation inside larger skill packs such as GBrain:

| Field | Typical use |
| --- | --- |
| `triggers` | Phrases that tell the agent when to read this skill |
| `tools` | Tool names the workflow expects after the skill is loaded |
| `version` | Skill package version for humans and packaging |
| `mutating` | Whether the workflow writes durable state |

Treat these as skill-author conventions unless a product feature explicitly
reads them. The runtime contract for OpenBrain itself is: scan metadata,
mount on agents, expose paths, let the model `read` the body.

### Minimal Example

```yaml
---
name: search
description: Search project docs and return cited answers.
id: skill-search
tags: docs,retrieval
---

# Search Skill

Read this file before searching.

## Steps
1. Clarify the question.
2. Search the workspace.
3. Answer with file citations.
```

## Mounting Skills On An Agent

Agents declare skills in `AGENT.md` front matter:

```yaml
skills:
  - ./skills
```

Relative paths resolve inside `agents/<name>/.agent`.

### Supported Reference Forms

| Form | Meaning |
| --- | --- |
| `./skills` | Scan every child directory under `.agent/skills` that contains `SKILL.md` |
| `./skills/query/SKILL.md` | Mount one explicit skill file |
| `@skills/search` | Mount a global skill from `<baseDir>/skills/search/SKILL.md` |
| `@skill-search` | Mount a previously scanned skill by explicit node id |

You do not need to enumerate every skill file when they already live under one
directory. Prefer `./skills` for agent-local skill packs.

Example from the built-in GBrain agent:

```yaml
skills:
  - ./skills
```

Example using a shared global skill:

```yaml
skills:
  - "@skills/search"
```

Example mixing global and local packages:

```yaml
skills:
  - "@skills/plan"
  - ./skills/deep-research
```

Tools use the same reference model. See `docs/runtime.md` for the full scanner
reference rules.

## How Runtime Uses Skills

During node refresh, runtime scans tools, then skills, then agents. Agent
manifests resolve their `skills:` entries against the in-memory scan set and
any agent-local directories/files.

When an agent loop starts, runtime may append host-owned sections to the system
prompt:

- `Available Skills`: short catalog of mounted skills with `@skills/<slug>` paths
- `Selected Skills`: explicit user-selected skills for the current turn

These appendices are runtime behavior. They are not stored inside `AGENT.md`.

`Selected Skills` can come from an interactive user turn or from a runtime
caller such as cron. The call meta fields are:

- `selectedSkillIDs`: explicit skill node ids to select for this turn
- `selectedSkillContext`: non-secret runtime context passed with the selection

Cron tasks use the same mechanism by placing those fields under
`payload.data`. Runtime promotes them into agent-call meta before dispatching
the task. This lets a task select a skill without modifying the target agent's
base prompt.

The catalog tells the model that skills exist and where to read them. It does
not inline every skill body into the prompt. The expected agent workflow is:

1. Match the user request to a skill from the catalog or agent prompt routing.
2. Use `read` on that skill's `SKILL.md`.
3. Follow the skill instructions with the agent's existing tools.

Skills do not define new runtime tools. A `tools:` list inside `SKILL.md` is
documentation for the workflow author and the model, not a scanner mount list.

## Bundled Helpers

A skill may ship helper scripts or binaries under its own directory when a
workflow needs deterministic local behavior. Helpers are invoked by the agent's
existing tools, usually `shell`; they are not MCP servers and they do not grant
new model-visible tool schemas.

Example: `skills/openbrain-cloud-sync` bundles
`bin/openbrain-cloud-sync-helper[.exe]`. The helper performs credential-bound
local setup such as temporary `GIT_ASKPASS` and Cloud Brain sync triggering.
The skill text tells the agent when to run it, how to interpret its structured
JSON result, and when to notify the user instead of making a destructive git
decision.

Helper output must not include secrets. `selectedSkillContext` also must not
include secrets; use it for workspace ids, paths, scheduling hints, and other
non-secret routing context only.

## Large Skill Packs

The built-in `agents/gbrain` package ships dozens of skills under
`.agent/skills/`. It mounts them with a single directory reference and keeps
routing guidance in the agent prompt body plus generated resolver content.

Patterns worth copying:

- Keep `AGENT.md` front matter short. Long front matter breaks Markdown editor
  front matter rendering and is harder to review.
- Put detailed routing tables in the agent prompt body or a dedicated resolver
  file, not in YAML list form inside front matter.
- Keep each skill focused. One workflow per directory.

GBrain-specific skill conventions live in the external `colinagent/gbrain`
fork, which tracks upstream `garrytan/gbrain`. OpenBrain runtime behavior
itself stays the same.

## Authoring Checklist

- Create `skills/<slug>/SKILL.md` or `agents/<name>/.agent/skills/<slug>/SKILL.md`.
- Include `name` and `description`.
- Add `id: skill-<slug>` for built-in or shared packages.
- Mount the skill from `AGENT.md` with `./skills`, a file path, or `@skills/<slug>`.
- Write the workflow in the Markdown body, not only in front matter.
- Tell the agent when to read the skill in the parent agent prompt if routing matters.

## Validation And Tests

Scanner tests live in `opagent-runtime/internal/scan/`. Useful cases to copy:

- `TestScanSkills_RequiresNameAndDescription`
- `TestScanAgents_ResolvesRelativeSkillDirectoryRef`

Run:

```bash
(cd opagent-runtime && go test ./internal/scan/...)
```

## Related Docs

- `docs/runtime.md`: node scanning, reference resolution, prompt assembly
- `docs/tools.md`: tool-server manifests and MCP refresh behavior
- `docs/opagent-protocol.md`: shared protocol types and opcodes
- `docs/subagent.md`: agent-to-agent delegation, which is separate from skills
