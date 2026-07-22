import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const markdownEditorSource = readFileSync(path.join(__dirname, 'MarkdownEditor.tsx'), 'utf8');
const markdownSetupSource = readFileSync(path.join(__dirname, 'codemirror/setup.ts'), 'utf8');
const markdownThemeSource = readFileSync(path.join(__dirname, 'codemirror/theme.ts'), 'utf8');
const frontmatterPanelSource = readFileSync(path.join(__dirname, 'FrontmatterPropertiesPanel.tsx'), 'utf8');
const frontmatterPluginSource = readFileSync(path.join(__dirname, 'codemirror/frontmatterPanelPlugin.ts'), 'utf8');
const stylesSource = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');

test('markdown text offset hover zone stays above the editor surface', () => {
  assert.match(markdownEditorSource, /className="relative z-0 h-full"/);
  assert.match(
    stylesSource,
    /\.op-md-text-offset-zone\s*\{[\s\S]*position:\s*absolute;[\s\S]*z-index:\s*2;[\s\S]*pointer-events:\s*auto;/m,
  );
});

test('markdown content width hover zone stays above the editor surface', () => {
  assert.match(markdownEditorSource, /className="op-md-content-width-zone"/);
  assert.match(
    stylesSource,
    /\.op-md-content-width-zone\s*\{[\s\S]*position:\s*absolute;[\s\S]*right:\s*0;[\s\S]*z-index:\s*2;[\s\S]*pointer-events:\s*auto;/m,
  );
});

test('markdown text offset hover stays hidden while text entry has focus', () => {
  assert.match(markdownEditorSource, /const TEXT_OFFSET_HOVER_DELAY_MS = 1000;/);
  assert.match(markdownEditorSource, /function isTextEntryFocusedElement\(element: Element \| null\): boolean/);
  assert.match(markdownEditorSource, /tagName === 'INPUT' \|\| tagName === 'TEXTAREA'/);
  assert.match(markdownEditorSource, /element\.closest\?\.\('\.cm-content'\)/);
  assert.match(markdownEditorSource, /if \(isDocumentTextEntryFocused\(\)\) \{\s*hideTextOffsetHover\(\);\s*return;\s*\}/m);
  assert.match(markdownEditorSource, /document\.addEventListener\('focusin', hideIfTextEntryFocused, true\);/);
  assert.match(markdownEditorSource, /document\.addEventListener\('keydown', hideIfTextEntryFocused, true\);/);
  assert.match(markdownEditorSource, /document\.addEventListener\('input', hideIfTextEntryFocused, true\);/);
  assert.match(markdownEditorSource, /visible=\{textOffsetHovered \|\| textOffsetDragging\}\s+hitTargetEnabled=\{textOffsetHovered \|\| textOffsetDragging\}/m);
  assert.doesNotMatch(markdownEditorSource, /visible=\{textOffsetHovered \|\| textOffsetDragging\}\s+hitTargetEnabled\s+activeColor/m);
  assert.match(markdownEditorSource, /if \(!textOffsetHoveredStateRef\.current && !textOffsetDraggingStateRef\.current\) \{\s*return;\s*\}\s*event\.preventDefault\(\);/m);
});

test('markdown content width hover follows text offset reveal semantics', () => {
  assert.match(markdownEditorSource, /const ensureContentWidthHoverTimer = useCallback\(\(\) => \{/);
  assert.match(markdownEditorSource, /if \(isDocumentTextEntryFocused\(\)\) \{\s*hideContentWidthHover\(\);\s*return;\s*\}/m);
  assert.match(markdownEditorSource, /contentWidthHoverDeadlineRef\.current = performance\.now\(\) \+ TEXT_OFFSET_HOVER_DELAY_MS;/);
  assert.match(markdownEditorSource, /visible=\{contentWidthHovered \|\| contentWidthDragging\}\s+hitTargetEnabled=\{contentWidthHovered \|\| contentWidthDragging\}/m);
});

test('markdown text offset zone click arms reveal after text editing focus', () => {
  assert.match(markdownEditorSource, /function blurDocumentTextEntryFocus\(\): void/);
  assert.match(markdownEditorSource, /active\.blur\(\);/);
  assert.match(markdownEditorSource, /const armTextOffsetHoverReveal = useCallback\(\(\) => \{/);
  assert.match(markdownEditorSource, /blurDocumentTextEntryFocus\(\);\s*textOffsetHoverDeadlineRef\.current = performance\.now\(\) \+ TEXT_OFFSET_HOVER_DELAY_MS;\s*ensureTextOffsetHoverTimer\(\);/m);
  assert.match(markdownEditorSource, /onPointerDown=\{\(event\) => \{\s*if \(event\.button !== 0\) \{\s*return;\s*\}\s*armTextOffsetHoverReveal\(\);/m);
});

test('markdown content width drag previews and persists ui setting', () => {
  assert.match(markdownEditorSource, /window\.electronAPI\?\.settings\?\.previewMarkdownContentWidth\?\.\(nextValue\);/);
  assert.match(markdownEditorSource, /const finalPersistedWidth = normalizeMarkdownContentWidth\(drag\.currentWidth\);/);
  assert.match(markdownEditorSource, /markdownContentWidth: finalPersistedWidth,/);
});

test('markdown layout width variables are scoped to each editor instance', () => {
  assert.match(markdownEditorSource, /const markdownEditorStyle = \{/);
  assert.match(markdownEditorSource, /'--op-md-content-padding-left': `\$\{Math\.max\(0, textOffset\)\}px`,/);
  assert.match(markdownEditorSource, /'--op-md-content-max-width': `\$\{Math\.max\(0, contentWidth\)\}px`,/);
  assert.match(markdownEditorSource, /style=\{markdownEditorStyle\}/);
  assert.doesNotMatch(markdownEditorSource, /setMarkdownContentWidthCssVar/);
  assert.doesNotMatch(markdownEditorSource, /setMarkdownTextOffsetCssVar/);
  assert.doesNotMatch(markdownEditorSource, /setMarkdownEffectiveColumnOffsetCssVar/);
});

test('markdown editor shows scroll jump controls after scroll activity', () => {
  assert.match(markdownEditorSource, /const SCROLL_JUMP_CONTROL_VISIBLE_MS = 1500;/);
  assert.match(markdownEditorSource, /const SCROLL_JUMP_CONTROL_SCREEN_THRESHOLD = 2;/);
  assert.match(markdownEditorSource, /const \[scrollJumpControlsVisible, setScrollJumpControlsVisible\] = useState\(false\);/);
  assert.match(markdownEditorSource, /const threshold = Math\.max\(1, scrollEl\.clientHeight \* SCROLL_JUMP_CONTROL_SCREEN_THRESHOLD\);/);
  assert.match(markdownEditorSource, /Math\.abs\(currentTop - session\.baselineTop\) < threshold/);
  assert.match(markdownEditorSource, /view\.scrollDOM\.addEventListener\('scroll', onScrollJumpControls, \{ passive: true \}\);/);
  assert.match(markdownEditorSource, /title="Scroll to top"/);
  assert.match(markdownEditorSource, /title="Scroll to bottom"/);
  assert.match(stylesSource, /\.op-md-scroll-jump-controls\s*\{/m);
});

test('chat markdown external updates follow bottom only while attached', () => {
  assert.match(markdownEditorSource, /isChatMarkdownScrollNearBottom/);
  assert.match(markdownEditorSource, /shouldFollowChatMarkdownUpdate/);
  assert.match(markdownEditorSource, /const chatScrollDetachedRef = useRef\(false\);/);
  assert.match(markdownEditorSource, /activeChatPath \? state\.shouldScrollChatToBottom\(activeChatPath\) : false/);
  assert.match(markdownEditorSource, /const shouldFollowBottom = shouldFollowActiveChatBottom\(\);/);
  assert.match(markdownEditorSource, /consumeChatScrollToBottom\(activeChatPath\);/);
  assert.doesNotMatch(markdownEditorSource, /const shouldFollowBottom = shouldFollowStreaming;/);
});

test('chat markdown path retarget can reuse same tab view state', () => {
  assert.match(markdownEditorSource, /const activeTabViewStateKey = activeTab\?\.id \? `tab:\$\{activeTab\.id\}` : '';/);
  assert.match(markdownEditorSource, /loadViewState\(activeIdentity\)\s*\|\|\s*\(activeTabViewStateKey \? loadViewState\(activeTabViewStateKey\) : null\)/m);
  assert.match(markdownEditorSource, /saveViewState\(activeTabViewStateKey, nextViewState\);/);
});

test('markdown editor prefers emoji presentation for symbolic emoji', () => {
  assert.match(stylesSource, /--op-markdown-document-font-family:/);
  assert.match(markdownThemeSource, /var\(--op-markdown-document-font-family\)/);
  assert.match(markdownThemeSource, /font-variant-emoji': 'emoji'/);
});

test('markdown rendered spaces stay fixed across heading and body sizes', () => {
  assert.match(stylesSource, /--op-md-rendered-space-width:\s*4px;/);
  assert.match(
    stylesSource,
    /\.op-markdown-editor \.cm-line\s*\{[^}]*word-spacing:\s*calc\(var\(--op-md-rendered-space-width\) - 0\.25em\);/m,
  );
  assert.match(
    stylesSource,
    /\.op-markdown-editor \.cm-md-heading\s*\{[^}]*word-spacing:\s*calc\(var\(--op-md-rendered-space-width\) - 0\.25em\);/m,
  );
  assert.match(
    stylesSource,
    /\.op-markdown-editor \.cm-md-code,[\s\S]*?\.op-markdown-editor \.cm-md-link,[\s\S]*?\.op-markdown-editor \.cm-md-frontmatter-link[\s\S]*?\{[^}]*word-spacing:\s*normal;/m,
  );
});

test('markdown table widgets use markdown document emoji font', () => {
  assert.match(stylesSource, /\.cm-md-table-block\s*\{[^}]*font-family:\s*var\(--op-markdown-document-font-family\);/m);
  assert.match(stylesSource, /\.cm-md-table-block\s*\{[^}]*font-variant-emoji:\s*emoji;/m);
});

test('markdown editor keeps scrollable bottom clearance for body text and activity panel overlay', () => {
  assert.match(stylesSource, /--op-editor-content-bottom-clearance:\s*calc\(/);
  assert.match(
    stylesSource,
    /\.op-markdown-editor \.cm-content,[\s\S]*?padding-bottom:\s*max\([\s\S]*?var\(--op-editor-content-bottom-clearance\),[\s\S]*?var\(--op-editor-bottom-safe-area\)/m,
  );
  assert.match(stylesSource, /--op-activity-panel-content-bottom-clearance:/);
  assert.match(
    stylesSource,
    /\.op-activity-panel-body\s*\{[\s\S]*?padding-bottom:\s*var\(--op-activity-panel-content-bottom-clearance\);/m,
  );
});

test('markdown editor mounts obsidian-style frontmatter properties panel', () => {
  assert.match(markdownSetupSource, /frontmatterPanelOptionsExtension\(\{ readOnly, exportMode \}\)/);
  assert.match(markdownSetupSource, /\.\.\.\(!exportMode \? \[frontmatterPanelPlugin\(\)\] : \[\]\)/);
  assert.match(markdownSetupSource, /reconfigureFrontmatterPanelOptions/);
  assert.match(markdownSetupSource, /refreshFrontmatterPanel\(view\)/);
  assert.match(stylesSource, /\.op-md-frontmatter-properties\s*\{/);
  assert.match(stylesSource, /\.cm-md-frontmatter-collapsed\s*\{/);
  assert.match(stylesSource, /\.op-markdown-editor \.cm-scroller\s*\{[^}]*flex-direction:\s*column;/m);
  assert.match(
    stylesSource,
    /\.op-markdown-editor \.cm-content\s*\{[^}]*width:\s*min\(100%, var\(--op-md-content-max-width\)\);[^}]*max-width:\s*var\(--op-md-content-max-width\);/m,
  );
  assert.match(frontmatterPanelSource, /aria-label="Show frontmatter source"/);
  assert.match(frontmatterPanelSource, /function ObjectPropertyEditor\(/);
  assert.match(frontmatterPanelSource, /function ObjectListPropertyEditor\(/);
  assert.match(frontmatterPanelSource, /function RunPropertyEditor\(/);
  assert.match(frontmatterPanelSource, /function RunEndpointModeSelect\(/);
  assert.match(frontmatterPanelSource, /<SelectMenu/);
  assert.match(frontmatterPanelSource, /inferRunEndpointMode,/);
  assert.match(frontmatterPanelSource, /switchRunEndpointMode,/);
  assert.doesNotMatch(frontmatterPanelSource, /RUN_EMPTY_FIELD_ORDER/);
  assert.match(frontmatterPanelSource, /delete nextRun\.url;/);
  assert.match(frontmatterPanelSource, /delete nextRun\.header;/);
  assert.match(frontmatterPanelSource, /delete nextRun\.command;/);
  assert.match(frontmatterPanelSource, /function isRunDaemonPath\(path: PropertyPath\): boolean \{/);
  assert.match(frontmatterPanelSource, /function RunHeaderEditor\(/);
  assert.match(frontmatterPanelSource, /label="Add header"/);
  assert.match(frontmatterPanelSource, /label="Add daemon"/);
  assert.match(frontmatterPanelSource, /onClick=\{\(\) => onPatch\(true\)\}/);
  assert.match(frontmatterPanelSource, /function BooleanPropertyValue\(/);
  assert.match(stylesSource, /\.op-md-frontmatter-property-row-nested\s*\{/);
  assert.doesNotMatch(frontmatterPanelSource, />Open</);
  assert.doesNotMatch(frontmatterPanelSource, /Edit in source/);
  assert.doesNotMatch(frontmatterPanelSource, /RUN_LIFECYCLE_OPTIONS/);
  assert.doesNotMatch(frontmatterPanelSource, /RUN_SCHEDULE_FIELD_ORDER/);
  assert.doesNotMatch(frontmatterPanelSource, /RUN_AUTH/);
  assert.match(frontmatterPluginSource, /document\.addEventListener\('mousedown', this\.handleDocumentMouseDown, true\)/);
  assert.match(frontmatterPluginSource, /this\.view\.posAtCoords\(\{ x: event\.clientX, y: event\.clientY \}\)/);
  assert.doesNotMatch(markdownSetupSource, /agentConfigFrontmatterGuard/);
  assert.doesNotMatch(markdownSetupSource, /protectFrontmatter/);
  assert.doesNotMatch(markdownEditorSource, /protectAgentConfigFrontmatter/);
});
