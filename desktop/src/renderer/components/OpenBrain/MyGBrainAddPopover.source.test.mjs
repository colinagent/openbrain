import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './MyGBrainAddPopover.tsx'),
  'utf8',
);

test('public brain directory supports subscribe and unsubscribe actions', () => {
  assert.match(source, /onUnsubscribePublicBrain: \(ownerUID: string\) => Promise<void>;/);
  assert.match(source, /entry\.subscribed\s*\?\s*onUnsubscribePublicBrain\(entry\.ownerUID\)\s*:\s*onSubscribePublicBrain\(entry\.ownerUID\)/);
  assert.doesNotMatch(source, /disabled=\{actionBusy \|\| busy \|\| entry\.subscribed\}/);
  assert.match(source, /\{entry\.subscribed \? 'Remove' : '\+'\}/);
  assert.match(source, /\{entry\.name\}/);
  assert.match(source, /entry\.avatar/);
  assert.match(source, /PublicBrainDirectoryAvatar/);
  assert.match(source, /@\{entry\.username\}/);
  assert.match(source, /\{entry\.description\}/);
  assert.match(source, /No other public brains found\./);
  assert.doesNotMatch(source, /ownerLabel/);
});

test('add popover reads anchor position before paint and uses viewport-fixed layers', () => {
  assert.match(source, /useLayoutEffect/);
  assert.match(source, /computePopoverLayout\(anchor, panel\)/);
  assert.match(source, /openbrain-add-popover-panel--measuring/);
  assert.match(source, /openbrain-add-popover-scrim/);
  assert.match(source, /openbrain-add-popover-panel/);
  assert.match(source, /useDismissOnOutsideInteraction/);
  assert.doesNotMatch(source, /transform:\s*['"]translate\(-50%/);
});

test('add popover does not duplicate the MyGBrain chat action', () => {
  assert.doesNotMatch(source, /onOpenMyChat/);
  assert.doesNotMatch(source, /Open my chat/);
  assert.doesNotMatch(source, /openbrain-add-popover-chat-link/);
});
