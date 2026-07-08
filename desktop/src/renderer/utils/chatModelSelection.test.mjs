import test from 'node:test';
import assert from 'node:assert/strict';

import { resolveChatModelSelection, resolveDefaultChatModelSelection } from './chatModelSelection.ts';

const MODELS = [
  { key: 'cloud:gpt-5', id: 'gpt-5', enabled: true, label: 'GPT-5', provider: 'cloud' },
  { key: 'local-claude:claude-4', id: 'claude-4', enabled: true, label: 'Claude 4', provider: 'local-claude' },
];

test('prefers explicit model selection', () => {
  const result = resolveChatModelSelection(MODELS, 'local-claude:claude-4');
  assert.equal(result.effectiveModelKey, 'local-claude:claude-4');
  assert.equal(result.source, 'explicit');
});

test('preserves explicit selection even when it is not in enabled models', () => {
  const result = resolveChatModelSelection(MODELS, 'legacy-provider:legacy-model');
  assert.equal(result.effectiveModelKey, 'legacy-provider:legacy-model');
  assert.equal(result.effectiveModel, null);
  assert.equal(result.source, 'explicit');
});

test('returns null when no explicit model is selected', () => {
  const result = resolveChatModelSelection(MODELS, null);
  assert.equal(result.effectiveModelKey, null);
  assert.equal(result.effectiveModel, null);
  assert.equal(result.source, null);
});

test('returns null when there is no enabled model and no explicit selection', () => {
  const result = resolveChatModelSelection([], null);
  assert.equal(result.effectiveModelKey, null);
  assert.equal(result.effectiveModel, null);
  assert.equal(result.source, null);
});

test('resolves enabled Default Chat Model from model strategies', () => {
  const result = resolveDefaultChatModelSelection({
    version: 5,
    defaultModelKey: null,
    providers: {},
    models: MODELS,
    strategies: { auto: { defaultChatModelID: 'cloud:gpt-5' } },
    updatedAt: 0,
  });
  assert.equal(result.modelKey, 'cloud:gpt-5');
  assert.equal(result.model?.key, 'cloud:gpt-5');
  assert.equal(result.errorMessage, null);
});

test('does not resolve disabled or missing Default Chat Model', () => {
  const result = resolveDefaultChatModelSelection({
    version: 5,
    defaultModelKey: null,
    providers: {},
    models: [{ ...MODELS[0], enabled: false }],
    strategies: { auto: { defaultChatModelID: 'cloud:gpt-5' } },
    updatedAt: 0,
  });
  assert.equal(result.modelKey, null);
  assert.equal(result.model, null);
  assert.match(result.errorMessage || '', /Default Chat Model "cloud:gpt-5" is not available/);
});
