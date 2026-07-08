import React, { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { OpenBrainLogo } from '../Icons';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_EDITOR,
  UI_PILL_BTN_DIALOG,
  UI_PILL_BTN_PRIMARY,
  UI_PILL_BTN_SECONDARY,
} from '../staticGlassCapsule';
import { useBlockingModal } from '../../utils/useBlockingModal';
import { useAppStore } from '../../store/appStore';
import type { BillingReminderKind } from '../../store/billingReminderStore';
import { BILLING_URL } from '../../services/billingAccess';

type BillingReminderDialogProps = {
  open: boolean;
  kind: BillingReminderKind;
  onCancel: () => void;
};

function getBillingReminderCopy(
  kind: BillingReminderKind,
  t: (key: string) => string,
): {
  title: string;
  description: string;
  showOpenModels: boolean;
} {
  switch (kind) {
    case 'bundled-token-required':
      return {
        title: t('dialog:billing.creditsRequired'),
        description: t('dialog:billing.creditsDesc'),
        showOpenModels: true,
      };
    case 'quota-exhausted':
      return {
        title: t('dialog:billing.quotaExhausted'),
        description: t('dialog:billing.quotaDesc'),
        showOpenModels: false,
      };
    case 'chat-unavailable':
    default:
      return {
        title: t('dialog:billing.chatUnavailable'),
        description: t('dialog:billing.chatUnavailableDesc'),
        showOpenModels: false,
      };
  }
}

export function BillingReminderDialog({
  open,
  kind,
  onCancel,
}: BillingReminderDialogProps) {
  const { t } = useTranslation(['dialog', 'common']);
  useBlockingModal(open);
  const openModelsTab = useAppStore((state) => state.openModelsTab);

  const handleGoToBilling = useCallback(() => {
    window.open(BILLING_URL, '_blank');
    onCancel();
  }, [onCancel]);

  const handleOpenModels = useCallback(() => {
    openModelsTab();
    onCancel();
  }, [onCancel, openModelsTab]);

  useEffect(() => {
    if (!open) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onCancel();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onCancel]);

  if (!open) {
    return null;
  }

  const { title, description, showOpenModels } = getBillingReminderCopy(kind, t);

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/50" onMouseDown={onCancel} />
      <div
        className="relative w-[420px] max-w-[calc(100vw-32px)] rounded-2xl border border-border bg-editor-bg p-6 shadow-2xl"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-xl border border-border bg-titlebar-bg text-editor-fg shadow-sm">
          <OpenBrainLogo className="h-10 w-10" title="OpenBrain" />
        </div>
        <div className="mt-5 text-center">
          <div className="text-[22px] font-semibold leading-8 text-editor-fg">{title}</div>
          <div className="mt-2 text-sm leading-6 text-secondary-text">
            {description}
          </div>
        </div>
        <div className="mt-6 flex flex-wrap justify-center gap-2">
          <button
            type="button"
            className={`${UI_PILL_BTN_SECONDARY} ${UI_PILL_BTN_DIALOG}`}
            onClick={onCancel}
            autoFocus
          >
            {t('common:cancel')}
          </button>
          {showOpenModels ? (
            <button
              type="button"
              className={`${UI_PILL_BTN_SECONDARY} ${UI_PILL_BTN_DIALOG}`}
              onClick={handleOpenModels}
            >
              {t('dialog:billing.openModels')}
            </button>
          ) : null}
          <button
            type="button"
            className={`${UI_PILL_BTN_PRIMARY} ${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR} ${UI_PILL_BTN_DIALOG}`}
            onClick={handleGoToBilling}
          >
            {t('dialog:billing.goToBilling')}
          </button>
        </div>
      </div>
    </div>
  );
}
