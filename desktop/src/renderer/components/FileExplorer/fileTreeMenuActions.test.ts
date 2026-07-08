import assert from 'node:assert/strict';
import test from 'node:test';

const { buildFileTreeEntryMenu } =
  // @ts-ignore Node strip-types test runner requires explicit .ts extensions here.
  await import('./fileTreeMenuActions.ts');

test('builds local mac file entry actions with transfer actions', () => {
  const result = buildFileTreeEntryMenu({
    isDir: false,
    canRename: true,
    canDelete: true,
    canCut: true,
    canCopy: true,
    canPaste: true,
    canAddAgent: false,
    canCopyPath: true,
    canRevealInFinder: true,
  });

  assert.deepEqual(
    result.actions.map((action) => action.label),
    ['Cut', 'Copy', 'Paste', 'Rename...', 'Delete', 'Copy Path', 'Reveal in Finder'],
  );
  assert.equal(result.splitIndex, 5);
});

test('builds local mac directory entry actions with directory-only items', () => {
  const result = buildFileTreeEntryMenu({
    isDir: true,
    canRename: true,
    canDelete: true,
    canCut: true,
    canCopy: true,
    canPaste: true,
    canAddAgent: true,
    canCopyPath: true,
    canRevealInFinder: true,
  });

  assert.deepEqual(
    result.actions.map((action) => action.label),
    ['New File...', 'New Folder...', 'Cut', 'Copy', 'Paste', 'Rename...', 'Delete', 'Add Agent...', 'Copy Path', 'Reveal in Finder'],
  );
  assert.equal(result.splitIndex, 8);
});

test('keeps copy path and hides finder reveal for remote mac entries', () => {
  const result = buildFileTreeEntryMenu({
    isDir: false,
    canRename: true,
    canDelete: true,
    canCut: true,
    canCopy: true,
    canPaste: false,
    canAddAgent: false,
    canCopyPath: true,
    canRevealInFinder: false,
  });

  assert.deepEqual(
    result.actions.map((action) => action.label),
    ['Cut', 'Copy', 'Paste', 'Rename...', 'Delete', 'Copy Path'],
  );
  assert.equal(result.actions.some((action) => action.label === 'Reveal in Finder'), false);
  assert.equal(result.splitIndex, 5);
});

test('keeps copy path and hides finder reveal for non-mac local entries', () => {
  const result = buildFileTreeEntryMenu({
    isDir: true,
    canRename: true,
    canDelete: true,
    canCut: true,
    canCopy: true,
    canPaste: false,
    canAddAgent: true,
    canCopyPath: true,
    canRevealInFinder: false,
  });

  assert.deepEqual(
    result.actions.map((action) => action.label),
    ['New File...', 'New Folder...', 'Cut', 'Copy', 'Paste', 'Rename...', 'Delete', 'Add Agent...', 'Copy Path'],
  );
  assert.equal(result.actions.some((action) => action.label === 'Reveal in Finder'), false);
  assert.equal(result.splitIndex, 8);
});
