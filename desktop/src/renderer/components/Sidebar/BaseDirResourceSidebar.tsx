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
import {
  getBaseDirResourceEmptyMessage,
  getBaseDirResourceMissingMessage,
  joinBaseDirResourcePath,
} from './baseDirResourceSidebarUtils';

const MENU_VERTICAL_PADDING = 8;
const MENU_ITEM_HEIGHT = 34;

type BaseDirResourceSidebarProps = {
  rootLeafName: string;
  rootLabel: string;
};

export function BaseDirResourceSidebar({
  rootLeafName,
  rootLabel,
}: BaseDirResourceSidebarProps) {
  const connectionState = useAppStore((state) => state.connectionState);
  const storedBaseDir = useAppStore((state) => state.baseDir);
  const ensureDerivedDirs = useAppStore((state) => state.ensureDerivedDirs);
  const ensureDirectory = useAppStore((state) => state.ensureDirectory);
  const statPath = useAppStore((state) => state.statPath);
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
  const initialRoot = storedBaseDir ? joinBaseDirResourcePath(storedBaseDir, rootLeafName) : null;
  const [resourceRoot, setResourceRoot] = useState<string | null>(initialRoot);
  const [loading, setLoading] = useState(!initialRoot);
  const [rootExists, setRootExists] = useState(Boolean(initialRoot));
  const [rootMenuOpen, setRootMenuOpen] = useState(false);
  const rootMenuRef = useRef<HTMLDivElement | null>(null);
  const rootMenuTriggerRef = useRef<HTMLDivElement | null>(null);

  const refreshNodes = async () => {
    await refreshAgentNodes({ force: true });
  };

  const crud = useFileTreeCrudController({
    connected,
    expandedDirs,
    toggleDir,
    createFile: async (path) => {
      const result = await createFile(path);
      if (result.success) {
        await refreshNodes();
      }
      return result;
    },
    createFolder: async (path) => {
      const result = await createFolder(path);
      if (result.success) {
        await refreshNodes();
      }
      return result;
    },
    deleteEntry: async (path, isDir, options) => {
      const result = await deleteEntry(path, isDir, options);
      if (result.success) {
        await refreshNodes();
      }
      return result;
    },
    renameEntry: async (oldPath, newPath) => {
      const result = await renameEntry(oldPath, newPath);
      if (result.success) {
        await refreshNodes();
      }
      return result;
    },
    openFile,
  });

  const interaction = useFileTreeInteractionController({
    enabled: connected && !!resourceRoot,
    ensureExpanded: (dir) => {
      if (!expandedDirs.has(dir)) {
        toggleDir(dir);
      }
    },
    getDefaultPasteTargetDir: () => resourceRoot,
    selectionResetKey: resourceRoot || '',
    onTransferCommitted: async () => {
      await refreshNodes();
    },
  });

  const treeImport = useTreeImportDropController({
    enabled: connected && !!resourceRoot,
    ensureExpanded: (dir) => {
      if (!expandedDirs.has(dir)) {
        toggleDir(dir);
      }
    },
    onImportCommitted: async (targetDir) => {
      await loadDirectory(targetDir);
      await refreshNodes();
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
      setResourceRoot(null);
      setRootExists(false);
      setRootMenuOpen(false);
      setLoading(false);
      return;
    }

    let active = true;
    setLoading(true);

    const loadRoot = async () => {
      try {
        const derived = await ensureDerivedDirs({ force: true });
        if (!active) {
          return;
        }
        const baseDir = (derived?.baseDir || '').trim();
        if (!baseDir) {
          setResourceRoot(null);
          setRootExists(false);
          return;
        }

        const nextRoot = joinBaseDirResourcePath(baseDir, rootLeafName);
        let stat = await statPath(nextRoot);
        if (!active) {
          return;
        }

        if (stat.error || stat.isDir !== true) {
          await ensureDirectory(nextRoot);
          if (!active) {
            return;
          }
          stat = await statPath(nextRoot);
          if (!active) {
            return;
          }
        }

        setResourceRoot(nextRoot);
        setRootExists(!stat.error && stat.isDir === true);
      } catch (error) {
        console.error(`Failed to load ${rootLeafName} root:`, error);
        if (active) {
          setResourceRoot(null);
          setRootExists(false);
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
  }, [connected, ensureDerivedDirs, ensureDirectory, rootLeafName, statPath, storedBaseDir]);

  const ensureRootReady = async (): Promise<string | null> => {
    const root = (resourceRoot || '').trim();
    if (!connected || !root) {
      return null;
    }
    if (!rootExists) {
      await ensureDirectory(root);
      setRootExists(true);
    }
    return root;
  };

  const startRootInlineCreate = async (kind: 'file' | 'folder') => {
    setRootMenuOpen(false);
    const root = await ensureRootReady();
    if (!root) {
      return;
    }
    crud.startInlineCreateAtDir(root, kind, 0);
  };

  const startContextInlineCreate = async (kind: 'file' | 'folder') => {
    const ctx = crud.contextMenu.ctx;
    if (ctx?.kind === 'blank' && ctx.dir === resourceRoot && !rootExists) {
      await startRootInlineCreate(kind);
      return;
    }
    crud.startInlineCreate(kind);
  };

  const contextDirTarget = crud.contextMenu.ctx?.kind === 'blank'
    ? crud.contextMenu.ctx.dir
    : crud.contextMenu.ctx
      ? (crud.contextMenu.ctx.isDir ? crud.contextMenu.ctx.path : crud.contextMenu.ctx.parentDir)
      : null;

  const contextCreateActions = useMemo(() => {
    const canCreate = connected && !!resourceRoot;
    return [
      {
        label: 'New File...',
        disabled: !canCreate,
        onClick: () => {
          void startContextInlineCreate('file');
        },
      },
      {
        label: 'New Folder...',
        disabled: !canCreate,
        onClick: () => {
          void startContextInlineCreate('folder');
        },
      },
      {
        label: 'Paste',
        disabled: !(connected && !!contextDirTarget && interaction.clipboardItems.length > 0),
        onClick: () => {
          if (contextDirTarget) {
            void interaction.pasteInto(contextDirTarget);
          }
        },
      },
    ];
  }, [connected, contextDirTarget, interaction, resourceRoot, rootExists]);

  const entryActions = useMemo(() => {
    const canRename = connected && !!crud.activeRenameTarget && interaction.selectionArray.length <= 1;
    const canDelete = connected && !!crud.activeDeleteTarget;
    const actions = [];

    if (crud.contextMenu.ctx?.kind === 'entry' && crud.contextMenu.ctx.isDir) {
      actions.push(...contextCreateActions);
    } else if (crud.contextMenu.ctx?.kind === 'entry') {
      actions.push({
        label: 'Paste',
        disabled: !(connected && !!contextDirTarget && interaction.clipboardItems.length > 0),
        onClick: () => {
          if (contextDirTarget) {
            void interaction.pasteInto(contextDirTarget);
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
  }, [connected, contextCreateActions, contextDirTarget, crud, interaction, pushToast]);

  const menuActions = crud.contextMenu.ctx?.kind === 'blank' ? contextCreateActions : entryActions.actions;
  const menuSplitIndex = crud.contextMenu.ctx?.kind === 'entry' ? entryActions.splitIndex : undefined;
  const emptyMessage = getBaseDirResourceEmptyMessage(rootLabel);
  const missingMessage = getBaseDirResourceMissingMessage(rootLabel);
  const rootDropActive = useFileTreeSelectionStore((state) => (
    resourceRoot ? state.scopes[interaction.scopeId]?.dropTargetPath === resourceRoot : false
  ));
  const rootHeaderDropActive = Boolean(resourceRoot && (
    rootDropActive || treeImport.isRowDropTarget(resourceRoot)
  ));

  return (
    <div className="flex flex-col h-full text-prime-text">
      <div
        className={`ui-tabbar sidebar-root-header flex shrink-0 items-center gap-1 overflow-hidden px-2 text-secondary-text${rootHeaderDropActive ? ' file-tree-blank-drop-target' : ''}`}
        title={resourceRoot || undefined}
        onContextMenu={(event) => {
          event.preventDefault();
          interaction.focusTree();
          if (!resourceRoot) {
            return;
          }
          crud.openContextMenuAt(event.clientX, event.clientY, {
            kind: 'blank',
            dir: resourceRoot,
            depthForCreate: 0,
          });
        }}
        onDragOver={(event) => {
          if (resourceRoot) {
            interaction.handleRowDragOver(event, resourceRoot, resourceRoot, resourceRoot);
            if (!event.defaultPrevented) {
              treeImport.handleRowDragOver(event, resourceRoot, resourceRoot, resourceRoot);
            }
          }
        }}
        onDragLeave={(event) => {
          if (resourceRoot) {
            interaction.handleRowDragLeave(event, resourceRoot);
            treeImport.handleRowDragLeave(event, resourceRoot);
          }
        }}
        onDrop={(event) => {
          if (resourceRoot) {
            void interaction.handleRowDrop(event, resourceRoot, resourceRoot);
            if (!event.defaultPrevented) {
              treeImport.handleRowDrop(event, resourceRoot);
            }
          }
        }}
      >
        <div className="flex min-w-0 flex-1 items-center">
          <span className="ui-chrome-row-label truncate">{rootLabel}</span>
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
              if (!rect) {
                return null;
              }
              const menuWidth = 220;
              const menuHeight = MENU_VERTICAL_PADDING + MENU_ITEM_HEIGHT * 2;
              const left = Math.min(rect.right + 4, window.innerWidth - menuWidth - 8);
              const top = Math.min(Math.max(8, rect.top), window.innerHeight - menuHeight - 8);
              return createPortal(
                <div ref={rootMenuRef} style={{ position: 'fixed', left, top, width: menuWidth, zIndex: 9999 }}>
                  <PopupMenu className="w-[220px]" onContextMenu={(event) => event.preventDefault()}>
                    <PopupMenuItem disabled={!connected || !resourceRoot} onClick={() => void startRootInlineCreate('file')}>
                      New File
                    </PopupMenuItem>
                    <PopupMenuItem disabled={!connected || !resourceRoot} onClick={() => void startRootInlineCreate('folder')}>
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
        className={`flex-1 overflow-auto py-1 outline-none ${resourceRoot && treeImport.isBlankDropTarget(resourceRoot) ? 'file-tree-blank-drop-target' : ''}`}
        onKeyDown={(event) => void interaction.handleKeyDown(event)}
        onContextMenu={(event) => {
          event.preventDefault();
          interaction.focusTree();
          if (!resourceRoot) {
            return;
          }
          crud.openContextMenuAt(event.clientX, event.clientY, {
            kind: 'blank',
            dir: resourceRoot,
            depthForCreate: 0,
          });
        }}
        onDragOver={(event) => {
          if (resourceRoot) {
            interaction.handleBlankDragOver(event, resourceRoot);
            if (!event.defaultPrevented) {
              treeImport.handleBlankDragOver(event, resourceRoot);
            }
          }
        }}
        onDragLeave={(event) => {
          if (resourceRoot) {
            interaction.handleBlankDragLeave(event, resourceRoot);
            treeImport.handleBlankDragLeave(event, resourceRoot);
          }
        }}
        onDrop={(event) => {
          if (resourceRoot) {
            void interaction.handleBlankDrop(event, resourceRoot);
            if (!event.defaultPrevented) {
              treeImport.handleBlankDrop(event, resourceRoot);
            }
          }
        }}
      >
        {!connected ? null : loading ? (
          <div className="px-3 py-3 text-xs text-secondary-text">Loading...</div>
        ) : !resourceRoot ? (
          <div className="px-3 py-3 text-xs text-secondary-text">{missingMessage}</div>
        ) : rootExists ? (
          <FileTreeChildren
            dir={resourceRoot}
            depth={1}
            inlineCreate={crud.inlineCreate}
            onInlineCreateChange={(value) => {
              crud.setInlineCreate((prev) => (prev ? { ...prev, value, error: undefined } : prev));
            }}
            onInlineCreateCommit={crud.commitInlineCreate}
            onInlineCreateCancel={() => crud.setInlineCreate(null)}
            onContextMenu={crud.openContextMenuAt}
            contextMenuTargetPath={crud.contextMenuTargetPath}
            showAgentLabels={false}
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
        ) : (
          <div className="px-3 py-3 text-xs text-secondary-text">{emptyMessage}</div>
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
          if (crud.deleteBusy) {
            return;
          }
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
          if (crud.renameDialog?.busy) {
            return;
          }
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
