import React, { useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';

import type { LocalOpenBrainWorkspace } from '../../store/openBrainStore';
import type { OpenBrainPublicBrainProfile, OpenBrainSourceShare } from '../../services/openBrainService';
import { useBlockingModal } from '../../utils/useBlockingModal';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_EDITOR,
  UI_PILL_BTN_DIALOG,
  UI_PILL_BTN_PRIMARY,
  UI_PILL_BTN_SECONDARY,
} from '../staticGlassCapsule';

type SourceShareDialogProps = {
  open: boolean;
  source: LocalOpenBrainWorkspace | null;
  share: OpenBrainSourceShare | null;
  publicProfile: OpenBrainPublicBrainProfile | null;
  busy: boolean;
  error?: string | null;
  onCancel: () => void;
  onShareEmail: (email: string) => Promise<void>;
  onRevokeUser: (uid: string) => Promise<void>;
  onMakePublic: () => Promise<void>;
  onMakePrivate: () => Promise<void>;
  onUpdatePublicProfile: (description: string) => Promise<void>;
};

export function SourceShareDialog({
  open,
  source,
  share,
  publicProfile,
  busy,
  error,
  onCancel,
  onShareEmail,
  onRevokeUser,
  onMakePublic,
  onMakePrivate,
  onUpdatePublicProfile,
}: SourceShareDialogProps) {
  useBlockingModal(open);
  const [email, setEmail] = useState('');
  const [riskAcknowledged, setRiskAcknowledged] = useState(false);
  const [description, setDescription] = useState('');

  useEffect(() => {
    if (!open) {
      return;
    }
    setEmail('');
    setRiskAcknowledged(false);
    setDescription(publicProfile?.description || '');
  }, [open, publicProfile?.description, source?.sourceID]);

  const isPublic = share?.public?.status === 'active';
  const sourceName = source?.name || source?.path || source?.sourceID || 'OpenBrain source';
  const publicName = publicProfile?.name || 'Your public brain';
  const publicUsername = publicProfile?.username ? `@${publicProfile.username}` : '';
  const normalizedDescription = description.trim();
  const savedDescription = (publicProfile?.description || '').trim();
  const canShareEmail = email.trim().includes('@') && !busy;
  const canMakePublic = !isPublic && riskAcknowledged && !busy;
  const canSaveDescription = !busy && normalizedDescription.length <= 280 && normalizedDescription !== savedDescription;
  const users = useMemo(() => share?.users || [], [share?.users]);

  if (!open || !source) {
    return null;
  }

  return createPortal(
    <div className="no-drag fixed inset-0 z-[70] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/50" onMouseDown={busy ? undefined : onCancel} />
      <div
        className="relative flex max-h-[calc(100vh-32px)] w-[620px] max-w-[calc(100vw-32px)] flex-col overflow-hidden rounded border border-border bg-editor-bg shadow-2xl"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="border-b border-border px-5 py-4">
          <div className="text-base font-semibold text-editor-fg">Share source</div>
          <div className="mt-1 truncate text-sm text-secondary-text">{sourceName}</div>
        </div>

        <div className="min-h-0 flex-1 space-y-5 overflow-auto px-5 py-4">
          <section className="rounded border border-border bg-titlebar-bg/40 px-3 py-3">
            <div className="text-sm font-semibold text-editor-fg">Public brain</div>
            <div className="mt-2 truncate text-sm font-medium text-editor-fg">{publicName}</div>
            <div className="mt-1 text-xs leading-5 text-secondary-text">
              {publicUsername ? `${publicUsername} · ` : ''}{publicProfile?.activeSourceCount ?? 0} public sources
            </div>
            <div className="mt-3">
              <label className="text-xs font-medium text-editor-fg" htmlFor="openbrain-public-brain-description">
                Description
              </label>
              <textarea
                id="openbrain-public-brain-description"
                className="mt-2 h-20 w-full resize-none rounded border border-border bg-editor-bg px-2 py-2 text-xs leading-5 text-editor-fg outline-none focus:border-highlight"
                value={description}
                maxLength={280}
                onChange={(event) => setDescription(event.currentTarget.value)}
                placeholder="Describe what people can learn from your public brain."
                disabled={busy}
              />
              <div className="mt-2 flex items-center justify-between gap-3">
                <span className={`text-[11px] ${description.length > 280 ? 'text-red-400' : 'text-secondary-text'}`}>
                  {description.length}/280
                </span>
                <button
                  type="button"
                  className={`${UI_PILL_BTN_SECONDARY} ${UI_PILL_BTN_DIALOG} disabled:cursor-not-allowed disabled:opacity-50`}
                  disabled={!canSaveDescription}
                  onClick={() => void onUpdatePublicProfile(normalizedDescription)}
                >
                  Save description
                </button>
              </div>
            </div>
          </section>

          <section className="rounded border border-border px-3 py-3">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <div className="text-sm font-semibold text-editor-fg">Public access</div>
                <div className="mt-1 text-xs leading-5 text-secondary-text">
                  {isPublic ? 'Anyone using OpenBrain can add your public brain and read this source.' : 'Make this source public to include it in your public brain.'}
                </div>
              </div>
              {isPublic ? (
                <button
                  type="button"
                  className={`${UI_PILL_BTN_SECONDARY} ${UI_PILL_BTN_DIALOG} disabled:opacity-50`}
                  disabled={busy}
                  onClick={() => void onMakePrivate()}
                >
                  Make private
                </button>
              ) : (
                <button
                  type="button"
                  className={`${UI_PILL_BTN_PRIMARY} ${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR} ${UI_PILL_BTN_DIALOG} disabled:cursor-not-allowed disabled:opacity-50`}
                  disabled={!canMakePublic}
                  onClick={() => void onMakePublic()}
                >
                  Make public
                </button>
              )}
            </div>
            {!isPublic ? (
              <label className="mt-3 flex cursor-pointer items-start gap-2 rounded border border-accent/50 px-3 py-2 text-xs leading-5 text-secondary-text">
                <input
                  type="checkbox"
                  className="mt-1 shrink-0"
                  checked={riskAcknowledged}
                  disabled={busy}
                  onChange={(event) => setRiskAcknowledged(event.currentTarget.checked)}
                />
                <span>
                  I understand this may expose source content to people who add my public brain. I will not publish secrets, private code, customer data, tokens, or personal information.
                </span>
              </label>
            ) : null}
          </section>

          <section className="rounded border border-border px-3 py-3">
            <div className="text-sm font-semibold text-editor-fg">Read-only users</div>
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
              <input
                type="email"
                className="h-9 min-w-0 flex-1 rounded border border-border bg-editor-bg px-2 text-sm text-editor-fg outline-none focus:border-highlight"
                value={email}
                onChange={(event) => setEmail(event.currentTarget.value)}
                placeholder="email@example.com"
                disabled={busy}
              />
              <button
                type="button"
                className={`${UI_PILL_BTN_PRIMARY} ${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR} ${UI_PILL_BTN_DIALOG} disabled:cursor-not-allowed disabled:opacity-50`}
                disabled={!canShareEmail}
                onClick={() => void onShareEmail(email.trim()).then(() => setEmail(''))}
              >
                Add user
              </button>
            </div>
            <div className="mt-3 max-h-40 space-y-2 overflow-auto">
              {users.length === 0 ? (
                <div className="text-xs text-secondary-text">No read-only users.</div>
              ) : users.map((user) => {
                const label = user.email || user.name || user.username || user.uid;
                return (
                  <div key={user.uid} className="flex items-center justify-between gap-3 rounded border border-border bg-titlebar-bg/35 px-3 py-2">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium text-editor-fg">{label}</div>
                      {user.username ? <div className="truncate text-[11px] text-secondary-text">@{user.username}</div> : null}
                    </div>
                    <button
                      type="button"
                      className={`${UI_PILL_BTN_SECONDARY} ${UI_PILL_BTN_DIALOG} disabled:opacity-50`}
                      disabled={busy}
                      onClick={() => void onRevokeUser(user.uid)}
                    >
                      Remove
                    </button>
                  </div>
                );
              })}
            </div>
          </section>

          {error ? <div className="text-sm text-red-400 whitespace-pre-wrap">{error}</div> : null}
        </div>

        <div className="flex justify-end border-t border-border px-5 py-4">
          <button
            type="button"
            className={`${UI_PILL_BTN_SECONDARY} ${UI_PILL_BTN_DIALOG} disabled:opacity-50`}
            onClick={onCancel}
            disabled={busy}
          >
            Close
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
