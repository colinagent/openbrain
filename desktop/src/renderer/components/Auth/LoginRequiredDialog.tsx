import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { OpenBrainLogo } from '../Icons';
import { ChevronDownIcon, ChevronRightIcon } from '../Icons';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_EDITOR,
  UI_PILL_BTN_PRIMARY,
  UI_PILL_BTN_SECONDARY,
} from '../staticGlassCapsule';
import { writeClipboardText } from '../../services/clipboardService';
import { useBlockingModal } from '../../utils/useBlockingModal';
import { useAuthStore } from '../../store/authStore';
import type { LoginRequiredReason } from '../../store/loginRequiredStore';
import { translateLoginRequiredReason } from './LoginRequiredDialog.i18n';

type LoginRequiredDialogProps = {
  open: boolean;
  reason: LoginRequiredReason;
  onCancel: () => void;
};

type LoginMode = 'organization' | 'custom-gateway';

export function LoginRequiredDialog({
  open,
  reason,
  onCancel,
}: LoginRequiredDialogProps) {
  const { t } = useTranslation(['dialog', 'common', 'error']);
  useBlockingModal(open);
  const startLogin = useAuthStore((state) => state.startLogin);
  const loggedIn = useAuthStore((state) => state.loggedIn);
  const deviceCodeError = useAuthStore((state) => state.deviceCodeError);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loginUrl, setLoginUrl] = useState('');
  const [copied, setCopied] = useState(false);
  const [customGateway, setCustomGateway] = useState('');
  const [organizationCode, setOrganizationCode] = useState('');
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [loginMode, setLoginMode] = useState<LoginMode>('organization');
  const primaryPillClassName = `${UI_PILL_BTN_PRIMARY} ${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR}`;

  useEffect(() => {
    if (!open) {
      setBusy(false);
      setError(null);
      setLoginUrl('');
      setCopied(false);
      setCustomGateway('');
      setOrganizationCode('');
      setAdvancedOpen(false);
      setLoginMode('organization');
      return;
    }
    if (loggedIn) {
      onCancel();
      return;
    }
  }, [loggedIn, onCancel, open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') {
        return;
      }
      event.preventDefault();
      onCancel();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onCancel, open]);

  useEffect(() => {
    if (open && deviceCodeError) {
      setError(deviceCodeError);
    }
  }, [deviceCodeError, open]);

  const handleSignIn = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      const nextGateway = customGateway.trim();
      if (loginMode === 'custom-gateway' && !nextGateway) {
        setError(t('error:auth.gatewayRequired'));
        return;
      }
      const result = await startLogin({
        gateway: loginMode === 'custom-gateway' ? nextGateway : undefined,
        orgSlug: loginMode === 'organization' ? organizationCode.trim() || undefined : undefined,
      });
      if (result?.mode === 'device_code') {
        return;
      }
      const nextLoginUrl = (result?.loginUrl || '').trim();
      if (nextLoginUrl) {
        setLoginUrl(nextLoginUrl);
      }
    } catch (loginError) {
      const message = loginError instanceof Error ? loginError.message.trim() : '';
      setError(message || t('error:auth.startSignInFailed'));
    } finally {
      setBusy(false);
    }
  }, [customGateway, organizationCode, loginMode, startLogin]);

  const handleCopyLink = useCallback(async () => {
    const nextLoginUrl = loginUrl.trim();
    if (!nextLoginUrl) {
      return;
    }
    try {
      await writeClipboardText(nextLoginUrl);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch (copyError) {
      const message = copyError instanceof Error ? copyError.message.trim() : '';
      setError(message || t('error:auth.copyLinkFailed'));
    }
  }, [loginUrl]);

  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/50" onMouseDown={onCancel} />
      <div
        className="relative w-[420px] max-w-[calc(100vw-32px)] rounded-2xl border border-border bg-editor-bg p-6 shadow-2xl"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-xl border border-border bg-titlebar-bg text-editor-fg shadow-sm">
          <OpenBrainLogo className="h-10 w-10" title="OpenBrain" />
        </div>
        <div className="mt-5 text-center">
          <div className="text-[22px] font-semibold leading-8 text-editor-fg">{t('dialog:auth.signInRequired')}</div>
          <div className="mt-2 text-sm leading-6 text-secondary-text">
            {translateLoginRequiredReason(reason, t)}
          </div>
        </div>
        {loginUrl ? (
          <div className="mt-4 rounded border border-border bg-sidebar-bg px-3 py-2 text-left">
            <div className="text-[11px] uppercase tracking-wide text-secondary-text">{t('dialog:auth.signInLink')}</div>
            <div className="mt-1 break-all text-xs leading-5 text-prime-text">{loginUrl}</div>
          </div>
        ) : null}
        <div className="mt-5 text-left">
          <button
            type="button"
            className={`${UI_PILL_BTN_SECONDARY} w-full justify-between px-3 py-2 text-xs font-medium`}
            onClick={() => setAdvancedOpen((value) => !value)}
            disabled={busy}
            aria-expanded={advancedOpen}
          >
            <span>{t('dialog:auth.advancedOptions')}</span>
            {advancedOpen ? <ChevronDownIcon className="h-3.5 w-3.5" /> : <ChevronRightIcon className="h-3.5 w-3.5" />}
          </button>
          {advancedOpen ? (
            <div className="mt-3 rounded border border-border bg-sidebar-bg/60 p-3">
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  className={`rounded border px-3 py-2 text-left text-xs transition-colors ${
                    loginMode === 'organization'
                      ? 'border-highlight bg-editor-bg text-editor-fg'
                      : 'border-border bg-transparent text-secondary-text hover:text-highlight'
                  }`}
                  onClick={() => setLoginMode('organization')}
                  disabled={busy}
                >
                  <span className="block font-medium">{t('dialog:auth.orgCode')}</span>
                  <span className="mt-0.5 block text-[11px] text-secondary-text">{t('dialog:auth.orgCodeTeam')}</span>
                </button>
                <button
                  type="button"
                  className={`rounded border px-3 py-2 text-left text-xs transition-colors ${
                    loginMode === 'custom-gateway'
                      ? 'border-highlight bg-editor-bg text-editor-fg'
                      : 'border-border bg-transparent text-secondary-text hover:text-highlight'
                  }`}
                  onClick={() => setLoginMode('custom-gateway')}
                  disabled={busy}
                >
                  <span className="block font-medium">{t('dialog:auth.privateServer')}</span>
                  <span className="mt-0.5 block text-[11px] text-secondary-text">{t('dialog:auth.privateServerHint')}</span>
                </button>
              </div>

              {loginMode === 'organization' ? (
                <div className="mt-4">
                  <label className="text-[11px] uppercase tracking-wide text-secondary-text" htmlFor="openbrain-login-org">
                    {t('dialog:auth.orgCode')}
                  </label>
                  <input
                    id="openbrain-login-org"
                    type="text"
                    value={organizationCode}
                    onChange={(event) => setOrganizationCode(event.target.value)}
                    placeholder={t('dialog:auth.orgPlaceholder')}
                    className="mt-1 w-full rounded border border-border bg-editor-bg px-3 py-2 text-xs text-editor-fg outline-none focus:border-highlight"
                    disabled={busy}
                  />
                  <div className="mt-1 text-[11px] leading-4 text-secondary-text">
                    {t('dialog:auth.orgHelp')}
                  </div>
                </div>
              ) : null}

              {loginMode === 'custom-gateway' ? (
                <div className="mt-4">
                  <label className="text-[11px] uppercase tracking-wide text-secondary-text" htmlFor="openbrain-login-gateway">
                    {t('dialog:auth.privateServer')}
                  </label>
                  <input
                    id="openbrain-login-gateway"
                    type="url"
                    value={customGateway}
                    onChange={(event) => setCustomGateway(event.target.value)}
                    placeholder={t('dialog:auth.gatewayPlaceholder')}
                    className="mt-1 w-full rounded border border-border bg-editor-bg px-3 py-2 text-xs text-editor-fg outline-none focus:border-highlight"
                    disabled={busy}
                  />
                  <div className="mt-1 text-[11px] leading-4 text-secondary-text">
                    {t('dialog:auth.gatewayHelp')}
                  </div>
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
        {error ? (
          <div className="mt-4 rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-600 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
            {error}
          </div>
        ) : null}
        <div className="mt-6 flex justify-center">
          <div className="login-required-dialog-actions flex gap-2">
            <button
              type="button"
              className={`${UI_PILL_BTN_SECONDARY} min-w-[112px] px-5 py-2 text-sm font-medium`}
              onClick={onCancel}
            >
              {t('common:cancel')}
            </button>
            <button
              type="button"
              className={`${UI_PILL_BTN_SECONDARY} min-w-[112px] px-5 py-2 text-sm font-medium`}
              onClick={handleCopyLink}
              disabled={!loginUrl}
            >
              {copied ? t('dialog:auth.copied') : t('dialog:auth.copyLink')}
            </button>
            <button
              type="button"
              className={`${primaryPillClassName} min-w-[112px] px-5 py-2 text-sm`}
              onClick={handleSignIn}
              disabled={busy}
              autoFocus
            >
              {busy ? t('dialog:auth.opening') : t('dialog:auth.signIn')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
