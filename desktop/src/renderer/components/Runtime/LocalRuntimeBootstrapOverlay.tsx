import React from 'react';
import { useTranslation } from 'react-i18next';
import type { RuntimeBootstrapState } from '../../types/electron';
import { useBlockingModal } from '../../utils/useBlockingModal';

type LocalRuntimeBootstrapOverlayProps = {
  state: RuntimeBootstrapState | null | undefined;
  onRetry: () => void;
  onQuit: () => void;
};

function showActions(state: RuntimeBootstrapState | null | undefined): boolean {
  if (!state) {
    return false;
  }
  return state.canRetry === true || state.canQuit === true;
}

export const LocalRuntimeBootstrapOverlay: React.FC<LocalRuntimeBootstrapOverlayProps> = ({
  state,
  onRetry,
  onQuit,
}) => {
  const { t } = useTranslation(['shell', 'common']);
  const open = state?.visible === true;
  useBlockingModal(open);

  if (!open || !state) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-[120] bg-overlay-bg/95">
      <div className="flex h-full items-center justify-center px-6">
        <div className="w-full max-w-[560px] rounded-xl border border-border bg-editor-bg p-6 shadow-2xl">
          <div className="flex items-center gap-3">
            <div className="h-3 w-3 shrink-0 rounded-full bg-highlight" />
            <div className="text-lg font-semibold text-editor-fg">
              {state.phase === 'error' ? t('shell:runtime.setupIssue') : t('shell:runtime.preparing')}
            </div>
          </div>

          <div className="mt-4 text-sm leading-6 text-editor-fg">
            {state.message || t('shell:runtime.checking')}
          </div>

          {state.detail && (
            <div className="mt-2 text-xs leading-5 text-secondary-text">
              {state.detail}
            </div>
          )}

          {state.error && (
            <div className="mt-4 rounded-lg border border-border bg-sidebar-bg px-3 py-2 text-xs leading-5 text-accent">
              {state.error}
            </div>
          )}

          <div className="mt-5 grid grid-cols-2 gap-3 text-xs text-secondary-text">
            <div className="rounded-lg border border-border bg-sidebar-bg px-3 py-2">
              <div>{t('shell:runtime.current')}</div>
              <div className="mt-1 text-editor-fg">{state.installedVersion || t('shell:runtime.notSetup')}</div>
            </div>
            <div className="rounded-lg border border-border bg-sidebar-bg px-3 py-2">
              <div>{t('shell:runtime.target')}</div>
              <div className="mt-1 text-editor-fg">{state.latestVersion || t('shell:runtime.unknown')}</div>
            </div>
          </div>

          {state.phase !== 'error' && (
            <div className="mt-5 text-xs text-secondary-text">
              {t('shell:runtime.autoContinue')}
            </div>
          )}

          {showActions(state) && (
            <div className="mt-6 flex items-center justify-end gap-3">
              {state.canQuit && (
                <button className="dialog-text-btn" onClick={onQuit}>
                  {t('shell:runtime.quit')}
                </button>
              )}
              {state.canRetry && (
                <button className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor px-3 py-1.5 text-sm" onClick={onRetry}>
                  {t('shell:runtime.retry')}
                </button>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
