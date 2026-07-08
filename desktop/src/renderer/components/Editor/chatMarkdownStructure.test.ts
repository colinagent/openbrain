import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildOutlineTreeEntries,
  getLinesFromContent,
  parseOutlineEntries,
} from './chatMarkdownStructure';

test('builds a user-rooted outline for chat markdown', () => {
  const content = [
    '---',
    'thread: thread-1',
    'title: "Hello"',
    '---',
    '',
    '@user-example',
    'Hello **world** [link](https://example.com)',
    '',
    '@agent-coder',
    '',
    '# Answer',
    '## Details',
  ].join('\n');

  const tree = buildOutlineTreeEntries(parseOutlineEntries(getLinesFromContent(content)));
  assert.equal(tree.length, 3);
  assert.equal(tree[0].type, 'user-message');
  assert.equal(tree[0].text, 'Hello world link');
  assert.equal(tree[1].parentId, tree[0].id);
  assert.equal(tree[1].depth, 1);
  assert.equal(tree[2].parentId, tree[1].id);
  assert.equal(tree[2].depth, 2);
});

test('attaches headings to the nearest preceding user marker', () => {
  const content = [
    '@user-example',
    'first question',
    '',
    '@agent-coder',
    '# First answer',
    '',
    '@user-example',
    'second question',
    '',
    '@agent-coder',
    '# Second answer',
  ].join('\n');

  const tree = buildOutlineTreeEntries(parseOutlineEntries(getLinesFromContent(content)));
  const firstUser = tree[0];
  const firstHeading = tree[1];
  const secondUser = tree[2];
  const secondHeading = tree[3];

  assert.equal(firstHeading.parentId, firstUser.id);
  assert.equal(secondHeading.parentId, secondUser.id);
});

test('sanitizes and truncates user labels to 20 characters', () => {
  const content = [
    '@user-example',
    '- ![Pic](/pic.png) [Linked](https://example.com) `code` **bold** > quote',
  ].join('\n');

  const entries = parseOutlineEntries(getLinesFromContent(content));
  assert.equal(entries.length, 1);
  assert.equal(entries[0].type, 'user-message');
  assert.ok(entries[0].text.length <= 20);
  assert.ok(!entries[0].text.includes('!['));
  assert.ok(!entries[0].text.includes('['));
  assert.ok(!entries[0].text.includes('`'));
  assert.ok(!entries[0].text.includes('**'));
});

test('falls back to plain markdown headings when no user block exists', () => {
  const content = ['# Top', '## Child'].join('\n');
  const tree = buildOutlineTreeEntries(parseOutlineEntries(getLinesFromContent(content)));

  assert.equal(tree.length, 2);
  assert.equal(tree[0].type, 'heading');
  assert.equal(tree[0].parentId, null);
  assert.equal(tree[1].parentId, tree[0].id);
});

test('ignores headings inside unfinished fenced code blocks', () => {
  const content = ['```note', '# unfinished question'].join('\n');
  const entries = parseOutlineEntries(getLinesFromContent(content));

  assert.equal(entries.length, 0);
});
