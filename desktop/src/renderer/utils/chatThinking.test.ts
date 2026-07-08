import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getSupportedThinkingLevels,
  getThinkingPickerLevels,
  normalizeThinkingLevelForModel,
  UI_CHAT_THINKING_ON_LEVEL,
} from './chatThinking';

test('supported thinking levels preserve service order and raw values', () => {
  assert.deepEqual(
    getSupportedThinkingLevels({
      reasoning: true,
      reasoningLevels: ['minimal', 'low', 'low', 'xhigh', 'off', ''],
    }),
    ['minimal', 'low', 'xhigh'],
  );
});

test('supported thinking levels do not infer levels from reasoning without service values', () => {
  assert.deepEqual(
    getSupportedThinkingLevels({
      reasoning: true,
      reasoningControl: undefined,
      reasoningLevels: undefined,
    }),
    [],
  );
});

test('toggle reasoning models expose on/off thinking controls', () => {
  assert.deepEqual(
    getSupportedThinkingLevels({
      reasoning: true,
      reasoningControl: 'toggle',
      reasoningLevels: undefined,
    }),
    [UI_CHAT_THINKING_ON_LEVEL],
  );

  assert.deepEqual(
    getThinkingPickerLevels({
      reasoning: true,
      reasoningControl: 'toggle',
      reasoningLevels: undefined,
    }),
    ['off', UI_CHAT_THINKING_ON_LEVEL],
  );
});

test('thinking picker prepends off to service-provided levels', () => {
  assert.deepEqual(
    getThinkingPickerLevels({
      reasoning: true,
      reasoningLevels: ['minimal', 'xhigh'],
    }),
    ['off', 'minimal', 'xhigh'],
  );
});

test('normalizeThinkingLevelForModel falls back to off instead of mapping unsupported values', () => {
  const model = {
    reasoning: true,
    reasoningControl: 'level' as const,
    reasoningLevels: ['minimal', 'xhigh'],
  };

  assert.equal(normalizeThinkingLevelForModel(model, 'minimal'), 'minimal');
  assert.equal(normalizeThinkingLevelForModel(model, 'xhigh'), 'xhigh');
  assert.equal(normalizeThinkingLevelForModel(model, 'max'), 'off');
});

test('normalizeThinkingLevelForModel preserves on for toggle reasoning models', () => {
  const model = {
    reasoning: true,
    reasoningControl: 'toggle' as const,
    reasoningLevels: undefined,
  };

  assert.equal(normalizeThinkingLevelForModel(model, 'on'), 'on');
  assert.equal(normalizeThinkingLevelForModel(model, 'minimal'), 'off');
});
