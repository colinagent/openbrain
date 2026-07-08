import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'FileTreeItem.tsx'), 'utf8');

test('file tree delegates markdown routing to openFile instead of path-based chat branching', () => {
  assert.doesNotMatch(source, /openThreadTab/);
  assert.doesNotMatch(source, /setTargetChatPath/);
  assert.doesNotMatch(source, /\/\\.agent\/chat\//);
  assert.match(source, /if \(openableExtensions\.length === 0\) \{\s*openFile\(fullPath\);\s*return;\s*\}/s);
});

test('directory agent pill exposes a stable onboarding target', () => {
  assert.match(source, /data-onboarding-target="workspace-dir-agent-pill"/);
});

test('file tree item renders transient tree selection separately from the current file', () => {
  assert.match(source, /const isMultiSelected = useFileTreeSelectionStore/);
  assert.match(source, /state\.scopes\[scopeId\]\?\.selection\.has\(fullPath\) === true/);
  assert.match(source, /selected=\{isCurrentFile\}/);
  assert.match(source, /multiSelected=\{isMultiSelected\}/);
});

test('.agent directories render as ordinary directory labels with a hover-gated inline agent pill', () => {
  assert.doesNotMatch(source, /FileTreeAgentDirIcon/);
  assert.doesNotMatch(source, /AgentBotIcon/);
  assert.doesNotMatch(source, /file-tree-agent-badge/);
  assert.match(source, /<ChevronRightIcon className="w-3 h-3" \/>/);
  assert.match(source, /<span className="truncate">\{entry\.name\}<\/span>/);
  assert.match(source, /agentLabelPlacement === 'inline'/);
  assert.match(source, /file-tree-agent-inline-pill/);
});

test('file tree agent pill uses flat ui-capsule-pill without static glass', () => {
  const stylesSource = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');
  const source = readFileSync(path.join(__dirname, 'FileTreeItem.tsx'), 'utf8');
  assert.match(source, /ui-pill-btn-secondary file-tree-agent-pill/);
  assert.doesNotMatch(source, /OP_SG_CAPSULE/);
  assert.match(stylesSource, /\.file-tree-agent-pill\s*\{[^}]*box-shadow:\s*none;/m);
  assert.match(stylesSource, /\.file-tree-agent-pill\s*\{[^}]*padding:\s*1px 7px;/m);
  assert.match(stylesSource, /\.file-tree-agent-pill:hover[^}]*color:\s*var\(--color-highlight\)/m);
  assert.doesNotMatch(stylesSource, /\.file-tree-agent-pill:hover[^}]*background-color:/m);
});
