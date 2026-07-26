# Helper Architecture

## Decision

Implement `word-document` as a dependency-free Go binary and cross-compile it
with OpenBrain's Runtime and Marketplace release builders for Darwin arm64,
Darwin amd64, Linux amd64, and Windows amd64.

The installed path is:

```text
<baseDir>/skills/word-document/bin/word-document[.exe]
```

This avoids production dependencies on a system Python, Node.js, Microsoft
Word, WPS, LibreOffice, or a document server. Go is a build-time dependency
only. The helper uses `archive/zip`, `encoding/xml`, and other standard-library
packages; it performs no network requests.

## Safety boundary

- Accept only bounded, well-formed OPC ZIP packages with required DOCX parts.
- Reject encrypted entries, unsafe or duplicate paths, unsupported compression,
  altChunk/HTML parts, more than 2,048 entries, compressed input over 50 MiB,
  expanded content over 200 MiB, a single entry over 100 MiB, or compression
  ratios over 100:1.
- Preserve untouched ZIP members byte-for-byte at the compressed-member level.
- Validate all anchors before creating any mutation.
- Generate the complete package in memory, validate its OOXML relationships,
  content types, comment IDs and markers, then write temporary files in the
  destination directory.
- Refuse an existing output or audit path. Publish output and audit as a pair;
  remove the newly created output if publishing the audit fails.
- Store only basenames and hashes in audit JSON, never absolute system paths.

## Source and validation

The source module is `skills/word-document`. Run:

```sh
(cd skills/word-document && go test ./...)
go test ./opagent-runtime/internal/scan/...
scripts/check-openbrain-public-boundary.sh
```

Release builders inject the platform binary into the copied skill package.
The Go source is public AGPL-3.0 source; no proprietary Desktop or Server code
belongs in this package.
