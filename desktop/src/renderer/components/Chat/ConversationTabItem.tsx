import React from 'react';
import type { AwaitingUserState, ConversationRunStatus } from '../../store/chatWorkspaceStore';
import { CloseButton, PinIcon } from '../Icons';
import {
  ACTIVE_TAB_LABEL_CLASS,
  TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS,
  TAB_ITEM_FLEX_STYLE,
  getTabCloseButtonClassName,
  getTabShellClassName,
} from '../tabLayout';
import { ConversationStatusLight } from './ConversationStatusLight';

export function ConversationTabItem({
  title,
  buttonTitle,
  closeLabel,
  isSelected,
  isOpenInEditor = false,
  isPinned = false,
  status,
  awaitingUser = null,
  onSelect,
  onClose,
}: {
  title: string;
  buttonTitle: string;
  closeLabel: string;
  isSelected: boolean;
  isOpenInEditor?: boolean;
  isPinned?: boolean;
  status: ConversationRunStatus;
  awaitingUser?: AwaitingUserState | null;
  onSelect: () => void;
  onClose: (event: React.MouseEvent<HTMLButtonElement>) => void;
}) {
  const isSelectedOutsideEditor = isSelected && !isOpenInEditor;
  const shellClassName = getTabShellClassName(
    isOpenInEditor,
    isSelectedOutsideEditor ? 'conversation-tab-selected' : '',
  );
  const titleClassName = [
    'min-w-0 flex-1 truncate',
    isOpenInEditor ? ACTIVE_TAB_LABEL_CLASS : '',
    isSelectedOutsideEditor ? 'conversation-tab-selected-title' : '',
  ].filter(Boolean).join(' ');

  return (
    <div
      className={shellClassName}
      style={TAB_ITEM_FLEX_STYLE}
    >
      <button
        type="button"
        className="flex h-full min-w-0 flex-1 items-center gap-1.5 text-left text-sm"
        onClick={onSelect}
        title={buttonTitle}
      >
        <ConversationStatusLight status={status} awaitingUser={awaitingUser} />
        {isPinned && (
          <PinIcon
            className={`h-3.5 w-3.5 flex-shrink-0 ${isOpenInEditor ? ACTIVE_TAB_LABEL_CLASS : ''}`}
          />
        )}
        <span className={titleClassName}>
          {title}
        </span>
      </button>
      <CloseButton
        className={getTabCloseButtonClassName('bg-editor-bg', TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS)}
        onClick={onClose}
        aria-label={closeLabel}
        variant="inline"
      />
    </div>
  );
}
