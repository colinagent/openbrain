import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildModelSelectOption,
  getDisplayModelKeyText,
  getModelDisplayInfo,
  getVisibleProviderLabel,
} from './modelDisplay';

test('getModelDisplayInfo uses model name as primary text and model id as secondary text', () => {
  assert.deepEqual(getModelDisplayInfo('ltp-claude', 'claude-opus-4-6', 'Local Anthropic'), {
    primaryText: 'claude-opus-4-6',
    secondaryText: 'ltp-claude',
    providerText: 'Local Anthropic',
    triggerText: 'claude-opus-4-6',
    titleText: 'claude-opus-4-6 · ltp-claude · Local Anthropic',
  });
});

test('getModelDisplayInfo hides duplicate model id text', () => {
  assert.deepEqual(getModelDisplayInfo('claude-opus-4-6', 'claude-opus-4-6', 'Cloud'), {
    primaryText: 'claude-opus-4-6',
    secondaryText: null,
    providerText: 'Cloud',
    triggerText: 'claude-opus-4-6',
    titleText: 'claude-opus-4-6 · Cloud',
  });
});

test('getModelDisplayInfo hides missing model name text', () => {
  assert.deepEqual(getModelDisplayInfo('gpt-5.4', '', 'Cloud'), {
    primaryText: 'gpt-5.4',
    secondaryText: null,
    providerText: 'Cloud',
    triggerText: 'gpt-5.4',
    titleText: 'gpt-5.4 · Cloud',
  });
});

test('buildModelSelectOption uses model name for the label and provider for description', () => {
  assert.deepEqual(
    buildModelSelectOption({
      key: 'local-openai:ltp-claude',
      id: 'ltp-claude',
      label: 'claude-opus-4-6',
      provider: 'local-openai',
      providerLabel: 'Local OpenAI',
    }),
    {
      value: 'local-openai:ltp-claude',
      label: 'claude-opus-4-6',
      description: 'ltp-claude · Local OpenAI',
      title: 'claude-opus-4-6 · ltp-claude · Local OpenAI',
    },
  );
});

test('getDisplayModelKeyText shows the provider prefix for Cloud models', () => {
  assert.equal(getDisplayModelKeyText('cloud:gpt-5.4', 'Cloud'), 'cloud:gpt-5.4');
});

test('getVisibleProviderLabel shows the Cloud provider label', () => {
  assert.equal(getVisibleProviderLabel('cloud', 'Cloud'), 'Cloud');
});

test('getVisibleProviderLabel ignores stale labels for Cloud models', () => {
  assert.equal(getVisibleProviderLabel('cloud', 'OpenBrain'), 'Cloud');
});

test('getDisplayModelKeyText keeps the real provider key even when the label differs', () => {
  assert.equal(getDisplayModelKeyText('codemirror-gpt5.4:gpt-5.4', 'codemirror'), 'codemirror-gpt5.4:gpt-5.4');
});

test('getDisplayModelKeyText uses the provider key for managed organization models', () => {
  assert.equal(getDisplayModelKeyText('acme:gpt-5.4', 'Acme'), 'acme:gpt-5.4');
});
