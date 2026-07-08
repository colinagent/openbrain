import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildFileReferenceLink,
  buildSelectionReferenceLink,
} from './chatReferenceLinks';
import type { ChatSelectionSnapshot } from './chatSelectionPrompt';

test('buildSelectionReferenceLink writes only a markdown source link', () => {
  const snapshot: ChatSelectionSnapshot = {
    kind: 'text',
    text: 'type CreateMessage',
    from: 12,
    to: 30,
    startLine: 358,
    endLine: 362,
  };

  const link = buildSelectionReferenceLink(snapshot, '/tmp/protocol.go');

  assert.equal(link, '[protocol.go#L358-L362](/tmp/protocol.go)');
  assert.doesNotMatch(link, /type CreateMessage/);
  assert.doesNotMatch(link, /```/);
});

test('buildFileReferenceLink writes a native markdown file link', () => {
  const link = buildFileReferenceLink('/tmp/src/app.ts', false);

  assert.equal(link, '[file: app.ts](/tmp/src/app.ts)');
});

test('buildFileReferenceLink writes a native markdown directory link', () => {
  const link = buildFileReferenceLink('/tmp/src/components', true);

  assert.equal(link, '[directory: components](/tmp/src/components)');
});

test('buildSelectionReferenceLink does not include oversized selection content', () => {
  const snapshot: ChatSelectionSnapshot = {
    kind: 'text',
    text: Array.from({ length: 360 }, (_, index) => `line ${index + 1}`).join('\n'),
    from: 0,
    to: 1000,
    startLine: 1,
    endLine: 360,
  };

  const link = buildSelectionReferenceLink(snapshot, '/tmp/long.ts');

  assert.equal(link, '[long.ts#L1-L360](/tmp/long.ts)');
  assert.doesNotMatch(link, /truncated/);
  assert.doesNotMatch(link, /line 1/);
  assert.doesNotMatch(link, /line 360/);
});
