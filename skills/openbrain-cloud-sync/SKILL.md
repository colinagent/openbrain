---
id: skill-openbrain-cloud-sync
name: OpenBrain Cloud Sync
description: Batch sync OpenBrain Cloud workspaces with local git and Cloud Brain indexing.
tags: builtin,openbrain,cloud,sync
---

# OpenBrain Cloud Sync

Use this skill only for the managed `OpenBrain Cloud Sync` cron task or an explicit user request to sync OpenBrain Cloud workspaces.

## Required Flow

1. Determine the run mode. For scheduled runs, do not treat any task input,
   selected skill context, or persisted cron payload workspace list as
   authoritative; it is only a stale-prone audit snapshot. For a manual run,
   use the requested workspace from task input only as the target to pass back
   into the helper.
2. If this turn is a message-system follow-up with `selectedSkillContext.messageSystem=true`
   and `answers`, handle the user answer before normal preflight.
3. Run the helper preflight once before doing detailed work. The helper reads
   the current OpenBrain account from `~/.openbrain/configs/user/auth.json` and
   only returns workspaces from that account's local index partition. On Windows,
   append `.exe` to the helper filename:
   ```sh
   ~/.openbrain/skills/openbrain-cloud-sync/bin/openbrain-cloud-sync-helper preflight
   ```
   For a manual request, use `preflight --include-disabled` so the requested
   workspace can be inspected even when auto sync is disabled.
4. For scheduled runs, process only the workspaces returned by helper preflight.
   Skip workspaces whose preflight result is `clean`. If a workspace appears in
   the cron payload but not in helper preflight, do not sync it and do not guess
   from old local state.
5. Before acting on each workspace, read `<workspacePath>/AGENTS.md` if it
   exists. Cron CWD can differ from the workspace currently being processed.
   When preflight returns `nested_git`, check whether the blocked path already
   has a durable decision under `## OpenBrain Cloud Sync Decisions` before
   publishing a new request.
6. For each workspace that needs sync, run:
   ```sh
   ~/.openbrain/skills/openbrain-cloud-sync/bin/openbrain-cloud-sync-helper sync --workspace-id <workspaceID>
   ```
   When applying an existing keep-independent decision, rerun with:
   ```sh
   ~/.openbrain/skills/openbrain-cloud-sync/bin/openbrain-cloud-sync-helper sync --workspace-id <workspaceID> --allow-nested <path>
   ```
7. If the helper returns `ok: true`, record the result and move to the next workspace.
8. If the helper returns a blocking status, inspect with normal git commands in that workspace and decide whether it is safe to continue.

The helper handles OpenBrain API auth, workspace GitHub token exchange, temporary `GIT_ASKPASS`, standard commit/pull/rebase/push, and Cloud Brain sync trigger. Do not print tokens, do not put tokens in remote URLs, and do not ask the user for GitHub credentials.

Token handling:

- If the helper returns `code: "git_token_expired"` with `retryable: true`, rerun that helper command once. The helper requests a fresh short-lived workspace git token on every run.
- If the helper returns `code: "login_required"`, the user's OpenBrain session token in `~/.openbrain/configs/user/auth.json` is expired or missing. Do not keep retrying git. Publish a message telling the user to sign in again from the OpenBrain desktop login dialog, then rerun the helper after the user confirms login is complete.
- If the helper returns `code: "cloud_permission_denied"`, the current OpenBrain account lacks Cloud workspace ACL access. Do not describe this as a GitHub App or repo permission problem. Publish a request with fixed options to switch accounts, request sharing, remove the local binding, or skip this workspace.
- If the helper returns `code: "workspace_not_bound_for_account"`, do not sync by guessing from old local state. Treat the workspace as not bound for the current account and use the same recovery choices as `cloud_permission_denied`.
- Use `git_permission_denied` only for real git/GitHub repository access failures from git commands.
- Never echo token values, never include them in command arguments, and never write them to git remotes.

## Blocking Cases

The helper blocks on any nested `.git` directory or index gitlink that is not
explicitly allowed with `--allow-nested <relpath>`. It does not read
`AGENTS.md` or decide policy itself. Before publishing a new nested-git request,
read `<workspacePath>/AGENTS.md` and look for an existing
`## OpenBrain Cloud Sync Decisions` entry for the blocked path. If the user
already chose `keep-independent` or another durable policy, apply that policy
and rerun the helper with the matching `--allow-nested` path instead of asking
again.

Stop and use `message_publish` when:

- A merge or rebase conflict requires choosing between user content and remote content.
- A nested git repository or gitlink is present and no durable workspace decision already covers the safe next step.
- A change would delete, overwrite, or rewrite user content.
- The helper returns `login_required`.
- The helper returns `cloud_permission_denied` or `workspace_not_bound_for_account`.
- The helper still returns `git_token_expired` after one rerun.
- The helper returns any failure that is not clearly mechanical.

The message must include the workspace name, path, failure reason, and one
structured question for the user. User decisions must be published as
`questions[]` options, not only as text in the body and not as command-style
`actions[]`:

```json
{
  "kind": "request",
  "title": "Sync Blocked: <workspace> / <path> <issue>",
  "body": "...",
  "questions": [
    {
      "id": "nested_git_resolution",
      "question": "What should happen to <path>?",
      "options": [
        { "id": "option-id", "label": "Human readable option" }
      ]
    }
  ]
}
```

Request question fields are intentionally minimal: use the message `title` for
the business heading, `questions[].id`, `questions[].question`, and
`questions[].options[].id/label`. Do not add per-question headers, `allowOther`,
option descriptions, or option tones. Every concrete option mentioned in the
body must have a matching `questions[].options[]` item with a stable `id` and
clear `label`. Body-only option lists are invalid for user decisions. The client
adds a free-form `Other...` answer automatically, so do not add an option whose
id or label is `other`. If you notice that you published a request without
`questions[]`, republish a corrected request before ending the turn.

For nested git/gitlink blockers, default to these option ids when applicable:

- `convert-submodule`: convert the nested repository into a proper submodule.
- `keep-independent`: remove the gitlink and keep the nested repository managed independently.
- `vendor-regular-files`: remove the nested `.git` metadata and commit it as regular workspace files.
- `remove-nested-repo`: remove the nested repository from this workspace.

For `cloud_permission_denied` or `workspace_not_bound_for_account`, use this
question shape and option ids:

```json
{
  "id": "cloud_permission_recovery",
  "question": "How would you like to resolve this OpenBrain workspace access issue?",
  "options": [
    { "id": "switch-account", "label": "Switch OpenBrain account" },
    { "id": "request-sharing", "label": "Request workspace sharing" },
    { "id": "remove-local-binding", "label": "Remove local binding" },
    { "id": "skip-workspace", "label": "Skip this workspace" }
  ]
}
```

Do not wait synchronously for the user; publish the message and finish the
current sync turn with the blocked workspace listed.

## User Answer Replies

When a user answers a request question, the message system resumes the original
thread with `requestTitle`, `answers`, `messageID`, `channelID`, and
`replyToMessageID` in selected skill context. `requestTitle` is the business
anchor for the original request; use it with `questionID` and `optionID` to map
the user's choice. Each answer has `questionID` and either `optionID` for a
listed option or `other:true` plus free-form `text` for the client-added Other
answer. Treat a known `optionID` as explicit consent for that option, but still
reinspect the workspace before changing git.

For question replies:

1. Read the pending/original message with `message_read` when the workspace,
   path, or original request is not already clear from `requestTitle`.
2. Read `<workspacePath>/AGENTS.md` before modifying workspace files. If the
   workspace has no `AGENTS.md`, create it only when the workspace conventions
   allow it. Do not create `AGENTS.md` when workspace instructions or product
   boundaries forbid it; publish a follow-up request for an allowed persistence
   target instead.
3. Re-run normal git inspection for the affected path; do not rely on stale
   details from the original request.
4. Before structural git changes, persist the user decision in the workspace
   `AGENTS.md` under this section, updating an existing matching path entry
   instead of appending duplicates:

   ```md
   ## OpenBrain Cloud Sync Decisions

   - `<path>/`: <durable policy>. Decision source: `<requestTitle>` / `<optionID or other answer messageID>`.
   ```

5. Apply only the selected option. For `keep-independent`, persist the
   decision in `AGENTS.md`, remove the parent gitlink, add the path to
   `.gitignore`, then rerun helper `sync` with
   `--allow-nested <path>` so the on-disk nested repo no longer blocks.
6. The message system marks the original request resolved when the user answer
   is recorded. After the selected option succeeds, use `message_update` on the
   original request (`replyToMessageID` when available) to add final detail or
   keep `status:"resolved"` idempotently. If the current workspace state no
   longer matches the original blocker, publish a new request with `questions[]`
   instead of guessing.
7. If the answer is `other:true`, read the free-form `text`. Execute it only
   when it clearly maps to a safe concrete option after inspection. Otherwise
   publish a new `questions[]` request that reflects the user's custom intent as
   an explicit option.

Nested git/gitlink option meanings:

- `convert-submodule`: persist that the path is intended to be a real submodule;
  maintain `.gitmodules` and do not vendor the nested files.
- `keep-independent`: persist that the path is an independent repository; remove
  the parent gitlink, add the path to `.gitignore`, rerun helper sync with
  `--allow-nested <path>`, and do not vendor the nested files.
- `vendor-regular-files`: persist that the path should be regular workspace
  content; remove nested git metadata only after confirming the nested repo
  state is safe.
- `remove-nested-repo`: persist that the path should not live in this workspace;
  remove it only after an explicit user answer and safe inspection.

## Safe Git Rules

- Never run `git reset --hard`, `git checkout -- .`, `git clean`, or destructive deletion commands unless the user explicitly approves.
- Use normal git inspection commands: `git status`, `git diff`, `git diff --staged`, `git ls-files --stage`, and `git submodule status`.
- If you resolve conflicts manually, use `git add` and `git rebase --continue` only after the resolution is clear.
- After manual conflict resolution, run the helper `sync` command again for that workspace so push and Cloud Brain sync still use managed credentials.

## Expected Output

At the end, summarize:

- Synced workspaces.
- Clean skipped workspaces.
- Blocked workspaces, request titles, message IDs, channels, and visible question option labels used for
  user notification.
