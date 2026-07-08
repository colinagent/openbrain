import assert from 'node:assert/strict';
import test from 'node:test';

import { EditorState } from '@codemirror/state';
import {
  appendChatSelectionToDraft,
  buildChatSelectionPrompt,
  buildMarkdownChatSelectionSnapshot,
  buildTextChatSelectionSnapshot,
} from './chatSelectionPrompt';

function createState(doc: string, from: number, to: number): EditorState {
  return EditorState.create({
    doc,
    selection: { anchor: from, head: to },
  });
}

test('buildMarkdownChatSelectionSnapshot resolves user-message and heading breadcrumbs from source positions', () => {
  const doc = [
    '@user-example',
    '',
    'Need auth flow',
    '',
    '## API Design',
    'Body paragraph',
  ].join('\n');
  const from = doc.indexOf('## API Design');
  const to = from + 2;

  const snapshot = buildMarkdownChatSelectionSnapshot(createState(doc, from, to));

  assert.ok(snapshot);
  assert.equal(snapshot?.kind, 'markdown');
  assert.deepEqual(snapshot?.breadcrumb, ['Need auth flow', 'API Design']);
  assert.equal(snapshot?.startLine, 5);
  assert.equal(snapshot?.endLine, 5);
});

test('buildMarkdownChatSelectionSnapshot marks frontmatter selections explicitly', () => {
  const doc = [
    '---',
    'title: hello',
    'threadID: thread-1',
    '---',
    '',
    'Body',
  ].join('\n');
  const from = doc.indexOf('title');
  const to = from + 'title: hello'.length;

  const snapshot = buildMarkdownChatSelectionSnapshot(createState(doc, from, to));

  assert.ok(snapshot);
  assert.deepEqual(snapshot?.breadcrumb, ['Frontmatter']);
  assert.equal(snapshot?.from, from);
  assert.equal(snapshot?.to, to);
});

test('buildTextChatSelectionSnapshot keeps line ranges stable when selection ends at the next line start', () => {
  const doc = ['alpha', 'beta', 'gamma'].join('\n');
  const from = doc.indexOf('beta');
  const to = doc.indexOf('gamma');

  const snapshot = buildTextChatSelectionSnapshot(createState(doc, from, to));

  assert.ok(snapshot);
  assert.equal(snapshot?.kind, 'text');
  assert.equal(snapshot?.startLine, 2);
  assert.equal(snapshot?.endLine, 2);
});

test('buildChatSelectionPrompt formats markdown selections with breadcrumb, source range, and safe fences', () => {
  const prompt = buildChatSelectionPrompt({
    kind: 'markdown',
    text: '```ts\nconst value = 1;\n```',
    from: 12,
    to: 37,
    startLine: 4,
    endLine: 6,
    breadcrumb: ['Need auth flow', 'API Design'],
  }, '/tmp/workspace/notes.md');

  assert.match(prompt, /Selection from `\/tmp\/workspace\/notes\.md`/);
  assert.match(prompt, /Section: `Need auth flow > API Design`/);
  assert.match(prompt, /Source range: `12-37`/);
  assert.match(prompt, /````markdown/);
  assert.match(prompt, /\n````$/);
});

test('buildChatSelectionPrompt formats text selections with file path and line range', () => {
  const prompt = buildChatSelectionPrompt({
    kind: 'text',
    text: 'const answer = 42;',
    from: 5,
    to: 23,
    startLine: 10,
    endLine: 12,
  }, '/tmp/workspace/src/app.ts');

  assert.match(prompt, /Selection from `\/tmp\/workspace\/src\/app\.ts:10-12`/);
  assert.match(prompt, /```ts/);
});

test('appendChatSelectionToDraft adds a blank line between existing draft and the new selection block', () => {
  const combined = appendChatSelectionToDraft('Existing draft', 'Selection block');

  assert.equal(combined, 'Existing draft\n\nSelection block');
});
