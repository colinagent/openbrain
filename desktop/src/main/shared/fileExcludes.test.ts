import assert from 'node:assert/strict';
import test from 'node:test';

import {
  filterFileEntries,
  getEnabledFileExcludePatterns,
  shouldExcludeFileEntry,
} from './fileExcludes';

test('default file excludes hide macOS and Windows metadata files', () => {
  const patterns = getEnabledFileExcludePatterns(undefined);

  assert.equal(shouldExcludeFileEntry('.DS_Store', '/workspace/note', patterns), true);
  assert.equal(shouldExcludeFileEntry('Thumbs.db', '/workspace/note', patterns), true);
  assert.equal(shouldExcludeFileEntry('.agent', '/workspace/note', patterns), false);
  assert.equal(shouldExcludeFileEntry('AGENT.md', '/workspace/note/.agent', patterns), false);
});

test('file exclude config can disable a default pattern and add a custom pattern', () => {
  const patterns = getEnabledFileExcludePatterns({
    '**/.DS_Store': false,
    '**/*.tmp': true,
  });

  assert.equal(shouldExcludeFileEntry('.DS_Store', '/workspace/note', patterns), false);
  assert.equal(shouldExcludeFileEntry('draft.tmp', '/workspace/note', patterns), true);
});

test('filterFileEntries preserves visible entries', () => {
  const patterns = getEnabledFileExcludePatterns(undefined);
  const entries = [
    { name: '.DS_Store', isDir: false },
    { name: '.agent', isDir: true },
    { name: 'note.md', isDir: false },
  ];

  assert.deepEqual(filterFileEntries(entries, '/workspace/note', patterns), [
    { name: '.agent', isDir: true },
    { name: 'note.md', isDir: false },
  ]);
});
