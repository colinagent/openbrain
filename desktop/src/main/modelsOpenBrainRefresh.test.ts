import assert from 'node:assert/strict';
import test from 'node:test';

type OpenBrainCatalogResponse = {
  data?: Array<{
    id?: string;
    name?: string;
    api?: string;
    reasoning?: boolean;
    reasoning_control?: string;
    reasoning_levels?: string[];
    context_windows?: number[];
    default_context_window?: number;
    service_tiers?: string[];
    max_output_tokens?: number;
    availability?: {
      available?: boolean;
    };
  }>;
};

type OpenBrainPoliciesResponse = {
  modelSelection?: {
    defaultChatModelID?: string;
    defaultChatThinkingLevel?: string;
    defaultInlineCompletionModelID?: string;
    defaultInlineCompletionThinkingLevel?: string;
  };
};

function parseOpenBrainCatalogApi(
  value: string | undefined
): 'openai-completions' | 'openai-responses' | 'anthropic-messages' | 'gemini-native' | null {
  const normalized = (value || '').trim();
  switch (normalized) {
    case 'openai-completions':
    case 'openai-responses':
    case 'anthropic-messages':
    case 'gemini-native':
      return normalized;
    default:
      return null;
  }
}

function collectModels(payload: OpenBrainCatalogResponse) {
  const out: Record<string, {
    api: string;
    reasoning: boolean;
    reasoningControl?: string;
    reasoningLevels?: string[];
    contextWindows?: number[];
    defaultContextWindow?: number;
    serviceTiers?: string[];
    maxOutputTokens?: number;
  }> = {};
  for (const item of payload.data || []) {
    const id = (item?.id || '').trim();
    const api = parseOpenBrainCatalogApi((item?.api || '').trim());
    if (!id || !api || item?.availability?.available === false) {
      continue;
    }
    const reasoningLevels = Array.isArray(item.reasoning_levels)
      ? item.reasoning_levels.map((level) => (level || '').trim()).filter(Boolean)
      : undefined;
    const contextWindows = Array.isArray(item.context_windows)
      ? Array.from(new Set(item.context_windows.filter((value) => Number.isInteger(value) && value > 0))).sort((a, b) => a - b)
      : undefined;
    const serviceTiers = Array.isArray(item.service_tiers)
      ? Array.from(new Set(item.service_tiers
          .map((value) => (value || '').trim().toLowerCase())
          .filter((value) => value === 'priority' || value === 'flex')))
      : undefined;
    out[id] = {
      api,
      reasoning: item.reasoning === true || Boolean(reasoningLevels?.length),
      ...(typeof item.reasoning_control === 'string' && item.reasoning_control.trim()
        ? { reasoningControl: item.reasoning_control.trim() }
        : {}),
      ...(reasoningLevels && reasoningLevels.length > 0 ? { reasoningLevels } : {}),
      ...(contextWindows && contextWindows.length > 0 ? { contextWindows } : {}),
      ...(typeof item.default_context_window === 'number' && Number.isInteger(item.default_context_window) && item.default_context_window > 0
        ? { defaultContextWindow: item.default_context_window }
        : {}),
      ...(serviceTiers && serviceTiers.length > 0 ? { serviceTiers } : {}),
      ...(typeof item.max_output_tokens === 'number' && Number.isInteger(item.max_output_tokens) && item.max_output_tokens > 0
        ? { maxOutputTokens: item.max_output_tokens }
        : {}),
    };
  }
  return out;
}

function collectStrategies(payload: OpenBrainPoliciesResponse | null | undefined) {
  const selection = payload?.modelSelection;
  if (!selection) {
    return undefined;
  }
  const defaultChatModelID = (selection.defaultChatModelID || '').trim();
  const defaultInlineCompletionModelID = (selection.defaultInlineCompletionModelID || '').trim();
  if (!defaultChatModelID && !defaultInlineCompletionModelID) {
    return undefined;
  }
  return {
    auto: {
      ...(defaultChatModelID ? { defaultChatModelID } : {}),
      ...(selection.defaultChatThinkingLevel ? { defaultChatThinkingLevel: selection.defaultChatThinkingLevel.trim() } : {}),
      ...(defaultInlineCompletionModelID ? { defaultInlineCompletionModelID } : {}),
      ...(selection.defaultInlineCompletionThinkingLevel
        ? { defaultInlineCompletionThinkingLevel: selection.defaultInlineCompletionThinkingLevel.trim() }
        : {}),
    },
  };
}

test('OpenBrain catalog refresh uses explicit api and flat reasoning fields', () => {
  const models = collectModels({
    data: [
      {
        id: 'gpt-5.4',
        name: 'gpt-5.4',
        api: 'openai-responses',
        reasoning: true,
        reasoning_control: 'level',
        reasoning_levels: ['low', 'medium', 'high'],
        service_tiers: ['priority'],
      },
      {
        id: 'claude-opus-4-6',
        name: 'claude-opus-4-6',
        api: 'anthropic-messages',
        reasoning_control: 'toggle',
        reasoning_levels: ['max'],
        context_windows: [300000, 1000000],
        default_context_window: 300000,
        max_output_tokens: 128000,
      },
      {
        id: 'missing-api',
        name: 'missing-api',
      },
      {
        id: 'catalog-only',
        name: 'catalog-only',
        api: 'openai-responses',
        reasoning: true,
        availability: { available: false },
      },
    ],
  });

  assert.deepEqual(models, {
    'gpt-5.4': {
      api: 'openai-responses',
      reasoning: true,
      reasoningControl: 'level',
      reasoningLevels: ['low', 'medium', 'high'],
      serviceTiers: ['priority'],
    },
    'claude-opus-4-6': {
      api: 'anthropic-messages',
      reasoning: true,
      reasoningControl: 'toggle',
      reasoningLevels: ['max'],
      contextWindows: [300000, 1000000],
      defaultContextWindow: 300000,
      maxOutputTokens: 128000,
    },
  });
});

test('OpenBrain policies map to local auto strategies', () => {
  const strategies = collectStrategies({
    modelSelection: {
      defaultChatModelID: 'gpt-5.4',
      defaultChatThinkingLevel: 'high',
      defaultInlineCompletionModelID: 'claude-opus-4-6',
      defaultInlineCompletionThinkingLevel: 'medium',
    },
  });

  assert.deepEqual(strategies, {
    auto: {
      defaultChatModelID: 'gpt-5.4',
      defaultChatThinkingLevel: 'high',
      defaultInlineCompletionModelID: 'claude-opus-4-6',
      defaultInlineCompletionThinkingLevel: 'medium',
    },
  });
});
