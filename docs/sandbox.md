# OpenBrain sandbox and permission policy

OpAgent Runtime is the execution and permission authority for the active local
or remote workspace. The portable enforcement design lives in the
[OpAgent sandbox documentation](https://github.com/colinagent/opagent/blob/main/docs/sandbox.md).
This page records OpenBrain's product-facing policy.

## Product profiles

OpenBrain presents Ask for approval, Approve for me, Full access, and Custom
profiles. The Runtime stores an immutable permission snapshot on each thread;
subagents inherit it as a ceiling. A model, repository, Skill, remote Tool, or
scheduled task cannot select or broaden its own profile.

Full access requires explicit user confirmation and trusted workspace state.
Safe profiles fail closed when the platform sandbox is unavailable. Approval
is pre-action: a later diff or warning cannot undo an external side effect.

## Host review

“Approve for me” uses the product's trusted `ApprovalReviewer` hook. If review
is unavailable, times out, or fails validation, the request returns to user
review or is denied; it is never silently approved. The public Runtime does not
contain a product reviewer or product account implementation.

## Product services

OpenBrain session resolution and managed-service authorization are compiled in
[`integrations/opagent`](../integrations/opagent). Unknown system services and
remote nodes fail closed. User configuration cannot grant private
`openbrain-server` trust or turn an arbitrary remote manifest into the managed
GBrain endpoint.

Local and remote workspaces follow the same rule: enforcement happens on the
machine that owns and executes the workspace Runtime.
