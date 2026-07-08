import React from 'react';
import type { AwaitingUserState, ConversationRunStatus } from '../../store/chatWorkspaceStore';
import { LoaderIcon } from '../Icons';

function truncateStatusMessage(message: string): string {
  const normalized = (message || '').trim().replace(/\s+/g, ' ');
  if (!normalized) {
    return '';
  }
  return normalized.length > 96 ? `${normalized.slice(0, 93)}...` : normalized;
}

function getAwaitingUserPreview(awaitingUser?: AwaitingUserState | null): string {
  const question = awaitingUser?.questions?.[awaitingUser.currentIndex]?.question || awaitingUser?.questions?.[0]?.question || '';
  return truncateStatusMessage(question);
}

export function getConversationStatusLightMeta(
  status: ConversationRunStatus,
  awaitingUser?: AwaitingUserState | null,
): {
  title: string;
  ariaLabel: string;
} | null {
  if (status === 'running') {
    return {
      title: 'Conversation running',
      ariaLabel: 'Conversation running',
    };
  }
  if (status === 'complete') {
    return {
      title: 'AI response complete',
      ariaLabel: 'AI response complete',
    };
  }
  if (status === 'awaiting_user') {
    const suffix = getAwaitingUserPreview(awaitingUser);
    const base = 'Conversation waiting for your input';
    return {
      title: suffix ? `${base}: ${suffix}` : base,
      ariaLabel: suffix ? `${base}: ${suffix}` : base,
    };
  }
  return null;
}

export function ConversationStatusLight({
  status,
  awaitingUser = null,
  className = '',
}: {
  status: ConversationRunStatus;
  awaitingUser?: AwaitingUserState | null;
  className?: string;
}) {
  const meta = getConversationStatusLightMeta(status, awaitingUser);
  if (!meta) {
    return (
      <span
        className={`conversation-status-light-slot ${className}`.trim()}
        aria-hidden="true"
      />
    );
  }
  return (
    <span
      className={`conversation-status-light-slot ${className}`.trim()}
      title={meta.title}
      role="img"
      aria-label={meta.ariaLabel}
    >
      {status === 'running' ? (
        <LoaderIcon className="conversation-status-spinner" />
      ) : (
        <span className="conversation-status-light" data-status={status} />
      )}
    </span>
  );
}
