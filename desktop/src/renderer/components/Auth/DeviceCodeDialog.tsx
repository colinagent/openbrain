import React, { useEffect, useState, useCallback } from 'react';
import { writeClipboardText } from '../../services/clipboardService';
import { UI_PILL_BTN_COMPACT, UI_PILL_BTN_SECONDARY } from '../staticGlassCapsule';
import { useBlockingModal } from '../../utils/useBlockingModal';

type DeviceCodeDialogProps = {
  open: boolean;
  userCode: string;
  verificationUri: string;
  expiresAt: number;
  onClose: () => void;
};

export const DeviceCodeDialog: React.FC<DeviceCodeDialogProps> = ({
  open,
  userCode,
  verificationUri,
  expiresAt,
  onClose,
}) => {
  useBlockingModal(open);

  const [copied, setCopied] = useState(false);
  const [remainingSeconds, setRemainingSeconds] = useState(0);

  useEffect(() => {
    if (!open) return;

    const update = () => {
      const remaining = Math.max(0, Math.floor((expiresAt - Date.now()) / 1000));
      setRemainingSeconds(remaining);
    };

    update();
    const interval = setInterval(update, 1000);
    return () => clearInterval(interval);
  }, [open, expiresAt]);

  const handleCopy = useCallback(async () => {
    try {
      await writeClipboardText(userCode);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // ignore
    }
  }, [userCode]);

  const handleOpenBrowser = useCallback(() => {
    window.open(verificationUri, '_blank');
  }, [verificationUri]);

  if (!open) return null;

  const minutes = Math.floor(remainingSeconds / 60);
  const seconds = remainingSeconds % 60;
  const timeDisplay = `${minutes}:${seconds.toString().padStart(2, '0')}`;

  return (
    <div className="fixed inset-0 z-[90] flex items-center justify-center bg-logo-light">
      <div className="bg-editor-bg border border-border rounded-lg w-[400px] overflow-hidden shadow-lg">
        <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-titlebar-bg">
          <span className="text-sm font-semibold text-sidebar-fg">Sign in to OpenBrain</span>
          <button className="dialog-text-btn" onClick={onClose}>
            Cancel
          </button>
        </div>

        <div className="p-6 text-center">
          <p className="text-sm text-secondary-text mb-4">
            Enter this code in your browser to sign in:
          </p>

          <div className="flex items-center justify-center gap-2 mb-4">
            <div className="font-mono text-2xl font-bold tracking-widest text-editor-fg bg-sidebar-bg px-4 py-2 rounded border border-border">
              {userCode}
            </div>
            <button
              onClick={handleCopy}
              className={`${UI_PILL_BTN_SECONDARY} ${UI_PILL_BTN_COMPACT}`}
              title="Copy code"
            >
              {copied ? '✓' : 'Copy'}
            </button>
          </div>

          <button
            onClick={handleOpenBrowser}
            className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor mb-4 w-full px-4 py-2.5 text-sm"
          >
            Open Browser
          </button>

          <p className="text-xs text-secondary-text">
            Code expires in <span className="font-mono">{timeDisplay}</span>
          </p>
        </div>

        <div className="px-6 pb-4 text-center">
          <p className="text-xs text-secondary-text">
            Waiting for authorization...
          </p>
        </div>
      </div>
    </div>
  );
};
