import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const skill = readFileSync(new URL('./SKILL.md', import.meta.url), 'utf8');
const architecture = readFileSync(new URL('./references/helper-architecture.md', import.meta.url), 'utf8');
const comments = readFileSync(new URL('./references/ooxml-comments.md', import.meta.url), 'utf8');
const trackedChanges = readFileSync(new URL('./references/ooxml-tracked-changes.md', import.meta.url), 'utf8');
const runtimeBuild = readFileSync(new URL('../../scripts/openbrain/build-runtime-release.sh', import.meta.url), 'utf8');
const marketplaceBuild = readFileSync(new URL('../../scripts/openbrain/build-marketplace-release.sh', import.meta.url), 'utf8');

test('Word skill requires deterministic inspect, write, validate flow', () => {
  assert.match(skill, /inspect --input/);
  assert.match(skill, /add-comments/);
  assert.match(skill, /add-redlines/);
  assert.match(skill, /apply-revisions/);
  assert.match(skill, /validate --input/);
  assert.match(skill, /input_sha256/);
  assert.match(skill, /context_hash/);
  assert.match(skill, /exact_quote/);
  assert.match(skill, /never overwrite an existing/i);
  assert.match(skill, /Do not retry by searching for a similar sentence/);
});

test('Word skill exposes true redline and clean-copy modes', () => {
  assert.match(skill, /`comments-only`/);
  assert.match(skill, /`redline`/);
  assert.match(skill, /`clean-copy`/);
  assert.match(trackedChanges, /w:delText/);
  assert.match(trackedChanges, /w:ins/);
  assert.match(trackedChanges, /revision_count: 0/);
  assert.match(trackedChanges, /Fail closed/);
});

test('Word helper is a bundled dependency-free executable', () => {
  assert.match(architecture, /dependency-free Go binary/);
  assert.match(architecture, /performs no network requests/);
  assert.match(architecture, /never absolute system paths/);
  for (const build of [runtimeBuild, marketplaceBuild]) {
    assert.match(build, /skills\/word-document\/bin\/word-document/);
    assert.match(build, /\.\/skills\/word-document\/cmd\/word-document/);
  }
});

test('OOXML comments contract is complete and failure-closed', () => {
  for (const token of [
    'word/comments.xml',
    'w:commentRangeStart',
    'w:commentRangeEnd',
    'w:commentReference',
    'word/_rels/document.xml.rels',
    '[Content_Types].xml',
  ]) {
    assert.ok(comments.includes(token), `missing ${token}`);
  }
  assert.match(comments, /marked non-anchorable/);
  assert.match(comments, /Fail\s+closed/);
});
