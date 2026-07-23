import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './Sidebar.tsx'),
  'utf8',
);

test('Sidebar routes the search icon into the temporary search view', () => {
  assert.match(source, /setView\('search'\)/);
});

test('Sidebar renders SearchSidebar for the temporary search tab', () => {
  assert.match(source, /<SearchSidebar \/>/);
  assert.match(source, /view === 'search'/);
});

test('Sidebar routes the Marketplace rail item to the existing editor tab command', () => {
  assert.match(source, /data-sidebar-rail-item=\{item\.key\}/);
  assert.match(source, /itemKey === 'marketplace'/);
  assert.match(source, /setView\('workspace'\);[\s\S]*openMarketplaceTab\(\)/);
});

test('Sidebar OpenBrain rail shows the local list without opening an editor tab', () => {
  assert.match(source, /view === 'openbrain'[\s\S]*<OpenBrainSidebar onOpenWorkspace=\{handleOpenBrainWorkspace\} onCreateSource=\{onCreateOpenBrainSource\} onBindSource=\{onBindOpenBrainSource\} \/>/);
  assert.doesNotMatch(source, /openOpenBrainTab\(\)/);
});

test('Sidebar switches back to the folder view after opening an OpenBrain workspace', () => {
  assert.match(source, /const handleOpenBrainWorkspace = async \(workspace: LocalOpenBrainWorkspace\) => \{/);
  assert.match(source, /await onOpenLocalNewTab\(workspace\.path\);[\s\S]*setView\('workspace'\);/);
});
