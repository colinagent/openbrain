# Subagents in OpenBrain

OpAgent Runtime owns generic subagent discovery, `agent_task` delegation,
child-thread persistence, model selection, and permission inheritance. See the
[OpAgent component documentation](https://github.com/colinagent/opagent/blob/main/docs/components.md).

OpenBrain uses that mechanism for product knowledge work. The GBrain Agent and
its local Skill pack stay in `agents/gbrain`; a parent may mount it when the
product needs retrieval, synthesis, ingestion, or durable knowledge
maintenance. It is optional and is not hard-coded into the generic Coder Agent.

Delegated GBrain work receives the parent thread's immutable permission
snapshot and active workspace context. The child cannot select Full access,
install a product session, or broaden Cloud permissions. Product account and
managed Tool policy are enforced by the Runtime host on the machine that owns
the workspace.

Generic Simple Memory is distributed by OpAgent. OpenBrain-specific knowledge,
account, Cloud, and GBrain prompts remain in this repository.
