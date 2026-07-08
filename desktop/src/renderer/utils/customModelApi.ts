import type { ModelEntry } from '../types/electron';

export type CustomModelAPI = ModelEntry['api'];

export type CustomModelAPIMeta = {
  value: CustomModelAPI;
  label: string;
  defaultBaseUrl: string;
  baseUrlPlaceholder: string;
  helperText: string;
};

export const CUSTOM_MODEL_API_OPTIONS: readonly CustomModelAPIMeta[] = [
  {
    value: 'openai-completions',
    label: 'OpenAI Completions',
    defaultBaseUrl: '',
    baseUrlPlaceholder: 'https://api.openai.com/v1 or your OpenAI-compatible endpoint',
    helperText: 'Direct OpenAI-compatible chat completions access. Use an endpoint that serves /chat/completions.',
  },
  {
    value: 'openai-responses',
    label: 'OpenAI Responses',
    defaultBaseUrl: '',
    baseUrlPlaceholder: 'https://api.openai.com/v1 or your Responses-compatible endpoint',
    helperText: 'Direct OpenAI-compatible Responses access. Use an endpoint that serves /responses.',
  },
  {
    value: 'anthropic-messages',
    label: 'Anthropic Messages',
    defaultBaseUrl: 'https://api.anthropic.com/v1',
    baseUrlPlaceholder: 'https://api.anthropic.com/v1',
    helperText: 'Direct Anthropic Messages access. Use a native Messages endpoint instead of an OpenAI-compatible proxy.',
  },
  {
    value: 'gemini-native',
    label: 'Gemini Native',
    defaultBaseUrl: 'https://generativelanguage.googleapis.com',
    baseUrlPlaceholder: 'https://generativelanguage.googleapis.com',
    helperText: 'Direct Gemini native access. Use a Google AI Studio-compatible base URL for generateContent streaming.',
  },
] as const;

export function getCustomModelApiMeta(api: CustomModelAPI): CustomModelAPIMeta {
  return (
    CUSTOM_MODEL_API_OPTIONS.find((option) => option.value === api) ||
    CUSTOM_MODEL_API_OPTIONS[0]
  );
}

export function resolveCustomModelBaseUrlForApiSwitch(
  currentBaseUrl: string,
  previousApi: CustomModelAPI,
  nextApi: CustomModelAPI
): string {
  const trimmed = currentBaseUrl.trim();
  const previousDefaultBaseUrl = getCustomModelApiMeta(previousApi).defaultBaseUrl;
  const nextDefaultBaseUrl = getCustomModelApiMeta(nextApi).defaultBaseUrl;
  if (!trimmed || trimmed === previousDefaultBaseUrl) {
    return nextDefaultBaseUrl;
  }
  return currentBaseUrl;
}
