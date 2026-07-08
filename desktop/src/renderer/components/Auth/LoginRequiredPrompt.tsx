import React from 'react';

import { showLoginRequiredDialog, type LoginRequiredReason } from '../../store/loginRequiredStore';
import { LogInIcon } from '../Icons';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_EDITOR,
  OP_SG_CAPSULE_ON_SIDEBAR,
  UI_PILL_BTN_PRIMARY,
} from '../staticGlassCapsule';

type LoginRequiredPromptProps = {
  reason?: LoginRequiredReason;
  title?: string;
  description?: string;
  actionLabel?: string;
  className?: string;
  /** Capsule substrate — sidebar for Messenger list, editor for main pane empty states */
  substrate?: 'editor' | 'sidebar';
};

export function LoginRequiredPrompt({
  reason = 'chat',
  title = 'Sign in required',
  description = 'Sign in to continue.',
  actionLabel = 'Sign in',
  className = 'flex h-full min-h-0 items-center justify-center bg-editor-bg text-center',
  substrate = 'editor',
}: LoginRequiredPromptProps) {
  const capsuleSubstrate = substrate === 'sidebar'
    ? OP_SG_CAPSULE_ON_SIDEBAR
    : OP_SG_CAPSULE_ON_EDITOR;

  return (
    <div className={className}>
      <div className="max-w-xs px-6">
        <div className="text-sm font-medium text-prime-text">{title}</div>
        <div className="mt-1 text-xs leading-5 text-tertiary-text">{description}</div>
        <button
          type="button"
          className={`login-required-prompt-sign-in ${UI_PILL_BTN_PRIMARY} ${OP_SG_CAPSULE} ${capsuleSubstrate} mt-4 px-4 py-2 text-xs`}
          onClick={() => showLoginRequiredDialog(reason)}
        >
          <LogInIcon className="h-3.5 w-3.5" />
          {actionLabel}
        </button>
      </div>
    </div>
  );
}
