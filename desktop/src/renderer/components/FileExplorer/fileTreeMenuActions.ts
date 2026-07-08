export type FileTreeEntryActionKey =
  | 'new-file'
  | 'new-folder'
  | 'rename'
  | 'delete'
  | 'cut'
  | 'copy'
  | 'paste'
  | 'add-agent'
  | 'copy-path'
  | 'reveal-in-finder';

export type FileTreeEntryMenuDescriptor = {
  key: FileTreeEntryActionKey;
  label: string;
  disabled: boolean;
};

type BuildFileTreeEntryMenuOptions = {
  isDir: boolean;
  canRename: boolean;
  canDelete: boolean;
  canCut: boolean;
  canCopy: boolean;
  canPaste: boolean;
  canAddAgent: boolean;
  canCopyPath: boolean;
  canRevealInFinder: boolean;
};

export function buildFileTreeEntryMenu({
  isDir,
  canRename,
  canDelete,
  canCut,
  canCopy,
  canPaste,
  canAddAgent,
  canCopyPath,
  canRevealInFinder,
}: BuildFileTreeEntryMenuOptions): {
  actions: FileTreeEntryMenuDescriptor[];
  splitIndex: number | undefined;
} {
  const actions: FileTreeEntryMenuDescriptor[] = [];

  if (isDir) {
    actions.push(
      { key: 'new-file', label: 'New File...', disabled: false },
      { key: 'new-folder', label: 'New Folder...', disabled: false },
    );
  }

  actions.push(
    { key: 'cut', label: 'Cut', disabled: !canCut },
    { key: 'copy', label: 'Copy', disabled: !canCopy },
    { key: 'paste', label: 'Paste', disabled: !canPaste },
    { key: 'rename', label: 'Rename...', disabled: !canRename },
    { key: 'delete', label: 'Delete', disabled: !canDelete },
  );

  if (isDir) {
    actions.push({
      key: 'add-agent',
      label: 'Add Agent...',
      disabled: !canAddAgent,
    });
  }

  const splitIndex = actions.length;

  actions.push({
    key: 'copy-path',
    label: 'Copy Path',
    disabled: !canCopyPath,
  });

  if (canRevealInFinder) {
    actions.push({
      key: 'reveal-in-finder',
      label: 'Reveal in Finder',
      disabled: false,
    });
  }

  return {
    actions,
    splitIndex: splitIndex < actions.length ? splitIndex : undefined,
  };
}
