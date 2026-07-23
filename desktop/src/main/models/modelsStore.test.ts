import assert from 'node:assert/strict';
import test from 'node:test';

import { mergeOpenBrainModels, mergeOpenBrainOrgCatalogs, normalizeModelsConfig } from './modelsStore';

test('normalizeModelsConfig preserves Cloud reasoning levels exactly as configured', () => {
  const config = normalizeModelsConfig({
    version: 5,
    defaultModelKey: 'cloud:gpt-5.4',
    models: [
      {
        key: 'cloud:gpt-5.4',
        id: 'gpt-5.4',
        enabled: true,
        provider: 'cloud',
        providerLabel: 'Cloud',
        api: 'openai-responses',
        reasoning: true,
        reasoningControl: 'level',
        reasoningLevels: ['minimal', 'low', 'medium', 'high', 'xhigh'],
      },
    ],
    updatedAt: 0,
  });

  const model = config.models.find((entry) => entry.id === 'gpt-5.4');
  assert.ok(model);
  assert.equal(model?.key, 'cloud:gpt-5.4');
  assert.equal(model?.reasoning, true);
  assert.equal(model?.reasoningControl, 'level');
  assert.deepEqual(model?.reasoningLevels, ['minimal', 'low', 'medium', 'high', 'xhigh']);
});

test('normalizeModelsConfig forces the Cloud provider label for hosted models', () => {
  const config = normalizeModelsConfig({
    version: 5,
    defaultModelKey: 'cloud:gpt-5.4',
    models: [
      {
        key: 'cloud:gpt-5.4',
        id: 'gpt-5.4',
        enabled: true,
        provider: 'cloud',
        providerLabel: 'Hosted',
        api: 'openai-responses',
        reasoning: true,
      },
    ],
    updatedAt: 0,
  });

  assert.equal(config.providers.cloud.label, 'Cloud');
  assert.equal(config.models[0]?.providerLabel, 'Cloud');
});

test('normalizeModelsConfig rejects a disabled Default Chat Model', () => {
  assert.throws(
    () => normalizeModelsConfig({
      version: 5,
      defaultModelKey: 'cloud:gpt-5-mini',
      strategies: {
        auto: {
          defaultChatModelID: 'cloud:gpt-5.5',
        },
      },
      models: [
        {
          key: 'cloud:gpt-5.5',
          id: 'gpt-5.5',
          enabled: false,
          provider: 'cloud',
          providerLabel: 'Cloud',
          api: 'openai-responses',
          reasoning: true,
        },
        {
          key: 'cloud:gpt-5-mini',
          id: 'gpt-5-mini',
          enabled: true,
          provider: 'cloud',
          providerLabel: 'Cloud',
          api: 'openai-responses',
          reasoning: true,
        },
      ],
      updatedAt: 0,
    }),
    /Default Chat Model cloud:gpt-5.5 must reference an enabled model/,
  );
});

test('normalizeModelsConfig infers toggle reasoning control for reasoning models without levels', () => {
  const config = normalizeModelsConfig({
    version: 5,
    defaultModelKey: 'openai:kimi-4.6',
    models: [
      {
        key: 'openai:kimi-4.6',
        id: 'kimi-4.6',
        enabled: true,
        provider: 'openai',
        api: 'openai-completions',
        reasoning: true,
        baseUrl: 'https://example.com/v1',
        apiKey: 'secret',
      },
    ],
    updatedAt: 0,
  });

  const model = config.models.find((entry) => entry.id === 'kimi-4.6');
  assert.ok(model);
  assert.equal(model?.reasoningControl, 'toggle');
});

test('normalizeModelsConfig flattens provider-based schema into runtime model entries', () => {
  const config = normalizeModelsConfig({
    version: 5,
    defaultModelKey: 'openai:gpt-5.4',
    providers: {
      openai: {
        api: 'openai-responses',
        baseUrl: 'https://api.openai.com/v1',
        apiKey: 'secret',
        models: [
          {
            id: 'gpt-5.4',
            label: 'GPT 5.4',
            enabled: true,
            reasoning: true,
            reasoningControl: 'level',
            reasoningLevels: ['minimal', 'low'],
            serviceTiers: ['priority'],
            maxOutputTokens: 64000,
          },
        ],
      },
    },
    updatedAt: 0,
  } as never);

  assert.equal(config.defaultModelKey, 'openai:gpt-5.4');
  assert.ok(config.providers.openai);
  assert.equal(config.providers.openai.baseUrl, 'https://api.openai.com/v1');
  const model = config.models.find((entry) => entry.key === 'openai:gpt-5.4');
  assert.ok(model);
  assert.deepEqual(model?.serviceTiers, ['priority']);
  assert.equal(model?.maxOutputTokens, 64000);
  assert.deepEqual(config.providers.openai.models[0]?.serviceTiers, ['priority']);
});

test('normalizeModelsConfig keeps bare managed org provider keys', () => {
  const config = normalizeModelsConfig({
    version: 5,
    defaultModelKey: 'acme:gpt-5.4',
    providers: {
      acme: {
        label: 'Acme',
        managed: true,
        baseUrl: 'https://should-be-ignored.example/v1',
        apiKey: 'should-be-ignored',
        models: [
          {
            key: 'acme:gpt-5.4',
            id: 'gpt-5.4',
            enabled: true,
            api: 'openai-responses',
            reasoning: true,
          },
        ],
      },
    },
    updatedAt: 0,
  } as never);

  assert.equal(config.defaultModelKey, 'acme:gpt-5.4');
  assert.ok(config.providers.acme);
  assert.equal(config.providers.acme.managed, true);
  assert.equal(config.providers.acme.baseUrl, undefined);
  assert.equal(config.providers.acme.apiKey, undefined);
  assert.deepEqual(Object.keys(config.providers), ['acme']);
  const model = config.models.find((entry) => entry.key === 'acme:gpt-5.4');
  assert.ok(model);
  assert.equal(model?.provider, 'acme');
  assert.equal(model?.baseUrl, undefined);
  assert.equal(model?.apiKey, undefined);
});

test('mergeOpenBrainModels preserves remote context options, service tiers, and max output token limits', () => {
  const merged = mergeOpenBrainModels(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: null,
      models: [],
      updatedAt: 0,
    }),
    {
      models: [
        {
          id: 'claude-opus-4-6',
          api: 'anthropic-messages',
          reasoning: true,
          reasoningLevels: ['low', 'medium', 'high', 'max'],
          contextWindows: [300000, 1000000],
          defaultContextWindow: 300000,
          serviceTiers: ['priority'],
          maxOutputTokens: 128000,
        },
      ],
    },
    1,
  );

  const model = merged.models.find((entry) => entry.key === 'cloud:claude-opus-4-6');
  assert.ok(model);
  assert.deepEqual(model?.contextWindows, [300000, 1000000]);
  assert.equal(model?.defaultContextWindow, 300000);
  assert.deepEqual(model?.serviceTiers, ['priority']);
  assert.equal(model?.maxOutputTokens, 128000);
});

test('mergeOpenBrainModels does not infer reasoning when provider capabilities are missing', () => {
  const merged = mergeOpenBrainModels(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: null,
      models: [],
      updatedAt: 0,
    }),
    {
      models: [
        {
          id: 'claude-opus-4-6',
          api: 'openai-completions',
          reasoning: false,
        },
      ],
    },
    1,
  );

  const model = merged.models.find((entry) => entry.id === 'claude-opus-4-6');
  assert.ok(model);
  assert.equal(model?.key, 'cloud:claude-opus-4-6');
  assert.equal(model?.enabled, true);
  assert.equal(model?.reasoning, false);
  assert.equal(model?.reasoningControl, undefined);
  assert.equal(model?.reasoningLevels, undefined);
});

test('mergeOpenBrainModels keeps Cloud and user-defined provider entries with the same canonical id', () => {
  const merged = mergeOpenBrainModels(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: 'local-claude:claude-opus-4-6',
      models: [
        {
          key: 'local-claude:claude-opus-4-6',
          id: 'claude-opus-4-6',
          enabled: true,
          provider: 'local-claude',
          providerLabel: 'Local Claude',
          api: 'anthropic-messages',
          reasoning: true,
          reasoningLevels: ['low', 'medium', 'high', 'max'],
          baseUrl: 'https://api.anthropic.com/v1',
          apiKey: 'secret',
        },
      ],
      updatedAt: 0,
    }),
    {
      models: [
        {
          id: 'claude-opus-4-6',
          api: 'openai-completions',
          reasoning: true,
          reasoningLevels: ['low', 'medium', 'high', 'max'],
        },
      ],
    },
    1,
  );

  const matchingModels = merged.models.filter((entry) => entry.id === 'claude-opus-4-6');
  assert.equal(matchingModels.length, 2);
  assert.deepEqual(
    matchingModels.map((entry) => entry.key).sort(),
    ['cloud:claude-opus-4-6', 'local-claude:claude-opus-4-6'],
  );
});

test('mergeOpenBrainModels preserves existing disabled Cloud models on refresh', () => {
  const merged = mergeOpenBrainModels(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: 'cloud:gpt-5.4',
      models: [
        {
          key: 'cloud:gpt-5.4',
          id: 'gpt-5.4',
          enabled: false,
          provider: 'cloud',
          providerLabel: 'Cloud',
          api: 'openai-responses',
          reasoning: true,
          reasoningLevels: ['minimal', 'low', 'medium', 'high', 'xhigh'],
        },
      ],
      updatedAt: 0,
    }),
    {
      models: [
        {
          id: 'gpt-5.4',
          api: 'openai-responses',
          reasoning: true,
          reasoningLevels: ['minimal', 'low', 'medium', 'high', 'xhigh'],
        },
      ],
    },
    1,
  );

  const model = merged.models.find((entry) => entry.key === 'cloud:gpt-5.4');
  assert.ok(model);
  assert.equal(model?.enabled, false);
  assert.equal(merged.defaultModelKey, null);
});

test('normalizeModelsConfig keeps provider-first shape when models under one provider use different endpoints', () => {
  const config = normalizeModelsConfig({
    version: 5,
    defaultModelKey: 'codemirror:gpt-5.4',
    providers: {
      codemirror: {
        label: 'codemirror',
        apiKey: 'secret',
        models: [
          {
            id: 'gpt-5.4',
            enabled: true,
            api: 'openai-responses',
            baseUrl: 'https://api.aicodemirror.com/api/codex/backend-api/codex',
            reasoning: true,
          },
          {
            id: 'claude-opus-4-6',
            enabled: true,
            api: 'anthropic-messages',
            baseUrl: 'https://api.aicodemirror.com/api/claudecode',
            reasoning: true,
          },
        ],
      },
    },
    updatedAt: 0,
  } as never);

  assert.equal(config.providers.codemirror.baseUrl, undefined);
  assert.equal(config.providers.codemirror.apiKey, 'secret');
  const codemirrorProviderModels = config.providers.codemirror.models;
  const gpt = codemirrorProviderModels.find((model) => model.id === 'gpt-5.4');
  const claude = codemirrorProviderModels.find((model) => model.id === 'claude-opus-4-6');
  assert.equal(gpt?.baseUrl, 'https://api.aicodemirror.com/api/codex/backend-api/codex');
  assert.equal(claude?.baseUrl, 'https://api.aicodemirror.com/api/claudecode');
  assert.equal(config.models.find((entry) => entry.key === 'codemirror:gpt-5.4')?.baseUrl, 'https://api.aicodemirror.com/api/codex/backend-api/codex');
  assert.equal(config.models.find((entry) => entry.key === 'codemirror:claude-opus-4-6')?.baseUrl, 'https://api.aicodemirror.com/api/claudecode');
});

test('mergeOpenBrainModels refreshes strategy snapshots from the Cloud catalog', () => {
  const merged = mergeOpenBrainModels(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: 'cloud:gpt-5.4',
      models: [
        {
          key: 'cloud:gpt-5.4',
          id: 'gpt-5.4',
          enabled: true,
          provider: 'cloud',
          providerLabel: 'Cloud',
          api: 'openai-responses',
          reasoning: true,
        },
      ],
      updatedAt: 0,
    }),
    {
      models: [
        {
          id: 'gpt-5.4',
          api: 'openai-responses',
          reasoning: true,
        },
      ],
      strategies: {
        auto: {
          defaultChatModelID: 'gpt-5.4',
          defaultInlineCompletionModelID: 'gpt-5.4',
        },
      },
    },
    1,
  );

  assert.equal(merged.strategies?.auto?.defaultChatModelID, 'cloud:gpt-5.4');
  assert.equal(merged.strategies?.auto?.defaultInlineCompletionModelID, 'cloud:gpt-5.4');
});

test('mergeOpenBrainOrgCatalogs adds organization models as managed providers', () => {
  const merged = mergeOpenBrainOrgCatalogs(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: null,
      providers: {},
      updatedAt: 0,
    } as never),
    [
      {
        providerKey: 'cloud',
        providerLabel: 'Cloud',
        models: [
          {
            id: 'gpt-5.4',
            api: 'openai-responses',
            reasoning: true,
          },
        ],
      },
      {
        providerKey: 'acme',
        providerLabel: 'Acme',
        models: [
          {
            id: 'gpt-5.4',
            api: 'openai-responses',
            reasoning: true,
            reasoningLevels: ['minimal', 'low'],
          },
        ],
      },
    ],
    1,
    { activeOrgID: 'acme' },
  );

  const orgModel = merged.models.find((entry) => entry.key === 'acme:gpt-5.4');
  assert.ok(orgModel);
  assert.equal(merged.defaultModelKey, 'acme:gpt-5.4');
  assert.equal(orgModel?.provider, 'acme');
  assert.equal(orgModel?.providerLabel, 'Acme');
  assert.equal(orgModel?.baseUrl, undefined);
  assert.equal(orgModel?.apiKey, undefined);
  assert.equal(merged.providers['acme'].label, 'Acme');
  assert.equal(merged.providers['acme'].managed, true);
  assert.equal(merged.providers['acme'].baseUrl, undefined);
  assert.equal(merged.providers['acme'].apiKey, undefined);
});

test('mergeOpenBrainOrgCatalogs preserves custom provider default over active organization', () => {
  const merged = mergeOpenBrainOrgCatalogs(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: 'local-openai:gpt-5.4',
      providers: {
        'local-openai': {
          label: 'Local OpenAI',
          api: 'openai-responses',
          baseUrl: 'https://api.openai.com/v1',
          apiKey: 'local-key',
          models: [
            {
              id: 'gpt-5.4',
              enabled: true,
              reasoning: true,
            },
          ],
        },
      },
      updatedAt: 0,
    } as never),
    [
      {
        providerKey: 'acme',
        providerLabel: 'Acme',
        models: [
          {
            id: 'gpt-5.4',
            api: 'openai-responses',
            reasoning: true,
          },
        ],
      },
    ],
    1,
    { activeOrgID: 'acme' },
  );

  assert.equal(merged.defaultModelKey, 'local-openai:gpt-5.4');
  assert.ok(merged.models.find((entry) => entry.key === 'acme:gpt-5.4'));
});

test('mergeOpenBrainOrgCatalogs removes stale managed organizations after a tenant switch', () => {
  const merged = mergeOpenBrainOrgCatalogs(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: 'org-a:gpt-a',
      providers: {
        'org-a': {
          label: 'Org A',
          managed: true,
          models: [
            {
              id: 'gpt-a',
              enabled: true,
              api: 'openai-responses',
            },
          ],
        },
        'local-openai': {
          label: 'Local OpenAI',
          api: 'openai-responses',
          baseUrl: 'https://api.openai.com/v1',
          apiKey: 'local-key',
          models: [
            {
              id: 'gpt-local',
              enabled: true,
            },
          ],
        },
      },
      updatedAt: 0,
    } as never),
    [
      {
        providerKey: 'org-b',
        providerLabel: 'Org B',
        models: [
          {
            id: 'gpt-b',
            api: 'openai-responses',
            reasoning: false,
          },
        ],
      },
    ],
    1,
    { activeOrgID: 'org-b' },
  );

  assert.deepEqual(Object.keys(merged.providers).sort(), ['local-openai', 'org-b']);
  assert.deepEqual(merged.models.map((model) => model.key).sort(), ['local-openai:gpt-local', 'org-b:gpt-b']);
  assert.equal(merged.defaultModelKey, 'org-b:gpt-b');
  assert.equal(merged.providers['org-b'].managed, true);
  assert.equal(merged.providers['org-a'], undefined);
  assert.doesNotThrow(() => normalizeModelsConfig(merged));
});

test('mergeOpenBrainOrgCatalogs builds bare org provider keys from short remote model ids', () => {
  const merged = mergeOpenBrainOrgCatalogs(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: null,
      models: [],
      updatedAt: 0,
    }),
    [
      {
        providerKey: 'ltp',
        providerLabel: 'LTP',
        models: [
          {
            id: 'gpt-5.5',
            label: 'GPT-5.5',
            api: 'openai-responses',
            reasoning: true,
            reasoningLevels: ['low', 'medium', 'high', 'xhigh'],
          },
        ],
      },
    ],
    1,
    { activeOrgID: 'ltp' },
  );

  assert.equal(merged.defaultModelKey, 'ltp:gpt-5.5');
  assert.deepEqual(Object.keys(merged.providers), ['ltp']);
  assert.equal(merged.providers.ltp.managed, true);
  assert.deepEqual(merged.models.map((model) => model.key), ['ltp:gpt-5.5']);
  assert.equal(merged.models[0]?.id, 'gpt-5.5');
});

test('mergeOpenBrainOrgCatalogs replaces stale managed cache from the current catalog', () => {
  const merged = mergeOpenBrainOrgCatalogs(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: 'ltp:old-model',
      providers: {
        ltp: {
          label: 'LTP',
          managed: true,
          models: [
            {
              key: 'ltp:old-model',
              id: 'old-model',
              label: 'Old model',
              enabled: true,
              api: 'openai-responses',
              reasoning: true,
            },
          ],
        },
      },
      strategies: {
        auto: {
          defaultChatModelID: 'ltp:old-model',
        },
      },
      updatedAt: 0,
    } as never),
    [
      {
        providerKey: 'ltp',
        providerLabel: 'LTP',
        models: [
          {
            id: 'gpt-5.5',
            label: 'GPT-5.5',
            api: 'openai-responses',
            reasoning: true,
          },
        ],
        strategies: {
          auto: {
            defaultChatModelID: 'gpt-5.5',
          },
        },
      },
    ],
    1,
    { activeOrgID: 'ltp' },
  );

  assert.deepEqual(merged.models.map((model) => model.key), ['ltp:gpt-5.5']);
  assert.equal(merged.models[0]?.id, 'gpt-5.5');
  assert.equal(merged.models[0]?.label, 'GPT-5.5');
  assert.equal(merged.defaultModelKey, 'ltp:gpt-5.5');
  assert.equal(merged.strategies?.auto?.defaultChatModelID, 'ltp:gpt-5.5');
});

test('normalizeModelsConfig preserves upstream-namespaced ids for custom providers', () => {
  const config = normalizeModelsConfig({
    version: 5,
    defaultModelKey: 'openai:openai/gpt-5.5',
    providers: {
      openai: {
        api: 'openai-responses',
        baseUrl: 'https://api.openai.com/v1',
        apiKey: 'secret',
        models: [
          {
            id: 'openai/gpt-5.5',
            enabled: true,
            reasoning: true,
          },
        ],
      },
    },
    updatedAt: 0,
  } as never);

  assert.equal(config.defaultModelKey, 'openai:openai/gpt-5.5');
  assert.deepEqual(config.models.map((model) => model.key), ['openai:openai/gpt-5.5']);
  assert.equal(config.models[0]?.id, 'openai/gpt-5.5');
});

test('mergeOpenBrainOrgCatalogs privateOnly preserves runtime-local providers', () => {
  const merged = mergeOpenBrainOrgCatalogs(
    normalizeModelsConfig({
      version: 5,
      defaultModelKey: 'lt:gpt-5.5',
      providers: {
        lt: {
          label: 'lt',
          managed: true,
          models: [
            {
              id: 'gpt-5.5',
              enabled: true,
              api: 'openai-responses',
              reasoning: true,
            },
          ],
        },
        'local-openai': {
          label: 'Local OpenAI',
          api: 'openai-responses',
          baseUrl: 'https://api.openai.com/v1',
          apiKey: 'local-key',
          models: [
            {
              id: 'gpt-5.4',
              enabled: true,
              reasoning: true,
            },
          ],
        },
      },
      strategies: {
        auto: {
          defaultChatModelID: 'lt:gpt-5.5',
        },
      },
      updatedAt: 0,
    } as never),
    [
      {
        providerKey: 'cloud',
        providerLabel: 'Cloud',
        models: [
          {
            id: 'MiniMax-M2.7-highspeed',
            api: 'anthropic-messages',
            reasoning: true,
          },
        ],
        strategies: {
          auto: {
            defaultChatModelID: 'missing-from-catalog',
            defaultChatThinkingLevel: 'high',
          },
        },
      },
    ],
    1,
    { activeOrgID: 'cloud', privateOnly: true },
  );

  assert.deepEqual(Object.keys(merged.providers).sort(), ['cloud', 'local-openai']);
  assert.deepEqual(
    merged.models.map((model) => model.key).sort(),
    ['cloud:MiniMax-M2.7-highspeed', 'local-openai:gpt-5.4'],
  );
  assert.equal(merged.defaultModelKey, 'cloud:MiniMax-M2.7-highspeed');
  assert.equal(merged.strategies, undefined);
});

test('normalizeModelsConfig rejects unsupported schema versions', () => {
  assert.throws(
    () => normalizeModelsConfig({
      version: 1,
      defaultModelKey: null,
      models: [],
      updatedAt: 0,
    } as never),
    /version must be 5/,
  );
});
