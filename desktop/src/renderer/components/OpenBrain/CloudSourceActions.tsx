import React from 'react';
import { createPortal } from 'react-dom';

import { canManageOpenBrainSource, type LocalOpenBrainWorkspace } from '../../store/openBrainStore';
import { useBlockingModal } from '../../utils/useBlockingModal';
import { PopupMenuItem, PopupMenuSeparator } from '../PopupMenu';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_EDITOR,
  UI_PILL_BTN_DIALOG,
  UI_PILL_BTN_PRIMARY,
  UI_PILL_BTN_SECONDARY,
} from '../staticGlassCapsule';
import { ChatLineIcon, FolderIcon, SettingsIcon, TrashIcon, UserIcon } from '../Icons';

export type CloudSourceActionPayload = {
  disableQueries?: boolean;
  enableQueries?: boolean;
  disableSync?: boolean;
  hardDelete?: boolean;
  confirmWorkspaceID?: string;
  confirmName?: string;
};

type CloudSourceActionResult = {
  disabledQueries?: boolean;
  enabledQueries?: boolean;
  disabledSync?: boolean;
  hardDeleted?: boolean;
};

type CloudSourceActionMenuItemsProps = {
  source: LocalOpenBrainWorkspace;
  disabled?: boolean;
  sourceLinked?: boolean;
  onChatWithSource?: (source: LocalOpenBrainWorkspace) => void;
  onToggleSourceLink?: (source: LocalOpenBrainWorkspace) => void;
  onShareSource?: (source: LocalOpenBrainWorkspace) => void;
  onBindSource?: (source: LocalOpenBrainWorkspace) => void;
  onApplyAction: (source: LocalOpenBrainWorkspace, action: CloudSourceActionPayload) => void;
  onOpenHardDelete: (source: LocalOpenBrainWorkspace) => void;
  showCloudManagement?: boolean;
};

export function CloudSourceActionMenuItems({
  source,
  disabled = false,
  sourceLinked = true,
  onChatWithSource,
  onToggleSourceLink,
  onShareSource,
  onBindSource,
  onApplyAction,
  onOpenHardDelete,
  showCloudManagement = true,
}: CloudSourceActionMenuItemsProps) {
  const queriesDisabled = source.disabledQueries === true;
  const canManage = canManageOpenBrainSource(source);
  return (
    <>
      {onChatWithSource ? (
        <>
          <PopupMenuItem disabled={disabled} onClick={() => onChatWithSource(source)}>
            <ChatLineIcon className="h-4 w-4 opacity-70" />
            <span>Chat in this source</span>
          </PopupMenuItem>
          <PopupMenuSeparator />
        </>
      ) : null}
      {onToggleSourceLink ? (
        <>
          <PopupMenuItem disabled={disabled} onClick={() => onToggleSourceLink(source)}>
            <SettingsIcon className="h-4 w-4 opacity-70" />
            <span>{sourceLinked ? 'Disconnect source' : 'Connect source'}</span>
          </PopupMenuItem>
          <PopupMenuSeparator />
        </>
      ) : null}
      {onBindSource ? (
        <>
          <PopupMenuItem disabled={disabled} onClick={() => onBindSource(source)}>
            <FolderIcon className="h-4 w-4 opacity-70" />
            <span>{source.path ? 'Rebind folder...' : 'Bind folder...'}</span>
          </PopupMenuItem>
          <PopupMenuSeparator />
        </>
      ) : null}
      {showCloudManagement && onShareSource && canManage ? (
        <>
          <PopupMenuItem disabled={disabled} onClick={() => onShareSource(source)}>
            <UserIcon className="h-4 w-4 opacity-70" />
            <span>Share source...</span>
          </PopupMenuItem>
          <PopupMenuSeparator />
        </>
      ) : null}
      {showCloudManagement && canManage ? (
        <>
          <PopupMenuItem disabled={disabled} onClick={() => onApplyAction(source, queriesDisabled ? { enableQueries: true } : { disableQueries: true })}>
            <SettingsIcon className="h-4 w-4 opacity-70" />
            <span>{queriesDisabled ? 'Enable cloud queries' : 'Disable cloud queries'}</span>
          </PopupMenuItem>
          <PopupMenuItem disabled={disabled} onClick={() => onApplyAction(source, { disableSync: true })}>
            <SettingsIcon className="h-4 w-4 opacity-70" />
            <span>Turn off sync</span>
          </PopupMenuItem>
          <PopupMenuSeparator />
          <PopupMenuItem disabled={disabled} onClick={() => onOpenHardDelete(source)}>
            <TrashIcon className="h-4 w-4 opacity-70" />
            <span>Permanently delete this cloud source...</span>
          </PopupMenuItem>
        </>
      ) : null}
    </>
  );
}

export function sourceActionSuccessMessage(
  action: CloudSourceActionPayload,
  result: CloudSourceActionResult,
): string {
  const hardDeleted = action.hardDelete || result.hardDeleted === true;
  const disabledQueries = action.disableQueries || result.disabledQueries === true;
  const enabledQueries = action.enableQueries || result.enabledQueries === true;
  const disabledSync = action.disableSync || result.disabledSync === true;
  if (hardDeleted) {
    return 'Cloud source deleted. Local files, GitHub repository, and other accounts\' cloud sources were not deleted.';
  }
  if (disabledQueries && disabledSync) {
    return 'Cloud queries disabled and sync stopped. Local files and GitHub repository were not deleted.';
  }
  if (disabledQueries) {
    return 'Cloud queries disabled';
  }
  if (enabledQueries) {
    return 'Cloud queries enabled.';
  }
  return 'Cloud sync stopped. Local files and GitHub repository were not deleted.';
}

type HardDeleteCloudSourceDialogProps = {
  open: boolean;
  source: LocalOpenBrainWorkspace | null;
  busy: boolean;
  error?: string | null;
  onCancel: () => void;
  onSubmit: (action: CloudSourceActionPayload) => void;
};

function CloudSourceDetailRow({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <div className="grid grid-cols-[92px_minmax(0,1fr)] gap-2">
      <span className="text-tertiary-text">{label}</span>
      <span className="break-all font-mono text-editor-fg">{value}</span>
    </div>
  );
}

export function HardDeleteCloudSourceDialog({
  open,
  source,
  busy,
  error,
  onCancel,
  onSubmit,
}: HardDeleteCloudSourceDialogProps) {
  useBlockingModal(open);

  if (!open || !source) {
    return null;
  }

  const sourceName = source.name.trim() || source.sourceID;
  const workspaceID = (source.workspaceID || source.sourceID).trim();
  const sourceID = (source.sourceID || workspaceID).trim();
  const repoURL = (source.remoteURL || '').trim();

  const handlePrimary = () => {
    if (busy) {
      return;
    }
    onSubmit({
      hardDelete: true,
      confirmWorkspaceID: workspaceID,
      confirmName: sourceName,
    });
  };

  const handleCancelMouseDown = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    if (!busy) {
      onCancel();
    }
  };

  const handleDeleteMouseDown = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    handlePrimary();
  };

  const handleButtonKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>, action: () => void) => {
    if (event.key !== 'Enter' && event.key !== ' ') {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    action();
  };

  return createPortal(
    <div className="no-drag pointer-events-auto fixed inset-0 z-[90] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/50" onMouseDown={busy ? undefined : onCancel} />
      <div
        className="no-drag pointer-events-auto relative z-10 w-[540px] max-w-[calc(100vw-32px)] rounded border border-border bg-editor-bg p-5 shadow-2xl"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded border border-border bg-titlebar-bg text-accent">
            <TrashIcon className="h-5 w-5" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-base font-semibold text-editor-fg">Permanently delete this cloud source</div>
            <div className="mt-1 truncate text-sm text-secondary-text">{sourceName}</div>
          </div>
        </div>

        <div className="mt-4 rounded border border-border bg-titlebar-bg/50 px-3 py-2 text-sm leading-5 text-secondary-text">
          This deletes only the selected cloud source for the current account. Local files, the GitHub repository, and other accounts' cloud sources for the same repo will not be deleted.
          <div className="mt-2 grid gap-1 text-xs">
            <CloudSourceDetailRow label="Source name" value={sourceName} />
            <CloudSourceDetailRow label="Workspace ID" value={workspaceID} />
            <CloudSourceDetailRow label="Source ID" value={sourceID} />
            {repoURL ? (
              <CloudSourceDetailRow label="Repo" value={repoURL} />
            ) : null}
          </div>
        </div>

        <div className="mt-4 rounded border border-accent/60 px-3 py-3">
          <div className="text-sm font-medium text-editor-fg">Review before deleting</div>
          <div className="mt-1 text-xs leading-5 text-secondary-text">
            Clicking Delete cloud source is the second confirmation. It deletes cloud records and indexed data, disables cloud queries, and turns off sync for this cloud source.
          </div>
        </div>

        {error ? <div className="mt-3 text-sm text-red-400 whitespace-pre-wrap">{error}</div> : null}

        <div className="mt-5 flex flex-wrap justify-end gap-2">
          <button
            type="button"
            className={`no-drag pointer-events-auto ${UI_PILL_BTN_SECONDARY} ${UI_PILL_BTN_DIALOG} disabled:opacity-50`}
            onMouseDown={handleCancelMouseDown}
            onKeyDown={(event) => handleButtonKeyDown(event, onCancel)}
            disabled={busy}
          >
            Cancel
          </button>
          <button
            type="button"
            className={`no-drag pointer-events-auto ${UI_PILL_BTN_PRIMARY} ${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR} ${UI_PILL_BTN_DIALOG} disabled:cursor-not-allowed disabled:opacity-50`}
            onMouseDown={handleDeleteMouseDown}
            onKeyDown={(event) => handleButtonKeyDown(event, handlePrimary)}
            disabled={busy}
          >
            {busy ? 'Deleting...' : 'Delete cloud source'}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
