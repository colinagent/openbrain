---
id: skill-word-document
name: word-document
description: Inspect and safely edit Microsoft Word DOCX files with deterministic anchors, true Word comments and tracked changes, accepted or rejected clean copies, validation, and machine-readable audits. Use for reading DOCX structure or text, adding review comments, proposing redlines, accepting or rejecting revisions, and validating generated Word documents without overwriting the original.
tags: builtin,document,docx,word
---

# Word Document

Use the bundled helper for every DOCX read or write. Do not improvise ZIP/XML
mutation code. On Windows, append `.exe` to the helper name.

```sh
word_helper="$HOME/.openbrain/skills/word-document/bin/word-document"
```

## Required workflow

1. Inspect the source before planning any change:

   ```sh
   "$word_helper" inspect --input contract.docx --output contract.inspect.json
   ```

2. Use only a returned `block_id`, zero-based Unicode `start`/`end` offsets,
   `exact_quote`, and `context_hash`. Do not calculate anchors from a converted
   Markdown or plain-text copy.
3. Build one JSON plan for the whole operation. Keep a stable `finding_id` for
   every requested write. Re-inspect if the source SHA-256 changed.
4. Write to a new path and always request an audit file:

   ```sh
   "$word_helper" add-comments \
     --input contract.docx \
     --plan comments.plan.json \
     --output contract_reviewed.docx \
     --audit contract_reviewed.audit.json
   ```

5. Validate the result before presenting it:

   ```sh
   "$word_helper" validate --input contract_reviewed.docx
   ```

6. Report the new DOCX and audit paths. State that generated legal or business
   content still requires the user's professional review.

## Output modes

- `comments-only`: use `add-comments` and the comment plan below.
- `redline`: use `add-redlines` with a redline plan. This creates real
  `w:del`/`w:ins` revisions and enables `w:trackRevisions`.
- `clean-copy`: run `apply-revisions --mode accept` on a redline DOCX. Use
  `--mode reject` only when the user explicitly wants the original wording.

```sh
"$word_helper" add-redlines \
  --input contract.docx \
  --plan redlines.plan.json \
  --output contract_redline.docx \
  --audit contract_redline.audit.json

"$word_helper" apply-revisions \
  --input contract_redline.docx \
  --mode accept \
  --output contract_clean.docx \
  --audit contract_clean.audit.json
```

## Comment plan

```json
{
  "version": 1,
  "input_sha256": "<sha256 from inspect>",
  "comments": [
    {
      "finding_id": "F-001",
      "block_id": "document.p000001",
      "exact_quote": "the exact source text",
      "start": 12,
      "end": 33,
      "context_hash": "<context_hash from inspect>",
      "body": "Review note",
      "author": "OpenBrain",
      "created_at": "2026-07-26T08:00:00Z"
    }
  ]
}
```

Omit `created_at` to use the current UTC time. Omit `author` to use
`OpenBrain`. Keep comment bodies plain text; the helper escapes XML.

## Redline plan

Use the same anchor contract and replace `comments` with `changes`. Each change
adds `replacement`; an empty replacement means deletion-only.

```json
{
  "version": 1,
  "input_sha256": "<sha256 from inspect>",
  "changes": [
    {
      "finding_id": "F-002",
      "block_id": "document.p000003",
      "exact_quote": "five business days",
      "replacement": "three business days",
      "start": 48,
      "end": 66,
      "context_hash": "<context_hash from inspect>",
      "author": "OpenBrain",
      "created_at": "2026-07-26T09:00:00Z"
    }
  ]
}
```

## Failure rules

- Never use the input path as an output path and never overwrite an existing
  output or audit file.
- Treat a changed input hash, changed context hash, quote mismatch, invalid
  offsets, overlapping ranges, unsupported run structure, malformed package,
  altChunk/HTML part, or resource-budget failure as a hard stop.
- Do not place a new redline inside an existing revision. Accept/reject supports
  text insertions, deletions, and moves; it fails closed on property, table-cell,
  numbering, or other revision structures that cannot be restored losslessly.
- Do not retry by searching for a similar sentence or selecting the first
  duplicate. Re-inspect, rebuild the plan, and keep the same finding IDs only
  when they still refer to the same findings.
- Do not claim Word/WPS compatibility unless `validate` returns `valid: true`.
  Validation proves package and OOXML invariants, not pixel-identical layout.
- Do not send document contents to an external conversion or preview service.
- Do not put credentials, grant tokens, hidden prompts, or private system paths
  in plans, comments, filenames, or audit metadata.

## References

- Read [references/helper-architecture.md](references/helper-architecture.md)
  before changing, rebuilding, or distributing the helper.
- Read [references/ooxml-comments.md](references/ooxml-comments.md) when a task
  involves comments, anchor failures, or existing comments.
- Read [references/ooxml-tracked-changes.md](references/ooxml-tracked-changes.md)
  before adding redlines or producing an accepted/rejected clean copy.
