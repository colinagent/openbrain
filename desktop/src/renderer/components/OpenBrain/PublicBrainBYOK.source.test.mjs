import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';

const serviceSource = fs.readFileSync(new URL('../../services/openBrainService.ts', import.meta.url), 'utf8');
const dialogSource = fs.readFileSync(new URL('./PublicBrainHostedChatDialog.tsx', import.meta.url), 'utf8');

test('public brain BYOK stays in the active runtime and uses retrieval-only Cloud semantics', () => {
  assert.match(serviceSource, /executionMode: 'hosted' \| 'runtime_byok'/);
  assert.match(serviceSource, /conversations\/\$\{encodeURIComponent\(conversationID\.trim\(\)\)\}\/byok-turns/);
  assert.match(serviceSource, /billingResponsibility: 'external_provider'/);
  assert.match(serviceSource, /\/v1\/openbrain\/runtime\/models/);
  assert.match(dialogSource, /listOpenBrainRuntimeModels/);
  assert.match(dialogSource, /workspaceTabIDRef\.current/);
  assert.match(dialogSource, /runOpenBrainPublicBrainBYOKTurn/);
  assert.match(dialogSource, /Cloud retrieval/);
  assert.match(dialogSource, /credentials never enter the Cloud Brain API/);
  assert.match(dialogSource, /runtime_byok_not_allowed/);
  assert.doesNotMatch(dialogSource, /apiKey\s*:/);
});
