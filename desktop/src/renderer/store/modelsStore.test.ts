import assert from 'node:assert/strict';
import test from 'node:test';

import { useModelsStore } from './modelsStore';
import type { ModelsConfig } from '../types/electron';

const emptyConfig = {
  version: 5,
  defaultModelKey: null,
  providers: {},
  models: [],
  updatedAt: 0,
};

function installElectronBridge(
  set: (config: unknown) => Promise<unknown> = async (config) => config,
  get?: () => Promise<unknown>,
) {
  const previousWindow = (globalThis as { window?: unknown }).window;
  (globalThis as { window?: unknown }).window = {
    electronAPI: {
      models: {
        ...(get ? { get } : {}),
        set,
      },
    },
  };
  return () => {
    (globalThis as { window?: unknown }).window = previousWindow;
  };
}

test('load reuses an in-flight models request', async () => {
  let getCalls = 0;
  let resolveGet: (config: unknown) => void = () => {};
  const config = {
    ...emptyConfig,
    defaultModelKey: 'cloud:gpt-5.4',
    models: [
      {
        key: 'cloud:gpt-5.4',
        id: 'gpt-5.4',
        label: 'GPT 5.4',
        enabled: true,
        provider: 'cloud',
        providerLabel: 'Cloud',
        api: 'openai-responses',
        reasoning: true,
        reasoningControl: 'level',
        reasoningLevels: ['minimal', 'low', 'medium', 'high', 'xhigh'],
      },
    ],
  };
  const restoreWindow = installElectronBridge(undefined, async () => {
    getCalls += 1;
    return new Promise((resolve) => {
      resolveGet = resolve;
    });
  });
  try {
    useModelsStore.setState({
      config: emptyConfig,
      loading: false,
      error: null,
    });

    const firstLoad = useModelsStore.getState().load();
    const secondLoad = useModelsStore.getState().load();
    assert.equal(getCalls, 1);
    assert.equal(useModelsStore.getState().loading, true);

    resolveGet(config);
    await Promise.all([firstLoad, secondLoad]);

    assert.equal(useModelsStore.getState().loading, false);
    assert.equal(useModelsStore.getState().config.defaultModelKey, 'cloud:gpt-5.4');
    assert.equal(useModelsStore.getState().config.models.length, 1);
  } finally {
    restoreWindow();
  }
});

test('toggleEnabled refuses to disable the Default Chat Model', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        models: [
          {
            key: 'cloud:gpt-5.5',
            id: 'gpt-5.5',
            label: 'GPT 5.5',
            enabled: true,
            provider: 'cloud',
            providerLabel: 'Cloud',
            api: 'openai-responses',
            reasoning: true,
          },
        ],
        strategies: {
          auto: {
            defaultChatModelID: 'cloud:gpt-5.5',
          },
        },
      },
      loading: false,
      error: null,
    });

    await assert.rejects(
      () => useModelsStore.getState().toggleEnabled('cloud:gpt-5.5'),
      /Default Chat Model cannot be disabled/,
    );

    const model = useModelsStore.getState().config.models.find((entry) => entry.key === 'cloud:gpt-5.5');
    assert.equal(model?.enabled, true);
  } finally {
    restoreWindow();
  }
});

test('toggleEnabled can disable a model after Default Chat Model points elsewhere', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        models: [
          {
            key: 'cloud:gpt-5.5',
            id: 'gpt-5.5',
            label: 'GPT 5.5',
            enabled: true,
            provider: 'cloud',
            providerLabel: 'Cloud',
            api: 'openai-responses',
            reasoning: true,
          },
          {
            key: 'cloud:gpt-5-mini',
            id: 'gpt-5-mini',
            label: 'GPT 5 mini',
            enabled: true,
            provider: 'cloud',
            providerLabel: 'Cloud',
            api: 'openai-responses',
            reasoning: true,
          },
        ],
        strategies: {
          auto: {
            defaultChatModelID: 'cloud:gpt-5-mini',
          },
        },
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().toggleEnabled('cloud:gpt-5.5');

    const model = useModelsStore.getState().config.models.find((entry) => entry.key === 'cloud:gpt-5.5');
    assert.equal(model?.enabled, false);
  } finally {
    restoreWindow();
  }
});

test('setDefaultChatModel changes the protected Default Chat Model and enables the target', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        models: [
          {
            key: 'cloud:gpt-5.5',
            id: 'gpt-5.5',
            label: 'GPT 5.5',
            enabled: true,
            provider: 'cloud',
            providerLabel: 'Cloud',
            api: 'openai-responses',
            reasoning: true,
          },
          {
            key: 'cloud:gpt-5-mini',
            id: 'gpt-5-mini',
            label: 'GPT 5 mini',
            enabled: false,
            provider: 'cloud',
            providerLabel: 'Cloud',
            api: 'openai-responses',
            reasoning: true,
          },
        ],
        strategies: {
          auto: {
            defaultChatModelID: 'cloud:gpt-5.5',
            defaultChatThinkingLevel: 'high',
          },
        },
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().setDefaultChatModel('cloud:gpt-5-mini');

    const config = useModelsStore.getState().config;
    assert.equal(config.strategies?.auto?.defaultChatModelID, 'cloud:gpt-5-mini');
    assert.equal(config.strategies?.auto?.defaultChatThinkingLevel, 'high');
    assert.equal(config.models.find((entry) => entry.key === 'cloud:gpt-5-mini')?.enabled, true);

    await useModelsStore.getState().toggleEnabled('cloud:gpt-5.5');
    assert.equal(config.models.find((entry) => entry.key === 'cloud:gpt-5.5')?.enabled, true);
    assert.equal(useModelsStore.getState().config.models.find((entry) => entry.key === 'cloud:gpt-5.5')?.enabled, false);
  } finally {
    restoreWindow();
  }
});

test('addProviderModel preserves the provider key and provider label', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        models: [
          ...emptyConfig.models,
          {
            key: 'cloud:claude-opus-4-6',
            id: 'claude-opus-4-6',
            label: 'Claude Opus 4.6',
            enabled: true,
            provider: 'cloud',
            providerLabel: 'Cloud',
            api: 'openai-completions',
            reasoning: true,
            reasoningControl: 'level',
            reasoningLevels: ['low', 'medium', 'high', 'max'],
          },
        ],
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().addProviderModel({
      provider: 'local-claude',
      providerLabel: 'Local Claude',
      id: 'claude-opus-4-6',
      label: 'Claude Direct',
      api: 'anthropic-messages',
      reasoning: true,
      reasoningControl: 'level',
      reasoningLevels: ['low', 'medium', 'high', 'max'],
      baseUrl: 'https://api.anthropic.com/v1',
      apiKey: 'anthropic-key',
    });

    const matchingModels = useModelsStore.getState().config.models.filter((entry) => entry.id === 'claude-opus-4-6');
    assert.equal(matchingModels.length, 2);
    const model = matchingModels.find((entry) => entry.key === 'local-claude:claude-opus-4-6');
    assert.ok(model);
    if (!model) {
      throw new Error('local-claude:claude-opus-4-6 model not found');
    }
    assert.equal(model.provider, 'local-claude');
    assert.equal(model.providerLabel, 'Local Claude');
    assert.equal(model.api, 'anthropic-messages');
    assert.equal(model.reasoning, true);
    assert.equal(model.reasoningControl, 'level');
    assert.deepEqual(model.reasoningLevels, ['low', 'medium', 'high', 'max']);
  } finally {
    restoreWindow();
  }
});

test('setModelPreference persists per-model thinking, context, and priority tier', async () => {
  let savedConfig: unknown;
  const restoreWindow = installElectronBridge(async (config) => {
    savedConfig = config;
    return config;
  });
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        models: [{
          key: 'cloud:gpt-5.4',
          id: 'gpt-5.4',
          enabled: true,
          provider: 'cloud',
          api: 'openai-responses',
          reasoning: true,
        }],
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().setModelPreference('cloud:gpt-5.4', {
      thinkingLevel: 'high',
      contextWindow: 300000,
      serviceTier: 'priority',
    });

    assert.deepEqual(useModelsStore.getState().config.modelPreferences?.['cloud:gpt-5.4'], {
      thinkingLevel: 'high',
      contextWindow: 300000,
      serviceTier: 'priority',
    });
    assert.deepEqual((savedConfig as ModelsConfig).modelPreferences?.['cloud:gpt-5.4'], {
      thinkingLevel: 'high',
      contextWindow: 300000,
      serviceTier: 'priority',
    });
  } finally {
    restoreWindow();
  }
});

test('setModelPreference rolls back when persistence fails', async () => {
  const restoreWindow = installElectronBridge(async () => {
    throw new Error('write failed');
  });
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        models: [{
          key: 'cloud:gpt-5.4',
          id: 'gpt-5.4',
          enabled: true,
          provider: 'cloud',
          api: 'openai-responses',
          reasoning: true,
        }],
      },
      loading: false,
      error: null,
    });

    await assert.rejects(
      () => useModelsStore.getState().setModelPreference('cloud:gpt-5.4', { contextWindow: 300000 }),
      /write failed/,
    );

    assert.equal(useModelsStore.getState().config.modelPreferences, undefined);
  } finally {
    restoreWindow();
  }
});

test('addProviderModel keeps provider-specific identity even when canonical model id matches Cloud', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        models: [
          ...emptyConfig.models,
          {
            key: 'cloud:gpt-5.4',
            id: 'gpt-5.4',
            label: 'GPT 5.4',
            enabled: true,
            provider: 'cloud',
            providerLabel: 'Cloud',
            api: 'openai-responses',
            reasoning: true,
            reasoningControl: 'level',
            reasoningLevels: ['minimal', 'low', 'medium', 'high', 'xhigh'],
          },
        ],
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().addProviderModel({
      provider: 'codemirror-local',
      providerLabel: 'CodeMirror 本地',
      id: 'gpt-5.4',
      label: 'My GPT Runtime',
      api: 'openai-responses',
      reasoning: true,
      reasoningControl: 'level',
      reasoningLevels: ['minimal', 'low', 'medium', 'high', 'xhigh'],
      baseUrl: 'https://api.aicodemirror.com/api/openai/v1',
      apiKey: 'cm-key',
    });

    const model = useModelsStore.getState().config.models.find((entry) => entry.key === 'codemirror-local:gpt-5.4');
    assert.ok(model);
    if (!model) {
      throw new Error('codemirror-local:gpt-5.4 model not found');
    }
    assert.equal(model.label, 'My GPT Runtime');
    assert.equal(model.api, 'openai-responses');
    assert.equal(model.provider, 'codemirror-local');
    assert.equal(model.providerLabel, 'CodeMirror 本地');
    assert.equal(model.reasoning, true);
    assert.equal(model.reasoningControl, 'level');
    assert.deepEqual(model.reasoningLevels, ['minimal', 'low', 'medium', 'high', 'xhigh']);
  } finally {
    restoreWindow();
  }
});

test('addProviderModel can reuse an existing provider without resubmitting endpoint credentials', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        providers: {
          ...emptyConfig.providers,
          'local-openai': {
            label: 'Local OpenAI',
            api: 'openai-responses',
            baseUrl: 'https://api.openai.com/v1',
            apiKey: 'openai-key',
            models: [
              {
                id: 'gpt-5.4',
                label: 'GPT 5.4',
                enabled: true,
                reasoning: true,
                reasoningControl: 'level',
                reasoningLevels: ['minimal', 'low'],
              },
            ],
          },
        },
        models: [
          ...emptyConfig.models,
          {
            key: 'local-openai:gpt-5.4',
            id: 'gpt-5.4',
            label: 'GPT 5.4',
            enabled: true,
            provider: 'local-openai',
            providerLabel: 'Local OpenAI',
            api: 'openai-responses',
            reasoning: true,
            reasoningControl: 'level',
            reasoningLevels: ['minimal', 'low'],
            baseUrl: 'https://api.openai.com/v1',
            apiKey: 'openai-key',
          },
        ],
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().addProviderModel({
      provider: 'local-openai',
      id: 'gpt-5-mini',
      label: 'GPT 5 Mini',
      api: 'openai-responses',
      reasoning: true,
      reasoningLevels: ['minimal', 'low'],
    });

    const model = useModelsStore.getState().config.models.find((entry) => entry.key === 'local-openai:gpt-5-mini');
    assert.ok(model);
    if (!model) {
      throw new Error('local-openai:gpt-5-mini model not found');
    }
    assert.equal(model.baseUrl, 'https://api.openai.com/v1');
    assert.equal(model.apiKey, 'openai-key');
  } finally {
    restoreWindow();
  }
});

test('addProviderModel infers toggle reasoning control when reasoning is enabled without levels', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().addProviderModel({
      provider: 'local-openai',
      providerLabel: 'Local OpenAI',
      id: 'kimi-4.6',
      label: 'Kimi 4.6',
      api: 'openai-completions',
      reasoning: true,
      baseUrl: 'https://example.com/v1',
      apiKey: 'secret',
    });

    const model = useModelsStore.getState().config.models.find((entry) => entry.key === 'local-openai:kimi-4.6');
    assert.ok(model);
    assert.equal(model?.reasoningControl, 'toggle');
  } finally {
    restoreWindow();
  }
});

test('addProviderModel allows model-level endpoint override under one provider', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        providers: {
          ...emptyConfig.providers,
          codemirror: {
            label: 'codemirror',
            apiKey: 'cm-key',
            models: [
              {
                id: 'gpt-5.4',
                label: 'GPT 5.4',
                enabled: true,
                api: 'openai-responses',
                baseUrl: 'https://api.aicodemirror.com/api/codex/backend-api/codex',
                reasoning: true,
                reasoningControl: 'level',
                reasoningLevels: ['minimal', 'low'],
              },
            ],
          },
        },
        models: [
          ...emptyConfig.models,
          {
            key: 'codemirror:gpt-5.4',
            id: 'gpt-5.4',
            label: 'GPT 5.4',
            enabled: true,
            provider: 'codemirror',
            providerLabel: 'codemirror',
            api: 'openai-responses',
            reasoning: true,
            reasoningControl: 'level',
            reasoningLevels: ['minimal', 'low'],
            baseUrl: 'https://api.aicodemirror.com/api/codex/backend-api/codex',
            apiKey: 'cm-key',
          },
        ],
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().addProviderModel({
      provider: 'codemirror',
      id: 'claude-opus-4-6',
      label: 'Claude Opus 4.6',
      api: 'anthropic-messages',
      reasoning: true,
      reasoningLevels: ['low', 'medium', 'high', 'max'],
      baseUrl: 'https://api.aicodemirror.com/api/claudecode',
    });

    const model = useModelsStore.getState().config.models.find((entry) => entry.key === 'codemirror:claude-opus-4-6');
    assert.ok(model);
    if (!model) {
      throw new Error('codemirror:claude-opus-4-6 model not found');
    }
    assert.equal(model.baseUrl, 'https://api.aicodemirror.com/api/claudecode');
    assert.equal(model.apiKey, 'cm-key');
    assert.equal(useModelsStore.getState().config.providers.codemirror.baseUrl, undefined);
  } finally {
    restoreWindow();
  }
});

test('updateProvider rewrites provider metadata across all provider models', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        providers: {
          ...emptyConfig.providers,
          'local-openai': {
            label: 'Local OpenAI',
            api: 'openai-responses',
            baseUrl: 'https://api.openai.com/v1',
            apiKey: 'openai-key',
            models: [
              { id: 'gpt-5.4', enabled: true, reasoning: true },
              { id: 'gpt-5-mini', enabled: true, reasoning: false },
            ],
          },
        },
        models: [
          ...emptyConfig.models,
          {
            key: 'local-openai:gpt-5.4',
            id: 'gpt-5.4',
            label: 'GPT 5.4',
            enabled: true,
            provider: 'local-openai',
            providerLabel: 'Local OpenAI',
            api: 'openai-responses',
            reasoning: true,
            baseUrl: 'https://api.openai.com/v1',
            apiKey: 'openai-key',
          },
          {
            key: 'local-openai:gpt-5-mini',
            id: 'gpt-5-mini',
            label: 'GPT 5 Mini',
            enabled: true,
            provider: 'local-openai',
            providerLabel: 'Local OpenAI',
            api: 'openai-responses',
            reasoning: false,
            baseUrl: 'https://api.openai.com/v1',
            apiKey: 'openai-key',
          },
        ],
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().updateProvider('local-openai', {
      providerLabel: 'OpenAI Mirror',
      baseUrl: 'https://mirror.example.com/v1',
      apiKey: 'mirror-key',
    });

    const updatedModels = useModelsStore.getState().config.models.filter((entry) => entry.provider === 'local-openai');
    assert.equal(updatedModels.length, 2);
    assert.ok(updatedModels.every((entry) => entry.providerLabel === 'OpenAI Mirror'));
    assert.ok(updatedModels.every((entry) => entry.baseUrl === 'https://mirror.example.com/v1'));
    assert.ok(updatedModels.every((entry) => entry.apiKey === 'mirror-key'));
  } finally {
    restoreWindow();
  }
});

test('removeProvider removes every model under that provider and re-picks the default model', async () => {
  const restoreWindow = installElectronBridge();
  try {
    useModelsStore.setState({
      config: {
        ...emptyConfig,
        defaultModelKey: 'local-openai:gpt-5.4',
        providers: {
          ...emptyConfig.providers,
          'local-openai': {
            label: 'Local OpenAI',
            api: 'openai-responses',
            baseUrl: 'https://api.openai.com/v1',
            apiKey: 'openai-key',
            models: [
              { id: 'gpt-5.4', enabled: true, reasoning: true },
            ],
          },
        },
        models: [
          ...emptyConfig.models,
          {
            key: 'local-openai:gpt-5.4',
            id: 'gpt-5.4',
            label: 'GPT 5.4',
            enabled: true,
            provider: 'local-openai',
            providerLabel: 'Local OpenAI',
            api: 'openai-responses',
            reasoning: true,
            baseUrl: 'https://api.openai.com/v1',
            apiKey: 'openai-key',
          },
        ],
      },
      loading: false,
      error: null,
    });

    await useModelsStore.getState().removeProvider('local-openai');

    assert.equal(useModelsStore.getState().config.models.some((entry) => entry.provider === 'local-openai'), false);
    assert.equal(useModelsStore.getState().config.defaultModelKey, null);
  } finally {
    restoreWindow();
  }
});

test('managed organization providers cannot be edited or removed as custom providers', async () => {
  const restoreWindow = installElectronBridge();
  try {
    const orgConfig: ModelsConfig = {
      ...emptyConfig,
      defaultModelKey: 'acme:gpt-5.4',
      providers: {
        acme: {
          label: 'Acme',
          managed: true,
          models: [
            {
              id: 'gpt-5.4',
              label: 'GPT 5.4',
              enabled: true,
              api: 'openai-responses',
              reasoning: true,
            },
          ],
        },
      },
      models: [
        {
          key: 'acme:gpt-5.4',
          id: 'gpt-5.4',
          label: 'GPT 5.4',
          enabled: true,
          provider: 'acme',
          providerLabel: 'Acme',
          api: 'openai-responses',
          reasoning: true,
        },
      ],
    };
    useModelsStore.setState({
      config: orgConfig,
      loading: false,
      error: null,
    });

    await useModelsStore.getState().updateProvider('acme', {
      providerLabel: 'Edited',
      baseUrl: 'https://example.com/v1',
      apiKey: 'secret',
    });
    await useModelsStore.getState().removeProviderModel('acme:gpt-5.4');
    await useModelsStore.getState().removeProvider('acme');

    const current = useModelsStore.getState().config;
    assert.equal(current.models.length, 1);
    assert.equal(current.models[0].providerLabel, 'Acme');
    assert.equal(current.models[0].baseUrl, undefined);
    assert.equal(current.models[0].apiKey, undefined);
    assert.ok(current.providers['acme']);
  } finally {
    restoreWindow();
  }
});
