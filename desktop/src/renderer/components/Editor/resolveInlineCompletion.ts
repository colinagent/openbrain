import { useModelsStore } from '../../store/modelsStore';
import { useUiStore } from '../../store/uiStore';
import {
  useAppStore,
  type EditorCompletionRequest,
  type EditorCompletionResult,
} from '../../store/appStore';
import type { ModelEntry } from '../../types/electron';
import { resolveChatModelSelection } from '../../utils/chatModelSelection';
import { isThreadChatPath } from '../../utils/chatAgentTarget';

function normalizeText(value: string | null | undefined): string {
  return (value || '').trim();
}

function resolveEnabledModelByKey(models: ModelEntry[], modelKey: string | null | undefined): ModelEntry | null {
  const normalizedModelKey = normalizeText(modelKey);
  if (!normalizedModelKey) {
    return null;
  }
  return models.find((model) => model.enabled && model.key === normalizedModelKey) || null;
}

async function resolveInlineCompletionTarget(): Promise<{ modelKey: string | null; thinkingLevel: string | null }> {
  const completion = useUiStore.getState().completion;
  const modelsConfig = useModelsStore.getState().config;

  if (completion.mode === 'custom') {
    const customModelKey = normalizeText(completion.customModelKey);
    return {
      modelKey: resolveEnabledModelByKey(modelsConfig.models, customModelKey)?.key || null,
      thinkingLevel: null,
    };
  }

  const enabledModels = modelsConfig.models.filter((model) => model.enabled);
  const effectiveModelKey = resolveChatModelSelection(
    enabledModels,
    modelsConfig.defaultModelKey,
  ).effectiveModelKey;
  return { modelKey: effectiveModelKey, thinkingLevel: null };
}

export function isInlineCompletionEnabledForPath(
  path: string | null | undefined,
): boolean {
  const completion = useUiStore.getState().completion;
  if (!completion.enabled || completion.mode === 'off') {
    return false;
  }
  return !isThreadChatPath(path || '');
}

export async function requestInlineCompletion(
  documentPath: string | null | undefined,
  request: Omit<EditorCompletionRequest, 'modelKey' | 'documentPath'>,
): Promise<EditorCompletionResult | null> {
  const completion = useUiStore.getState().completion;
  if (
    !completion.enabled ||
    completion.mode === 'off' ||
    isThreadChatPath(documentPath || '')
  ) {
    return null;
  }

  const target = await resolveInlineCompletionTarget();
  if (!target.modelKey) {
    return null;
  }

  return useAppStore.getState().requestEditorCompletion({
    ...request,
    modelKey: target.modelKey,
    thinkingLevel: target.thinkingLevel || undefined,
    documentPath: documentPath || null,
  });
}

export function cancelInlineCompletion(requestID: string): void {
  void useAppStore.getState().cancelEditorCompletion(requestID);
}
