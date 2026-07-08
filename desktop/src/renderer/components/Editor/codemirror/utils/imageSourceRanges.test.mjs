import assert from 'node:assert/strict';
import test from 'node:test';

import {
  isImageSourceActive,
  matchHeadingAvatarImage,
  matchLeadingMarkdownImage,
} from './imageSourceRanges.ts';

test('matches a standalone markdown image source range', () => {
  const input = '![Chart](./assets/chart.png){width=50%}';
  const match = matchLeadingMarkdownImage(input);

  assert.ok(match);
  assert.equal(match.trailingText, '');
  assert.equal(input.slice(match.sourceFrom, match.sourceTo), input);
  assert.equal(input.slice(match.replaceFrom, match.replaceTo), input);
  assert.equal(match.widthPercent, 50);
});

test('parses 3-digit image width percentages for live preview widgets', () => {
  const input = '![Chart](./assets/chart.png){width=150%}';
  const match = matchLeadingMarkdownImage(input);

  assert.ok(match);
  assert.equal(match.widthPercent, 150);
  assert.equal(input.slice(match.sourceFrom, match.sourceTo), input);
});

test('keeps inline avatar spacing out of the editable image source range', () => {
  const input = '![Op](/.openbrain/resources/avatars/op.png)  OpenBrain';
  const match = matchLeadingMarkdownImage(input);

  assert.ok(match);
  assert.equal(match.trailingText, 'OpenBrain');
  assert.equal(
    input.slice(match.sourceFrom, match.sourceTo),
    '![Op](/.openbrain/resources/avatars/op.png)'
  );
  assert.equal(
    input.slice(match.replaceFrom, match.replaceTo),
    '![Op](/.openbrain/resources/avatars/op.png)'
  );
  assert.equal(
    isImageSourceActive(
      { from: match.replaceTo + 1, to: match.replaceTo + 1, head: match.replaceTo + 1, empty: true },
      match.sourceFrom,
      match.sourceTo
    ),
    false
  );
});

test('matches heading avatar images without exposing source on heading text focus', () => {
  const input = '![Op](/.openbrain/resources/avatars/op.png)  Release Notes';
  const match = matchHeadingAvatarImage(input);

  assert.ok(match);
  assert.equal(
    input.slice(match.sourceFrom, match.sourceTo),
    '![Op](/.openbrain/resources/avatars/op.png)'
  );
  assert.equal(match.trailingText, 'Release Notes');
});

test('treats only the image token range as active', () => {
  assert.equal(
    isImageSourceActive({ from: 12, to: 12, head: 12, empty: true }, 10, 20),
    true
  );
  assert.equal(
    isImageSourceActive({ from: 21, to: 21, head: 21, empty: true }, 10, 20),
    false
  );
  assert.equal(
    isImageSourceActive({ from: 18, to: 24, head: 24, empty: false }, 10, 20),
    true
  );
  assert.equal(
    isImageSourceActive({ from: 21, to: 30, head: 30, empty: false }, 10, 20),
    false
  );
});
