import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildMarkdownPdfDefaultPath,
  replaceMarkdownLikeExtensionWithPdf,
} from './pdfExportPaths';

test('replaces markdown extension with pdf', () => {
  assert.equal(replaceMarkdownLikeExtensionWithPdf('notes.md'), 'notes.pdf');
  assert.equal(replaceMarkdownLikeExtensionWithPdf('notes.markdown'), 'notes.pdf');
  assert.equal(replaceMarkdownLikeExtensionWithPdf('NOTES.MD'), 'NOTES.pdf');
});

test('appends pdf extension for non-markdown names', () => {
  assert.equal(replaceMarkdownLikeExtensionWithPdf('notes'), 'notes.pdf');
  assert.equal(replaceMarkdownLikeExtensionWithPdf('notes.txt'), 'notes.txt.pdf');
  assert.equal(replaceMarkdownLikeExtensionWithPdf('notes.pdf'), 'notes.pdf');
});

test('builds default path next to the source markdown file', () => {
  assert.equal(
    buildMarkdownPdfDefaultPath({
      sourcePath: '/tmp/docs/guide.markdown',
      currentDir: '/tmp/ignored',
    }),
    '/tmp/docs/guide.pdf',
  );
});

test('falls back to current dir when source path is absent', () => {
  assert.equal(
    buildMarkdownPdfDefaultPath({
      currentDir: '/tmp/workspace',
    }),
    '/tmp/workspace/Untitled.pdf',
  );
});

test('falls back to untitled pdf when no path hints exist', () => {
  assert.equal(buildMarkdownPdfDefaultPath({}), 'Untitled.pdf');
});
