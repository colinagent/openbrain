# OpenBrain Skills

Generic Skill discovery and authoring are part of
[OpAgent](https://github.com/colinagent/opagent/blob/main/docs/components.md).
This repository keeps only Skills that belong to the OpenBrain product.

## GBrain Skill pack

`agents/gbrain/.agent/skills` contains the knowledge workflows mounted only by
the GBrain Agent: retrieval, synthesis, ingestion, filing, research, schema,
maintenance, and migration guidance. Keeping the pack under the Agent prevents
product knowledge behavior from appearing in unrelated generic Agents.

The resolver is on-demand. Narrow read-only questions should call the relevant
brain Tool first; a workflow reads only the minimal matching `SKILL.md` files.

## OpenBrain Cloud Sync

`skills/openbrain-cloud-sync` is a product Skill and Go helper for managed Cloud
workspace synchronization. It reads the active product account and workspace
index from `~/.opagent/openbrain`, uses short-lived product-issued Git
credentials, and fails closed on account/binding mismatches, nested repositories,
conflicts, or destructive choices.

The Skill publishes structured user questions for decisions and records durable
workspace policy only after explicit answers. It never prints credentials or
puts tokens in Git remote URLs.

```bash
(cd skills/openbrain-cloud-sync && go test ./...)
node --test skills/openbrain-cloud-sync/SKILL.source.test.mjs
```

Generic Coder, Simple Memory, and Word Document packages are maintained in the
OpAgent repository rather than copied here.
