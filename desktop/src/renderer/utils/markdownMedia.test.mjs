import assert from 'node:assert/strict';
import test from 'node:test';

import {
  DEFAULT_CHAT_MARKDOWN_IMAGE_WIDTH_PERCENT,
  getDefaultMarkdownImageWidthPercent,
  parseMarkdownImage,
  resolveMarkdownImagePath,
  replaceMarkdownImageWidth,
  resolveRenderedMarkdownImageWidth,
} from './markdownMedia.ts';

test('parses 3-digit width percentages from markdown image attributes', () => {
  const parsed = parseMarkdownImage('![Chart](./assets/chart.png){width=125%}');

  assert.deepEqual(parsed, {
    alt: 'Chart',
    url: './assets/chart.png',
    widthPercent: 125,
  });
});

test('writes 100 percent as an explicit markdown image width', () => {
  const next = replaceMarkdownImageWidth('![Chart](./assets/chart.png)', 100);

  assert.equal(next, '![Chart](./assets/chart.png){width=100%}');
});

test('writes larger explicit markdown image widths without collapsing to auto', () => {
  const next = replaceMarkdownImageWidth('![Chart](./assets/chart.png){width=75%}', 150);

  assert.equal(next, '![Chart](./assets/chart.png){width=150%}');
});

test('uses chat default width when markdown source omits width attrs', () => {
  const next = resolveRenderedMarkdownImageWidth(null, {
    defaultWidthPercent: DEFAULT_CHAT_MARKDOWN_IMAGE_WIDTH_PERCENT,
  });

  assert.equal(next, 10);
});

test('explicit markdown width wins over chat default width', () => {
  const next = resolveRenderedMarkdownImageWidth(50, {
    defaultWidthPercent: DEFAULT_CHAT_MARKDOWN_IMAGE_WIDTH_PERCENT,
  });

  assert.equal(next, 50);
});

test('uses chat image default only for conversation markdown files', () => {
  assert.equal(
    getDefaultMarkdownImageWidthPercent('/Users/example/project/.agent/chat/thread.md'),
    10,
  );
  assert.equal(
    getDefaultMarkdownImageWidthPercent('.agent/chat/thread.md'),
    10,
  );
  assert.equal(
    getDefaultMarkdownImageWidthPercent('/Users/example/project/notes/thread.md'),
    null,
  );
});

test('resolves relative markdown image paths against the current file', () => {
  const next = resolveMarkdownImagePath('/Users/example/notes/index.md', './assets/chart.png');

  assert.equal(next, '/Users/example/notes/assets/chart.png');
});

test('keeps absolute markdown image paths absolute', () => {
  const next = resolveMarkdownImagePath('/Users/example/notes/index.md', '/tmp/assets/photo.webp');

  assert.equal(next, '/tmp/assets/photo.webp');
});

test('rejects non-file or non-image markdown image paths', () => {
  assert.equal(resolveMarkdownImagePath('/Users/example/notes/index.md', './assets/readme.txt'), null);
  assert.equal(resolveMarkdownImagePath('/Users/example/notes/index.md', 'https://example.com/image.png'), null);
});
