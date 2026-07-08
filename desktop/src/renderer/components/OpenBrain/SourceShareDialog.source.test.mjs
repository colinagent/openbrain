import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './SourceShareDialog.tsx'),
  'utf8',
);

test('source share dialog owns public risk warning and source sharing controls', () => {
  assert.match(source, /Public brain/);
  assert.doesNotMatch(source, /Save name/);
  assert.doesNotMatch(source, /onSaveBrainName/);
  assert.match(source, /onUpdatePublicProfile/);
  assert.match(source, /Save description/);
  assert.match(source, /maxLength=\{280\}/);
  assert.match(source, /Make public/);
  assert.match(source, /Make private/);
  assert.match(source, /createPortal/);
  assert.match(source, /no-drag fixed inset-0 z-\[70\]/);
  assert.match(source, /secrets, private code, customer data, tokens, or personal information/);
  assert.match(source, /setRiskAcknowledged\(event\.currentTarget\.checked\)/);
  assert.match(source, /onShareEmail/);
  assert.match(source, /onRevokeUser/);
  assert.doesNotMatch(source, /window\.prompt/);
  assert.doesNotMatch(source, /window\.confirm/);
});
