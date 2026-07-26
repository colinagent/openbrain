# OOXML Comment Contract

## Parts and links

A generated comment uses all four OOXML structures below:

1. `word/document.xml` contains matching `w:commentRangeStart`,
   `w:commentRangeEnd`, and `w:commentReference` elements.
2. `word/comments.xml` contains one `w:comment` with the same non-negative
   integer ID, author, UTC timestamp, and escaped plain-text body.
3. `word/_rels/document.xml.rels` contains the standard comments relationship
   targeting `comments.xml`.
4. `[Content_Types].xml` contains the standard comments override.

Existing comments and relationships remain in place. New IDs use the lowest
available non-negative integer and must never collide with an existing ID.

## Anchor contract

`inspect` exposes paragraphs as blocks, including paragraphs inside table
cells. `start` and `end` are zero-based Unicode code-point offsets into the
block's `text`; `end` is exclusive. A valid mutation must satisfy all of:

- the plan's `input_sha256` equals the current DOCX hash;
- the block still exists;
- its `context_hash` is unchanged;
- slicing the current block at `start:end` equals `exact_quote` exactly;
- every selected run is marked `anchorable`;
- ranges in one batch do not overlap.

The helper may split a text run at either boundary. It clones that run's raw
`w:r` markup and therefore retains run properties such as bold, italic, font,
size, language, and style. It does not flatten the paragraph or replace table,
numbering, section, header, footer, drawing, field, hyperlink, existing comment,
or tracked-change structures around the selected runs.

Runs with drawings, field codes, tabs, breaks, multiple text nodes, or other
non-text children are visible in inspection but marked non-anchorable. Fail
closed instead of widening the requested range around them.

## Review checks

After writing, confirm that `validate` reports exactly one start, end, and
reference marker for every comment ID, that every marker resolves to a comment,
and that the comments relationship and content type are present. Treat any
validation error as a failed artifact and do not deliver it.
