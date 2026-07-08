import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

function resolveChatModelPreferencesPath() {
  const fromRepoRoot = path.resolve(
    process.cwd(),
    'desktop/src/renderer/utils/chatModelPreferences.ts'
  );
  const fromAppRoot = path.resolve(process.cwd(), 'src/renderer/utils/chatModelPreferences.ts');
  try {
    return readFileSync(fromRepoRoot, 'utf8') ? fromRepoRoot : fromAppRoot;
  } catch {
    return fromAppRoot;
  }
}

const source = readFileSync(resolveChatModelPreferencesPath(), 'utf8');

test('chat model preferences use a transient 200K fallback when catalog context options are absent', () => {
  assert.match(source, /export const FALLBACK_CONTEXT_WINDOW = 200_000;/);
  assert.match(source, /if \(options\.length === 0\) \{\s*return model \? FALLBACK_CONTEXT_WINDOW : null;\s*\}/m);
  assert.doesNotMatch(source, /getModelContextWindowOptions\(model: Pick<ModelEntry, 'contextWindow'/);
  assert.doesNotMatch(source, /model\?\.contextWindow/);
});
