import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import { writeClipboardText } from '../../services/clipboardService';
import { useAppStore } from '../../store/appStore';
import { useToastStore } from '../../store/toastStore';
import { DeleteConfirmDialog } from '../FileExplorer/DeleteConfirmDialog';
import { FileTreeChildren } from '../FileExplorer/FileTreeChildren';
import { FileTreeContextMenu } from '../FileExplorer/FileTreeContextMenu';
import { RenameEntryDialog } from '../FileExplorer/RenameEntryDialog';
import { TreeImportConflictDialog } from '../FileExplorer/TreeImportConflictDialog';
import { formatDeleteConfirmMessage } from '../FileExplorer/fileTreeDelete';
import { useFileTreeInteractionController } from '../FileExplorer/useFileTreeInteractionController';
import { useFileTreeCrudController } from '../FileExplorer/useFileTreeCrudController';
import { useFileTreeSelectionStore } from '../FileExplorer/fileTreeSelectionStore';
import { useTreeImportDropController } from '../FileExplorer/useTreeImportDropController';
import { PlusIcon } from '../Icons';
import { IconButton } from '../IconButton';
import { PopupMenu, PopupMenuItem } from '../PopupMenu';

const MENU_VERTICAL_PADDING = 8;
const MENU_ITEM_HEIGHT = 34;

export function AgentsSidebar() {
  const connectionState = useAppStore((state) => state.connectionState);
  const storedAgentsRoot = useAppStore((state) => state.agentsRootDir);
  const ensureDerivedDirs = useAppStore((state) => state.ensureDerivedDirs);
  const ensureDirectory = useAppStore((state) => state.ensureDirectory);
  const refreshAgentNodes = useAppStore((state) => state.refreshAgentNodes);
  const loadDirectory = useAppStore((state) => state.loadDirectory);
  const expandedDirs = useAppStore((state) => state.expandedDirs);
  const toggleDir = useAppStore((state) => state.toggleDir);
  const createFile = useAppStore((state) => state.createFile);
  const createFolder = useAppStore((state) => state.createFolder);
  const deleteEntry = useAppStore((state) => state.deleteEntry);
  const renameEntry = useAppStore((state) => state.renameEntry);
  const openFile = useAppStore((state) => state.openFile);
  const pushToast = useToastStore((state) => state.pushToast);

  const connected = connectionState === 'connected';
  const [agentsRoot, setAgentsRoot] = useState<string | null>(storedAgentsRoot);
  const [loading, setLoading] = useState(!storedAgentsRoot);
  const [rootMenuOpen, setRootMenuOpen] = useState(false);
  const rootMenuRef = useRef<HTMLDivElement | null>(null);
  const rootMenuTriggerRef = useRef<HTMLDivElement | null>(null);

  const crud = useFileTreeCrudController({
    connected,
    expandedDirs,
    toggleDir,
    createFile,
    createFolder,
    deleteEntry,
    renameEntry,
    openFile,
  });

  const interaction = useFileTreeInteractionController({
    enabled: connected && !!agentsRoot,
    ensureExpanded: (dir) => {
      if (!expandedDirs.has(dir)) {
        toggleDir(dir);
      }
    },
    getDefaultPasteTargetDir: () => agentsRoot,
    selectionResetKey: agentsRoot || '',
    onTransferCommitted: async () => {
      await refreshAgentNodes({ force: true });
    },
  });

  const treeImport = useTreeImportDropController({
    enabled: connected && !!agentsRoot,
    ensureExpanded: (dir) => {
      if (!expandedDirs.has(dir)) {
        toggleDir(dir);
      }
    },
    onImportCommitted: async (targetDir) => {
      await loadDirectory(targetDir);
      await refreshAgentNodes({ force: true });
    },
  });

  useEffect(() => {
    if (!rootMenuOpen) {
      return;
    }
    const onMouseDown = (event: MouseEvent) => {
      if (rootMenuRef.current?.contains(event.target as Node)) {
        return;
      }
      setRootMenuOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setRootMenuOpen(false);
      }
    };
    window.addEventListener('mousedown', onMouseDown, true);
    window.addEventListener('keydown', onKeyDown, true);
    return () => {
      window.removeEventListener('mousedown', onMouseDown, true);
      window.removeEventListener('keydown', onKeyDown, true);
    };
  }, [rootMenuOpen]);

  useEffect(() => {
    if (!connected) {
      setAgentsRoot(null);
      setRootMenuOpen(false);
      setLoading(false);
      return;
    }

    let active = true;
    if (!agentsRoot) {
      setLoading(true);
    }

    const loadRoot = async () => {
      try {
        const derived = await ensureDerivedDirs({ force: true });
        if (!active) {
          return;
        }
        const root = (derived?.agentsDir || '').trim();
        if (!root) {
          setAgentsRoot(null);
          return;
        }

        await ensureDirectory(root);
        if (!active) {
          return;
        }

        setAgentsRoot(root);
        await refreshAgentNodes({ force: true });
      } catch (error) {
        console.error('Failed to load agents root:', error);
        if (active) {
          setAgentsRoot(null);
        }
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    };

    void loadRoot();

    return () => {
      active = false;
    };
  }, [agentsRoot, connected, ensureDerivedDirs, ensureDirectory, refreshAgentNodes]);

  const startRootInlineCreate = async (kind: 'file' | 'folder') => {
    setRootMenuOpen(false);
    const root = (agentsRoot || '').trim();
    if (!root) {
      return;
    }
    await ensureDirectory(root);
    crud.startInlineCreateAtDir(root, kind, 0);
  };

  const startContextInlineCreate = (kind: 'file' | 'folder') => {
    crud.startInlineCreate(kind);
  };

  const rootCreateActions = useMemo(() => {
    const canCreate = connected && !!agentsRoot;
    return [
      {
        label: 'New File...',
        disabled: !canCreate,
        onClick: () => {
          void startRootInlineCreate('file');
        },
      },
      {
        label: 'New Folder...',
        disabled: !canCreate,
        onClick: () => {
          void startRootInlineCreate('folder');
        },
      },
      {
        label: 'Paste',
        disabled: !(connected && !!agentsRoot && interaction.clipboardItems.length > 0),
        onClick: () => {
          if (agentsRoot) {
            void interaction.pasteInto(agentsRoot);
          }
        },
      },
    ];
  }, [agentsRoot, connected, interaction]);

  const contextCreateActions = useMemo(() => {
    const canCreate = connected && !!agentsRoot;
    return [
      {
        label: 'New File...',
        disabled: !canCreate,
        onClick: () => {
          startContextInlineCreate('file');
        },
      },
      {
        label: 'New Folder...',
        disabled: !canCreate,
        onClick: () => {
          startContextInlineCreate('folder');
        },
      },
      {
        label: 'Paste',
        disabled: !(connected && !!agentsRoot && interaction.clipboardItems.length > 0),
        onClick: () => {
          const ctx = crud.contextMenu.ctx;
          const targetDir = ctx?.kind === 'blank'
            ? ctx.dir
            : ctx
              ? (ctx.isDir ? ctx.path : ctx.parentDir)
              : null;
          if (targetDir) {
            void interaction.pasteInto(targetDir);
          }
        },
      },
    ];
  }, [agentsRoot, connected, crud.contextMenu.ctx, interaction]);

  const entryActions = useMemo(() => {
    const canRename = connected && !!crud.activeRenameTarget && interaction.selectionArray.length <= 1;
    const canDelete = connected && !!crud.activeDeleteTarget;
    const actions = [];

    if (crud.contextMenu.ctx?.kind === 'entry' && crud.contextMenu.ctx.isDir) {
      actions.push(...contextCreateActions);
    } else if (crud.contextMenu.ctx?.kind === 'entry') {
      actions.push({
        label: 'Paste',
        disabled: !(connected && !!agentsRoot && interaction.clipboardItems.length > 0),
        onClick: () => {
          const ctx = crud.contextMenu.ctx;
          const targetDir = ctx?.kind === 'entry'
            ? (ctx.isDir ? ctx.path : ctx.parentDir)
            : null;
          if (targetDir) {
            void interaction.pasteInto(targetDir);
          }
        },
      });
    }

    actions.push(
      {
        label: 'Cut',
        disabled: !connected,
        onClick: () => {
          crud.closeContextMenu();
          if (crud.contextMenu.ctx?.kind === 'entry') {
            void interaction.cutSelected({
              path: crud.contextMenu.ctx.path,
              isDir: crud.contextMenu.ctx.isDir,
            });
          }
        },
      },
      {
        label: 'Copy',
        disabled: !connected,
        onClick: () => {
          crud.closeContextMenu();
          if (crud.contextMenu.ctx?.kind === 'entry') {
            void interaction.copySelected({
              path: crud.contextMenu.ctx.path,
              isDir: crud.contextMenu.ctx.isDir,
            });
          }
        },
      },
      {
        label: 'Rename...',
        disabled: !canRename,
        onClick: crud.openRenameDialog,
      },
      {
        label: 'Delete',
        disabled: !canDelete,
        onClick: async () => {
          if (crud.contextMenu.ctx?.kind !== 'entry') {
            return;
          }
          const targets = await interaction.getActionItems({
            path: crud.contextMenu.ctx.path,
            isDir: crud.contextMenu.ctx.isDir,
          });
          crud.requestDeleteTargets(targets);
        },
      },
    );

    let splitIndex: number | undefined;
    if (crud.contextMenu.ctx?.kind === 'entry') {
      splitIndex = actions.length;
      actions.push({
        label: 'Copy Path',
        disabled: false,
        onClick: async () => {
          crud.closeContextMenu();
          if (crud.contextMenu.ctx?.kind !== 'entry') {
            return;
          }
          try {
            await writeClipboardText(crud.contextMenu.ctx.path);
            pushToast('Path copied');
          } catch {
            pushToast('Failed to copy path');
          }
        },
      });
    }

    return { actions, splitIndex };
  }, [agentsRoot, connected, contextCreateActions, crud, interaction, pushToast]);

  const menuActions = crud.contextMenu.ctx?.kind === 'blank' ? rootCreateActions : entryActions.actions;
  const menuSplitIndex = crud.contextMenu.ctx?.kind === 'entry' ? entryActions.splitIndex : undefined;
  const rootDropActive = useFileTreeSelectionStore((state) => (
    agentsRoot ? state.scopes[interaction.scopeId]?.dropTargetPath === agentsRoot : false
  ));
  const rootHeaderDropActive = Boolean(agentsRoot && (
    rootDropActive || treeImport.isRowDropTarget(agentsRoot)
  ));

  return (
    <div className="flex flex-col h-full text-prime-text">
      <div
        className={`ui-tabbar sidebar-root-header flex shrink-0 items-center gap-1 overflow-hidden px-2 text-secondary-text${rootHeaderDropActive ? ' file-tree-blank-drop-target' : ''}`}
        title={agentsRoot || undefined}
        onContextMenu={(event) => {
          event.preventDefault();
          interaction.focusTree();
          if (!agentsRoot) {
            return;
          }
          crud.openContextMenuAt(event.clientX, event.clientY, {
            kind: 'blank',
            dir: agentsRoot,
            depthForCreate: 0,
          });
        }}
        onDragOver={(event) => {
          if (agentsRoot) {
            interaction.handleRowDragOver(event, agentsRoot, agentsRoot, agentsRoot);
            if (!event.defaultPrevented) {
              treeImport.handleRowDragOver(event, agentsRoot, agentsRoot, agentsRoot);
            }
          }
        }}
        onDragLeave={(event) => {
          if (agentsRoot) {
            interaction.handleRowDragLeave(event, agentsRoot);
            treeImport.handleRowDragLeave(event, agentsRoot);
          }
        }}
        onDrop={(event) => {
          if (agentsRoot) {
            void interaction.handleRowDrop(event, agentsRoot, agentsRoot);
            if (!event.defaultPrevented) {
              treeImport.handleRowDrop(event, agentsRoot);
            }
          }
        }}
      >
        <div className="flex min-w-0 flex-1 items-center">
          <span className="ui-chrome-row-label truncate">agents</span>
        </div>
        <div className="sidebar-root-header-actions ml-auto flex shrink-0 items-center gap-0.5">
          <div ref={rootMenuTriggerRef} className="relative" onClick={(event) => event.stopPropagation()}>
            <IconButton
              onClick={() => setRootMenuOpen((prev) => !prev)}
              title="Add"
            >
              <PlusIcon className="w-3.5 h-3.5" />
            </IconButton>
            {rootMenuOpen ? (() => {
              const rect = rootMenuTriggerRef.current?.getBoundingClientRect();
              if (!rect) return null;
              const menuW = 220;
              const menuH = MENU_VERTICAL_PADDING + MENU_ITEM_HEIGHT * 2;
              const left = Math.min(rect.right + 4, window.innerWidth - menuW - 8);
              const top = Math.min(Math.max(8, rect.top), window.innerHeight - menuH - 8);
              return createPortal(
                <div ref={rootMenuRef} style={{ position: 'fixed', left, top, width: menuW, zIndex: 9999 }}>
                  <PopupMenu className="w-[220px]" onContextMenu={(event) => event.preventDefault()}>
                    <PopupMenuItem disabled={!connected || !agentsRoot} onClick={() => void startRootInlineCreate('file')}>
                      New File
                    </PopupMenuItem>
                    <PopupMenuItem disabled={!connected || !agentsRoot} onClick={() => void startRootInlineCreate('folder')}>
                      New Folder
                    </PopupMenuItem>
                  </PopupMenu>
                </div>,
                document.body,
              );
            })() : null}
          </div>
        </div>
      </div>

      <div
        ref={interaction.containerRef}
        tabIndex={0}
        data-file-tree-scope={interaction.scopeId}
        className={`flex-1 overflow-auto py-1 outline-none ${agentsRoot && treeImport.isBlankDropTarget(agentsRoot) ? 'file-tree-blank-drop-target' : ''}`}
        onKeyDown={(event) => void interaction.handleKeyDown(event)}
        onContextMenu={(event) => {
          event.preventDefault();
          interaction.focusTree();
          if (!agentsRoot) {
            return;
          }
          crud.openContextMenuAt(event.clientX, event.clientY, {
            kind: 'blank',
            dir: agentsRoot,
            depthForCreate: 0,
          });
        }}
        onDragOver={(event) => {
          if (agentsRoot) {
            interaction.handleBlankDragOver(event, agentsRoot);
            if (!event.defaultPrevented) {
              treeImport.handleBlankDragOver(event, agentsRoot);
            }
          }
        }}
        onDragLeave={(event) => {
          if (agentsRoot) {
            interaction.handleBlankDragLeave(event, agentsRoot);
            treeImport.handleBlankDragLeave(event, agentsRoot);
          }
        }}
        onDrop={(event) => {
          if (agentsRoot) {
            void interaction.handleBlankDrop(event, agentsRoot);
            if (!event.defaultPrevented) {
              treeImport.handleBlankDrop(event, agentsRoot);
            }
          }
        }}
      >
        {!connected ? null : loading ? (
          <div className="px-3 py-3 text-xs text-secondary-text">Loading...</div>
        ) : !agentsRoot ? (
          <div className="px-3 py-3 text-xs text-secondary-text">No agents directory available</div>
        ) : (
          <FileTreeChildren
            dir={agentsRoot}
            depth={1}
            inlineCreate={crud.inlineCreate}
            contextMenuTargetPath={crud.contextMenuTargetPath}
            onInlineCreateChange={(value) => {
              crud.setInlineCreate((prev) => (prev ? { ...prev, value, error: undefined } : prev));
            }}
            onInlineCreateCommit={crud.commitInlineCreate}
            onInlineCreateCancel={() => crud.setInlineCreate(null)}
            onContextMenu={crud.openContextMenuAt}
            isExternalRowDropTarget={treeImport.isRowDropTarget}
            isExternalBlankDropTarget={treeImport.isBlankDropTarget}
            onExternalEntryDragOver={treeImport.handleRowDragOver}
            onExternalEntryDragLeave={treeImport.handleRowDragLeave}
            onExternalEntryDrop={treeImport.handleRowDrop}
            onExternalBlankDragOver={treeImport.handleBlankDragOver}
            onExternalBlankDragLeave={treeImport.handleBlankDragLeave}
            onExternalBlankDrop={treeImport.handleBlankDrop}
            scopeId={interaction.scopeId}
            onEntryClick={interaction.handleEntryClick}
            onPrepareContextSelection={interaction.prepareContextSelection}
            onInternalDragStart={interaction.handleDragStart}
            onInternalDragEnd={interaction.handleDragEnd}
            onInternalEntryDragOver={interaction.handleRowDragOver}
            onInternalEntryDragLeave={interaction.handleRowDragLeave}
            onInternalEntryDrop={interaction.handleRowDrop}
            onInternalBlankDragOver={interaction.handleBlankDragOver}
            onInternalBlankDragLeave={interaction.handleBlankDragLeave}
            onInternalBlankDrop={interaction.handleBlankDrop}
          />
        )}
      </div>

      <FileTreeContextMenu
        open={crud.contextMenu.open}
        x={crud.contextMenu.x}
        y={crud.contextMenu.y}
        actions={menuActions}
        splitIndex={menuSplitIndex}
        onDismiss={crud.closeContextMenu}
      />

      <DeleteConfirmDialog
        open={!!crud.deleteDialog}
        title="Delete"
        message={
          crud.deleteDialog ? formatDeleteConfirmMessage(crud.deleteDialog) : ''
        }
        error={crud.deleteError}
        primaryLabel="Move to Trash"
        secondaryLabel="Delete Permanently"
        busy={crud.deleteBusy}
        onCancel={() => {
          if (crud.deleteBusy) return;
          crud.setDeleteDialog(null);
          crud.setDeleteError(null);
        }}
        onPrimary={() => void crud.confirmDelete(true)}
        onSecondary={() => void crud.confirmDelete(false)}
      />

      <RenameEntryDialog
        open={!!crud.renameDialog}
        path={crud.renameDialog?.path || null}
        value={crud.renameDialog?.value || ''}
        error={crud.renameDialog?.error || null}
        busy={crud.renameDialog?.busy}
        onChange={(value) =>
          crud.setRenameDialog((prev) => (prev ? { ...prev, value, error: undefined } : prev))
        }
        onCancel={() => {
          if (crud.renameDialog?.busy) return;
          crud.setRenameDialog(null);
        }}
        onSubmit={() => void crud.submitRename()}
      />

      <TreeImportConflictDialog
        open={!!treeImport.pendingConflict}
        targetDir={treeImport.pendingConflict?.prepared.targetDir || null}
        conflicts={treeImport.pendingConflict?.prepared.conflicts || []}
        busy={treeImport.pendingConflict?.busy}
        onCancel={() => void treeImport.cancelPendingConflict()}
        onConfirm={() => void treeImport.confirmPendingConflict()}
      />
    </div>
  );
}
