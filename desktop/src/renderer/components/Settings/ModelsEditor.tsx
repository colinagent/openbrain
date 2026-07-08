import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { LogInIcon } from '../Icons';
import { SelectMenu } from '../SelectMenu';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_EDITOR,
  UI_PILL_BTN_PRIMARY,
  UI_PILL_BTN_SECONDARY,
} from '../staticGlassCapsule';
import { useAuthStore } from '../../store/authStore';
import { useModelsStore } from '../../store/modelsStore';
import { useUiStore } from '../../store/uiStore';
import type { ModelEntry } from '../../types/electron';
import { resolveModelReasoningControl, type ModelReasoningControl } from '../../../main/shared/modelReasoning';
import {
  CUSTOM_MODEL_API_OPTIONS,
  getCustomModelApiMeta,
  resolveCustomModelBaseUrlForApiSwitch,
} from '../../utils/customModelApi';
import { buildModelSelectOption, getModelEntryDisplay } from '../../utils/modelDisplay';
import {
  OPENBRAIN_PROVIDER_KEY,
  buildModelKey,
  deriveProviderKeyFromLabel,
  formatModelProviderLabel,
} from '../../../shared/modelKeys';
import { normalizeEditorCompletionSettings, type EditorCompletionMode } from '../../../main/shared/editorCompletion';

type NoticeTone = 'success' | 'warning' | 'error';

type NoticeState = {
  tone: NoticeTone;
  text: string;
} | null;

type ProviderGroup = {
  key: string;
  label: string;
  api?: ModelEntry['api'];
  baseUrl?: string;
  apiKey?: string;
  isBuiltIn: boolean;
  models: ModelEntry[];
};

const NEW_PROVIDER_TARGET = '__new__';
const DEFAULT_OPENAI_MODEL_API: ModelEntry['api'] = 'openai-responses';
const PROVIDER_API_MENU_CLASS_NAME = 'w-[360px] max-w-[calc(100vw-32px)]';
const PRIMARY_PILL_BUTTON_CLASS = `${UI_PILL_BTN_PRIMARY} ${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR}`;
const SECONDARY_PILL_BUTTON_CLASS = UI_PILL_BTN_SECONDARY;
const REASONING_CONTROL_OPTIONS: Array<{
  value: ModelReasoningControl;
  label: string;
  description: string;
}> = [
  {
    value: 'level',
    label: 'Level-based',
    description: 'Expose explicit reasoning levels like minimal/low/high.',
  },
  {
    value: 'toggle',
    label: 'On/off only',
    description: 'Expose only a boolean thinking switch.',
  },
];

export const ModelsEditor: React.FC = () => {
  const { t } = useTranslation(['settings', 'common']);
  const loggedIn = useAuthStore((state) => state.loggedIn);
  const startLogin = useAuthStore((state) => state.startLogin);
  const deviceCodeError = useAuthStore((state) => state.deviceCodeError);
  const load = useModelsStore((state) => state.load);
  const refreshFromOpenBrain = useModelsStore((state) => state.refreshFromOpenBrain);
  const toggleEnabled = useModelsStore((state) => state.toggleEnabled);
  const setDefaultChatModel = useModelsStore((state) => state.setDefaultChatModel);
  const addProviderModel = useModelsStore((state) => state.addProviderModel);
  const updateProvider = useModelsStore((state) => state.updateProvider);
  const removeProvider = useModelsStore((state) => state.removeProvider);
  const removeProviderModel = useModelsStore((state) => state.removeProviderModel);
  const updateProviderModel = useModelsStore((state) => state.updateProviderModel);
  const config = useModelsStore((state) => state.config);
  const loading = useModelsStore((state) => state.loading);
  const error = useModelsStore((state) => state.error);
  const completion = useUiStore((state) => state.completion);
  const setCompletion = useUiStore((state) => state.setCompletion);

  const [providerTarget, setProviderTarget] = useState<string>(NEW_PROVIDER_TARGET);
  const [newProviderLabel, setNewProviderLabel] = useState('');
  const [newModelID, setNewModelID] = useState('');
  const [newApi, setNewApi] = useState<ModelEntry['api']>(DEFAULT_OPENAI_MODEL_API);
  const [newBaseUrl, setNewBaseUrl] = useState('');
  const [newApiKey, setNewApiKey] = useState('');
  const [editingProviderKey, setEditingProviderKey] = useState<string | null>(null);
  const [editingProviderLabel, setEditingProviderLabel] = useState('');
  const [editingProviderBaseUrl, setEditingProviderBaseUrl] = useState('');
  const [editingProviderApiKey, setEditingProviderApiKey] = useState('');
  const [editingModelKey, setEditingModelKey] = useState<string | null>(null);
  const [editingModelLabel, setEditingModelLabel] = useState('');
  const [editingModelApi, setEditingModelApi] = useState<ModelEntry['api']>(DEFAULT_OPENAI_MODEL_API);
  const [editingModelReasoningControl, setEditingModelReasoningControl] = useState<ModelReasoningControl>('level');
  const [editingModelBaseUrl, setEditingModelBaseUrl] = useState('');
  const [editingModelApiKey, setEditingModelApiKey] = useState('');
  const [notice, setNotice] = useState<NoticeState>(null);
  const previousLoggedInRef = useRef(loggedIn);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (deviceCodeError) {
      setNotice({ tone: 'error', text: deviceCodeError });
    }
  }, [deviceCodeError]);

  const sortedModels = useMemo(() => {
    return [...config.models].sort((a, b) => {
      const providerCmp = (a.providerLabel || a.provider).localeCompare(b.providerLabel || b.provider);
      if (providerCmp !== 0) {
        return providerCmp;
      }
      const aLabel = (a.label || a.id).toLowerCase();
      const bLabel = (b.label || b.id).toLowerCase();
      return aLabel.localeCompare(bLabel);
    });
  }, [config.models]);

  const providerGroups = useMemo(() => {
    return Object.entries(config.providers)
      .map(([providerKey, provider]) => {
        const models = sortedModels.filter((model) => model.provider === providerKey);
        return {
          key: providerKey,
          label: formatModelProviderLabel(providerKey, provider.label) || providerKey,
          api: provider.api,
          baseUrl: provider.baseUrl,
          apiKey: provider.apiKey,
          isBuiltIn: providerKey === OPENBRAIN_PROVIDER_KEY || provider.managed === true,
          models,
        } satisfies ProviderGroup;
      })
      .sort((a, b) => {
        if (a.isBuiltIn && !b.isBuiltIn) {
          return -1;
        }
        if (!a.isBuiltIn && b.isBuiltIn) {
          return 1;
        }
        return a.label.localeCompare(b.label);
      });
  }, [config.providers, sortedModels]);

  const customProviderGroups = useMemo(
    () => providerGroups.filter((provider) => !provider.isBuiltIn),
    [providerGroups],
  );

  const selectedExistingProvider = useMemo(
    () => customProviderGroups.find((provider) => provider.key === providerTarget) || null,
    [customProviderGroups, providerTarget],
  );

  useEffect(() => {
    if (providerTarget === NEW_PROVIDER_TARGET) {
      return;
    }
    if (selectedExistingProvider) {
      return;
    }
    setProviderTarget(NEW_PROVIDER_TARGET);
  }, [providerTarget, selectedExistingProvider]);

  const openbrainModels = useMemo(
    () => sortedModels.filter((model) => model.provider === OPENBRAIN_PROVIDER_KEY),
    [sortedModels],
  );
  const openbrainModelOptions = useMemo(
    () => openbrainModels.map((model) => buildModelSelectOption(model, model.id)),
    [openbrainModels],
  );
  const selectedOpenBrainModel = useMemo(
    () => openbrainModels.find((model) => model.id === newModelID) || null,
    [openbrainModels, newModelID],
  );
  const currentApiMeta = useMemo(() => getCustomModelApiMeta(newApi), [newApi]);
  const canRefreshFromOpenBrain = Boolean(window.electronAPI?.models?.refreshFromOpenBrain);
  const openbrainModelSelectionDisabled = !loggedIn || loading || openbrainModelOptions.length === 0;
  const openbrainModelPlaceholder = !loggedIn
    ? 'Log in to load OpenBrain models'
    : openbrainModelOptions.length > 0
      ? 'Select model ID'
      : 'No OpenBrain models available';
  const addDisabled = loading || !loggedIn || openbrainModelOptions.length === 0;
  const completionModelOptions = useMemo(
    () => sortedModels.filter((model) => model.enabled).map((model) => buildModelSelectOption(model, model.key)),
    [sortedModels],
  );

  const providerTargetOptions = useMemo(
    () => [
      {
        value: NEW_PROVIDER_TARGET,
        label: 'New provider',
        description: 'Create endpoint + add first model',
        title: 'New provider',
      },
      ...customProviderGroups.map((provider) => ({
        value: provider.key,
        label: provider.label,
        description: provider.baseUrl || 'Saved provider',
        title: provider.label,
      })),
    ],
    [customProviderGroups],
  );

  useEffect(() => {
    const wasLoggedIn = previousLoggedInRef.current;
    previousLoggedInRef.current = loggedIn;
    if (wasLoggedIn || !loggedIn || !canRefreshFromOpenBrain) {
      return;
    }
    setNotice(null);
    void refreshFromOpenBrain();
  }, [canRefreshFromOpenBrain, loggedIn, refreshFromOpenBrain]);

  const handleRefresh = async () => {
    setNotice(null);
    if (!loggedIn) {
      setNotice({ tone: 'warning', text: 'Log in to sync models from OpenBrain.' });
      return;
    }
    await refreshFromOpenBrain();
    const latestError = useModelsStore.getState().error;
    if (!latestError) {
      setNotice({ tone: 'success', text: 'OpenBrain models refreshed.' });
    }
  };

  const handleLogin = async () => {
    setNotice(null);
    try {
      await startLogin();
    } catch (error) {
      const message = (error as Error).message?.trim();
      setNotice({
        tone: 'error',
        text: message ? `Failed to start sign in: ${message}` : 'Failed to start sign in.',
      });
    }
  };

  const handleApiChange = (nextApi: ModelEntry['api']) => {
    setNewBaseUrl((current) => resolveCustomModelBaseUrlForApiSwitch(current, newApi, nextApi));
    setNewApi(nextApi);
  };

  const handleModelIDChange = (nextModelID: string) => {
    setNewModelID(nextModelID);
    const nextModel = openbrainModels.find((model) => model.id === nextModelID);
    if (nextModel) {
      setNewApi(nextModel.api);
    }
    setNotice(null);
  };

  const handleProviderTargetChange = (nextValue: string) => {
    setProviderTarget(nextValue);
    setNotice(null);
    const nextProvider = customProviderGroups.find((provider) => provider.key === nextValue);
    if (!nextProvider) {
      setNewBaseUrl('');
      setNewApiKey('');
      return;
    }
    // Inherit from provider level first, then fall back to first model
    const firstModel = nextProvider.models[0];
    const selectedModel = openbrainModels.find((model) => model.id === newModelID);
    setNewApi(selectedModel?.api || nextProvider.api || firstModel?.api || DEFAULT_OPENAI_MODEL_API);
    setNewBaseUrl(nextProvider.baseUrl || firstModel?.baseUrl || '');
    setNewApiKey(nextProvider.apiKey || firstModel?.apiKey || '');
  };

  const persistCompletion = async (patch: { enabled?: boolean; mode?: EditorCompletionMode; customModelKey?: string | null }) => {
    const next = normalizeEditorCompletionSettings({ ...completion, ...patch });
    setCompletion(next);
    await window.electronAPI?.settings?.set?.({ ui: { completion: next } });
  };

  const handleStartEditModel = (model: ModelEntry) => {
    setEditingModelKey(model.key);
    setEditingModelLabel(model.label || '');
    setEditingModelApi(model.api);
    setEditingModelReasoningControl(resolveModelReasoningControl(model) || 'level');
    setEditingModelBaseUrl(model.baseUrl || '');
    setEditingModelApiKey(model.apiKey || '');
    setEditingProviderKey(null);
    setNotice(null);
  };

  const handleCancelEditModel = () => {
    setEditingModelKey(null);
    setEditingModelLabel('');
    setEditingModelApi(DEFAULT_OPENAI_MODEL_API);
    setEditingModelReasoningControl('level');
    setEditingModelBaseUrl('');
    setEditingModelApiKey('');
  };

  const handleSaveModel = async () => {
    if (!editingModelKey) {
      return;
    }
    const baseUrl = editingModelBaseUrl.trim();
    const apiKey = editingModelApiKey.trim();
    if (!baseUrl) {
      setNotice({ tone: 'error', text: 'Model base URL is required.' });
      return;
    }
    if (!apiKey) {
      setNotice({ tone: 'error', text: 'Model API key is required.' });
      return;
    }
    await updateProviderModel(editingModelKey, {
      label: editingModelLabel.trim() || undefined,
      api: editingModelApi,
      reasoningControl: editingModelReasoningControl,
      baseUrl,
      apiKey,
    });
    const modelLabel = editingModelLabel.trim() || editingModelKey;
    handleCancelEditModel();
    setNotice({ tone: 'success', text: `Model ${modelLabel} updated.` });
  };

  const handleStartEditProvider = (provider: ProviderGroup) => {
    setEditingProviderKey(provider.key);
    setEditingProviderLabel(provider.label === provider.key ? '' : provider.label);
    setEditingProviderBaseUrl(provider.baseUrl || '');
    setEditingProviderApiKey(provider.apiKey || '');
    setEditingModelKey(null);
    setNotice(null);
  };

  const handleCancelEditProvider = () => {
    setEditingProviderKey(null);
    setEditingProviderLabel('');
    setEditingProviderBaseUrl('');
    setEditingProviderApiKey('');
  };

  const handleSaveProvider = async (provider: ProviderGroup) => {
    const baseUrl = editingProviderBaseUrl.trim();
    const apiKey = editingProviderApiKey.trim();
    if (!baseUrl) {
      setNotice({ tone: 'error', text: 'Provider base URL is required.' });
      return;
    }
    if (!apiKey) {
      setNotice({ tone: 'error', text: 'Provider API key is required.' });
      return;
    }
    await updateProvider(provider.key, {
      providerLabel: editingProviderLabel.trim(),
      baseUrl,
      apiKey,
    });
    handleCancelEditProvider();
    setNotice({ tone: 'success', text: `Provider ${provider.label} updated.` });
  };

  const handleRemoveProvider = async (provider: ProviderGroup) => {
    if (provider.isBuiltIn) {
      return;
    }
    try {
      await removeProvider(provider.key);
    } catch (error) {
      setNotice({ tone: 'error', text: error instanceof Error ? error.message : 'Failed to remove provider.' });
      return;
    }
    if (providerTarget === provider.key) {
      setProviderTarget(NEW_PROVIDER_TARGET);
    }
    if (editingProviderKey === provider.key) {
      handleCancelEditProvider();
    }
    setNotice({ tone: 'success', text: `Provider ${provider.label} removed.` });
  };

  const handleToggleModelEnabled = async (model: ModelEntry) => {
    try {
      await toggleEnabled(model.key);
    } catch (error) {
      setNotice({ tone: 'error', text: error instanceof Error ? error.message : 'Failed to update model.' });
    }
  };

  const handleRemoveModel = async (model: ModelEntry) => {
    try {
      await removeProviderModel(model.key);
    } catch (error) {
      setNotice({ tone: 'error', text: error instanceof Error ? error.message : 'Failed to remove model.' });
    }
  };

  const handleSetDefaultChatModel = async (model: ModelEntry) => {
    try {
      await setDefaultChatModel(model.key);
      setNotice({ tone: 'success', text: `${model.label || model.id || model.key} set as Default Chat Model.` });
    } catch (error) {
      setNotice({ tone: 'error', text: error instanceof Error ? error.message : 'Failed to set Default Chat Model.' });
    }
  };

  const handleAdd = async () => {
    const targetProvider = selectedExistingProvider;
    const providerLabel = targetProvider ? targetProvider.label : newProviderLabel.trim();
    const providerKey = targetProvider ? targetProvider.key : deriveProviderKeyFromLabel(providerLabel);
    const id = newModelID.trim();
    const baseUrl = newBaseUrl.trim();
    const apiKey = newApiKey.trim();
    if (!loggedIn) {
      setNotice({ tone: 'warning', text: 'Log in first to load OpenBrain models and choose a canonical model ID.' });
      return;
    }
    if (!providerLabel) {
      setNotice({ tone: 'error', text: 'Provider name is required.' });
      return;
    }
    if (providerKey === OPENBRAIN_PROVIDER_KEY || config.providers[providerKey]?.managed === true) {
      setNotice({ tone: 'error', text: 'Provider name is reserved for managed OpenBrain organization providers.' });
      return;
    }
    if (!targetProvider && config.providers[providerKey]) {
      setNotice({ tone: 'error', text: 'Provider already exists. Choose that provider instead of creating a new one.' });
      return;
    }
    if (!id) {
      setNotice({ tone: 'error', text: 'Model ID is required.' });
      return;
    }
    if (!selectedOpenBrainModel) {
      setNotice({ tone: 'error', text: 'Select a model ID from the OpenBrain catalog.' });
      return;
    }
    if (!baseUrl) {
      setNotice({ tone: 'error', text: 'Base URL is required.' });
      return;
    }
    if (!apiKey) {
      setNotice({ tone: 'error', text: 'API key is required.' });
      return;
    }
    const modelKey = buildModelKey(providerKey, id);
    if (config.models.some((model) => model.key === modelKey)) {
      setNotice({ tone: 'error', text: 'Provider model already exists.' });
      return;
    }
    await addProviderModel({
      provider: providerKey,
      providerLabel,
      id,
      label: (selectedOpenBrainModel.label || '').trim() || selectedOpenBrainModel.id,
      api: newApi,
      reasoning: selectedOpenBrainModel.reasoning,
      reasoningLevels: selectedOpenBrainModel.reasoningLevels,
      baseUrl,
      apiKey,
    });
    if (!targetProvider) {
      setNewProviderLabel('');
      setNewBaseUrl('');
      setNewApiKey('');
    }
    setNewModelID('');
    setNewApi(targetProvider?.api || DEFAULT_OPENAI_MODEL_API);
    setNotice({
      tone: 'success',
      text: targetProvider ? `Model added to provider ${targetProvider.label}.` : 'Provider created and model added.',
    });
  };

  const renderModelRow = (model: ModelEntry) => {
    const isDefault = config.defaultModelKey === model.key;
    const isDefaultChat = (config.strategies?.auto?.defaultChatModelID || '').trim() === model.key;
    const display = getModelEntryDisplay(model);
    const isCustom = model.provider !== OPENBRAIN_PROVIDER_KEY && config.providers[model.provider]?.managed !== true;
    const isEditing = editingModelKey === model.key;
    const metaParts = [display.secondaryText, model.api, model.reasoning ? 'thinking' : null].filter(Boolean);
    const disableToggle = model.enabled && isDefaultChat;
    const canRemoveModel = isCustom && !isDefaultChat;

    return (
      <div key={model.key}>
        <div className="group flex items-start gap-3 px-4 py-2">
          <input
            type="checkbox"
            checked={model.enabled}
            onChange={() => void handleToggleModelEnabled(model)}
            className="mt-0.5 accent-accent"
            disabled={disableToggle}
            title={disableToggle ? 'Change Default Chat Model before disabling this model' : 'Enable model'}
          />
          <div className="min-w-0 flex-1">
            <span className="text-sm" title={display.titleText}>{display.primaryText}</span>
            {metaParts.length > 0 && (
              <div className="mt-0.5 truncate text-[11px] text-tertiary-text">{metaParts.join(' · ')}</div>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-1 pt-px">
            {isDefault ? (
              <span className="text-[11px] font-medium text-secondary-text">inline default</span>
            ) : null}
            {isDefaultChat ? (
              <span className="text-[11px] font-medium text-accent">default chat</span>
            ) : model.enabled ? (
              <button
                className={`${SECONDARY_PILL_BUTTON_CLASS} px-1.5 py-0.5 text-[11px] opacity-0 group-hover:opacity-100`}
                onClick={() => void handleSetDefaultChatModel(model)}
                title="Set as Default Chat Model"
              >
                Set chat default
              </button>
            ) : null}
            {isCustom && (
              <button
                className={`${SECONDARY_PILL_BUTTON_CLASS} ml-1 px-1.5 py-0.5 text-[11px] opacity-0 group-hover:opacity-100`}
                onClick={() => handleStartEditModel(model)}
                title="Edit model"
              >
                Edit
              </button>
            )}
            {canRemoveModel && (
              <button
                className={`${SECONDARY_PILL_BUTTON_CLASS} px-1.5 py-0.5 text-[11px] opacity-0 group-hover:opacity-100`}
                onClick={() => void handleRemoveModel(model)}
                title="Remove provider model"
              >
                Remove
              </button>
            )}
          </div>
        </div>

        {/* Inline model edit form */}
        {isCustom && isEditing && (
          <div className="border-t border-border/20 px-4 py-3 space-y-3">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">Display name</div>
                <input
                  className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                  value={editingModelLabel}
                  onChange={(e) => setEditingModelLabel(e.target.value)}
                  placeholder={model.id}
                />
              </div>
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">API protocol</div>
                <SelectMenu
                  triggerClassName="px-2 py-1 text-sm"
                  menuClassName="w-full"
                  ariaLabel="Model API protocol"
                  options={CUSTOM_MODEL_API_OPTIONS.map((option) => ({
                    value: option.value,
                    label: option.label,
                    description: option.helperText,
                    title: `${option.label} — ${option.helperText}`,
                  }))}
                  value={editingModelApi}
                  onChange={(value) => setEditingModelApi(value as ModelEntry['api'])}
                />
              </div>
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">Base URL</div>
                <input
                  className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                  value={editingModelBaseUrl}
                  onChange={(e) => setEditingModelBaseUrl(e.target.value)}
                  placeholder="Base URL"
                />
              </div>
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">API key</div>
                <input
                  className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                  type="password"
                  value={editingModelApiKey}
                  onChange={(e) => setEditingModelApiKey(e.target.value)}
                  placeholder="API key"
                />
              </div>
            </div>
            <div>
              <div className="mb-1 text-[11px] text-tertiary-text">Reasoning control</div>
              <SelectMenu
                triggerClassName="px-2 py-1 text-sm"
                menuClassName="w-full"
                ariaLabel="Reasoning control"
                options={REASONING_CONTROL_OPTIONS.map((option) => ({
                  value: option.value,
                  label: option.label,
                  description: option.description,
                  title: `${option.label} — ${option.description}`,
                }))}
                value={editingModelReasoningControl}
                onChange={(value) => setEditingModelReasoningControl(value as ModelReasoningControl)}
              />
            </div>
            <div className="flex items-center gap-2">
              <button
                className={`${PRIMARY_PILL_BUTTON_CLASS} px-2.5 py-1 text-xs`}
                onClick={() => void handleSaveModel()}
              >
                Save
              </button>
              <button
                className={`${SECONDARY_PILL_BUTTON_CLASS} px-2 py-1 text-xs`}
                onClick={handleCancelEditModel}
              >
                Cancel
              </button>
            </div>
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="flex h-full flex-col overflow-auto bg-editor-bg text-editor-fg">
      {/* Page header */}
      <div className="px-6 pt-5 pb-2">
        <div className="flex items-center gap-2">
          <div className="text-lg font-semibold text-prime-text">{t('settings:models.title')}</div>
          {canRefreshFromOpenBrain ? (
            <button
              className={`${PRIMARY_PILL_BUTTON_CLASS} shrink-0 px-3 py-1 text-xs`}
              onClick={handleRefresh}
              disabled={loading || !loggedIn}
            >
              {t('settings:models.refresh')}
            </button>
          ) : null}
        </div>
        <div className="mt-1 text-xs text-secondary-text">
          {t('settings:models.subtitle')}
        </div>
      </div>

      <div className="max-w-[980px] space-y-6 px-6 pb-6 pt-3">
        {/* Login banner */}
        {!loggedIn ? (
          <div className="rounded-lg border border-amber-200 bg-amber-50/80 px-4 py-3">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="text-sm font-medium text-amber-800">{t('settings:models.signInRequired')}</div>
                <div className="mt-0.5 text-xs text-amber-700">
                  {t('settings:models.signInHint')}
                </div>
              </div>
              <button
                className={`${PRIMARY_PILL_BUTTON_CLASS} shrink-0 px-3 py-1.5 text-xs`}
                onClick={handleLogin}
              >
                <LogInIcon className="h-3.5 w-3.5" />
                {t('settings:models.logIn')}
              </button>
            </div>
          </div>
        ) : null}

        {/* Notices */}
        {error && (
          <div className="rounded-lg px-3 py-2 text-xs text-red-500 bg-red-50 dark:bg-red-900/20">{error}</div>
        )}
        {notice && (
          <div
            className={`rounded-lg px-3 py-2 text-xs ${
              notice.tone === 'success'
                ? 'bg-green-50 text-health-text dark:bg-green-900/20'
                : notice.tone === 'warning'
                  ? 'bg-amber-50 text-amber-700'
                  : 'bg-red-50 text-red-700 dark:bg-red-900/20'
            }`}
          >
            {notice.text}
          </div>
        )}

        {/* ─── Providers ─── */}
        <div>
          <div className="mb-3 text-base font-semibold text-prime-text">
            Available providers
          </div>

          {providerGroups.length === 0 ? (
            <div className="rounded-lg border border-border/40 px-4 py-5 text-sm text-secondary-text">
              No providers configured yet.
            </div>
          ) : (
            <div className="space-y-3">
              {providerGroups.map((provider) => {
                const providerHasDefaultChat = provider.models.some((model) => (
                  (config.strategies?.auto?.defaultChatModelID || '').trim() === model.key
                ));
                const metaSegments = [
                  !provider.isBuiltIn && provider.key !== provider.label ? provider.key : null,
                  provider.isBuiltIn
                    ? 'built-in'
                    : `${provider.models.length} model${provider.models.length !== 1 ? 's' : ''}`,
                  provider.api || null,
                ].filter(Boolean);

                return (
                  <div key={provider.key} className="overflow-hidden rounded-lg border border-border/40">
                    {/* Provider header */}
                    <div className="flex items-center justify-between gap-3 px-4 py-3">
                      <div className="min-w-0">
                        <div className="flex items-baseline gap-2">
                          <span className="text-sm font-medium text-prime-text">{provider.label}</span>
                          <span className="text-[11px] text-tertiary-text">{metaSegments.join(' · ')}</span>
                        </div>
                        <div className="mt-0.5 truncate text-xs text-secondary-text">
                          {provider.isBuiltIn
                            ? 'Managed by your OpenBrain session. Use Refresh to sync the built-in catalog.'
                            : provider.baseUrl || 'Model-specific endpoints'}
                        </div>
                      </div>
                      {!provider.isBuiltIn && (
                        <div className="flex shrink-0 items-center gap-1">
                          <button
                            className={`${SECONDARY_PILL_BUTTON_CLASS} px-2 py-1 text-xs`}
                            onClick={() => handleStartEditProvider(provider)}
                          >
                            Edit provider
                          </button>
                          <button
                            className={`${SECONDARY_PILL_BUTTON_CLASS} px-2 py-1 text-xs`}
                            onClick={() => void handleRemoveProvider(provider)}
                            disabled={providerHasDefaultChat}
                            title={providerHasDefaultChat ? 'Change Default Chat Model before removing this provider' : 'Remove provider'}
                          >
                            Remove provider
                          </button>
                        </div>
                      )}
                    </div>

                    {/* Inline edit form */}
                    {editingProviderKey === provider.key && (
                      <div className="border-t border-border/30 px-4 py-3 space-y-3">
                        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                          <div>
                            <div className="mb-1 text-[11px] text-tertiary-text">Provider name</div>
                            <input
                              className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                              value={editingProviderLabel}
                              onChange={(e) => setEditingProviderLabel(e.target.value)}
                              placeholder="Provider name"
                            />
                          </div>
                          <div className="sm:col-span-2">
                            <div className="mb-1 text-[11px] text-tertiary-text">Base URL</div>
                            <input
                              className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                              value={editingProviderBaseUrl}
                              onChange={(e) => setEditingProviderBaseUrl(e.target.value)}
                              placeholder="Base URL"
                            />
                          </div>
                        </div>
                        <div>
                          <div className="mb-1 text-[11px] text-tertiary-text">API key</div>
                          <input
                            className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                            type="password"
                            value={editingProviderApiKey}
                            onChange={(e) => setEditingProviderApiKey(e.target.value)}
                            placeholder="API key"
                          />
                        </div>
                        <div className="flex items-center gap-2">
                          <button
                            className={`${PRIMARY_PILL_BUTTON_CLASS} px-2.5 py-1 text-xs`}
                            onClick={() => void handleSaveProvider(provider)}
                          >
                            Save
                          </button>
                          <button
                            className={`${SECONDARY_PILL_BUTTON_CLASS} px-2 py-1 text-xs`}
                            onClick={handleCancelEditProvider}
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    )}

                    {/* Model list */}
                    <div className="border-t border-border/20">
                      {provider.models.length === 0 ? (
                        <div className="px-4 py-3 text-xs text-tertiary-text">
                          {provider.isBuiltIn
                            ? 'No OpenBrain models loaded yet.'
                            : 'No models configured for this provider yet.'}
                        </div>
                      ) : (
                        <div className="divide-y divide-border/15">
                          {provider.models.map(renderModelRow)}
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* ─── Add model ─── */}
        <div>
          <div className="mb-3 text-base font-semibold text-prime-text">{t('settings:models.addModel')}</div>
          <div className="rounded-lg border border-border/40 p-4 space-y-4">
            {/* Row 1: Provider */}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">Provider</div>
                <SelectMenu
                  triggerClassName="px-2 py-1 text-sm"
                  menuClassName="w-full"
                  ariaLabel="Target provider"
                  options={providerTargetOptions}
                  value={providerTarget}
                  onChange={handleProviderTargetChange}
                  disabled={loading}
                />
              </div>
              <div>
                {!selectedExistingProvider ? (
                  <>
                    <div className="mb-1 text-[11px] text-tertiary-text">Provider name</div>
                    <input
                      className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                      value={newProviderLabel}
                      onChange={(e) => setNewProviderLabel(e.target.value)}
                      placeholder="Provider name"
                    />
                  </>
                ) : (
                  <>
                    <div className="mb-1 text-[11px] text-tertiary-text">Selected provider</div>
                    <div className="rounded border border-border/30 px-3 py-1.5">
                      <div className="truncate text-sm text-prime-text" title={selectedExistingProvider.label}>
                        {selectedExistingProvider.label}
                      </div>
                      <div
                        className="mt-0.5 truncate text-[11px] text-tertiary-text"
                        title={selectedExistingProvider.baseUrl || ''}
                      >
                        {selectedExistingProvider.baseUrl || 'Model-specific endpoints'}
                      </div>
                    </div>
                  </>
                )}
              </div>
            </div>

            {/* Row 2: Model + API */}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">Model</div>
                <SelectMenu
                  triggerClassName="px-2 py-1 text-sm"
                  menuClassName="w-full"
                  ariaLabel="OpenBrain model ID"
                  placeholder={openbrainModelPlaceholder}
                  options={openbrainModelOptions}
                  value={newModelID}
                  onChange={handleModelIDChange}
                  disabled={openbrainModelSelectionDisabled}
                />
              </div>
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">API protocol</div>
                <SelectMenu
                  triggerClassName="px-2 py-1 text-sm"
                  menuClassName={PROVIDER_API_MENU_CLASS_NAME}
                  ariaLabel="Provider API"
                  options={CUSTOM_MODEL_API_OPTIONS.map((option) => ({
                    value: option.value,
                    label: option.label,
                    description: option.helperText,
                    title: `${option.label} — ${option.helperText}`,
                  }))}
                  value={newApi}
                  onChange={handleApiChange}
                  disabled={loading}
                />
              </div>
            </div>

            {/* Row 3: Connection */}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">Base URL</div>
                <input
                  className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                  value={newBaseUrl}
                  onChange={(e) => setNewBaseUrl(e.target.value)}
                  placeholder={currentApiMeta.baseUrlPlaceholder}
                />
              </div>
              <div>
                <div className="mb-1 text-[11px] text-tertiary-text">API key</div>
                <input
                  className="w-full rounded border border-border bg-editor-bg px-2 py-1 text-sm"
                  type="password"
                  value={newApiKey}
                  onChange={(e) => setNewApiKey(e.target.value)}
                  placeholder="API key (required)"
                />
              </div>
            </div>

            <button
              className={`${PRIMARY_PILL_BUTTON_CLASS} px-3 py-1 text-xs`}
              onClick={handleAdd}
              disabled={addDisabled}
            >
              Add model
            </button>
          </div>
        </div>

        {/* ─── Inline completion ─── */}
        <div>
          <div className="mb-1 text-base font-semibold text-prime-text">
            Inline completion
          </div>
          <div className="mb-3 text-xs text-secondary-text">
            Choose how editor inline suggestions are generated, and optionally pin them to a dedicated model.
          </div>
          <div className="rounded-lg border border-border/40 p-4">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={completion.enabled}
                  onChange={(e) => { void persistCompletion({ enabled: e.target.checked }); }}
                  className="accent-accent"
                />
                <span>Enable</span>
              </label>
              <SelectMenu
                triggerClassName="px-2 py-1 text-sm"
                menuClassName="w-full"
                ariaLabel="Completion mode"
                options={[
                  { value: 'default', label: 'Default', description: 'Use the completion agent default model, then the global default model.' },
                  { value: 'custom', label: 'Custom model', description: 'Use a dedicated model for inline completion.' },
                  { value: 'off', label: 'Off', description: 'Disable inline completion.' },
                ]}
                value={completion.mode}
                onChange={(value) => { void persistCompletion({ mode: value as EditorCompletionMode }); }}
                disabled={!completion.enabled}
              />
              <SelectMenu
                triggerClassName="px-2 py-1 text-sm"
                menuClassName="w-full"
                ariaLabel="Completion custom model"
                placeholder="Select model"
                options={completionModelOptions}
                value={completion.customModelKey || ''}
                onChange={(value) => { void persistCompletion({ customModelKey: value || null }); }}
                disabled={!completion.enabled || completion.mode !== 'custom'}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
