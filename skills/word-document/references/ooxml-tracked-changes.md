# OOXML Tracked Changes Contract

## True replacements

For each anchored replacement, the helper writes adjacent revision wrappers:

```xml
<w:del w:id="10" w:author="OpenBrain" w:date="2026-07-26T09:00:00Z">
  <w:r><w:delText>old text</w:delText></w:r>
</w:del>
<w:ins w:id="11" w:author="OpenBrain" w:date="2026-07-26T09:00:00Z">
  <w:r><w:t>new text</w:t></w:r>
</w:ins>
```

IDs are unique across existing `w:ins`, `w:del`, `w:moveTo`, and `w:moveFrom`
elements. The inserted run inherits the first selected run's properties. Each
deleted run retains its own properties and changes only `w:t` to `w:delText`.
The helper adds `w:trackRevisions` and wires `word/settings.xml` when necessary.

The normal anchor contract still applies: input hash, block ID, Unicode
offsets, exact quote, context hash, non-overlap, and anchorable runs must all
match. A range may cross adjacent text runs, but not a hyperlink, field,
drawing, existing revision, or another structured boundary.

## Clean copies

`apply-revisions --mode accept` removes `w:del`/`w:moveFrom` content and unwraps
`w:ins`/`w:moveTo` content. `--mode reject` performs the inverse and converts
retained `w:delText`/`w:delInstrText` back to `w:t`/`w:instrText`. Both modes
remove move-range markers and `w:trackRevisions`, preserve comments and other
parts, and require the output validator to report `revision_count: 0`.

Fail closed on `w:pPrChange`, `w:rPrChange`, table/grid/row/cell property
changes, section changes, numbering changes, cell insertion/deletion/merge,
custom XML revision ranges, empty property-level deletion markers, or malformed
revision IDs/authors/dates. These structures require a richer Word revision
engine and must never be silently flattened.

## Validation

For a redline artifact, verify:

- every revision has a unique non-negative ID, non-empty author, and RFC 3339
  timestamp;
- deletions use `w:delText`, insertions use `w:t`, and XML remains well formed;
- settings relationship/content type are valid when newly created;
- the visible render is reviewed after structural validation.

For a clean copy, verify `revision_count` is zero, re-inspect the resulting text,
and render every page. Comments are independent review information and must not
be removed by accept/reject.
