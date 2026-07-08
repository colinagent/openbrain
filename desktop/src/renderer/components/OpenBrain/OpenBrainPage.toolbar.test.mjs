import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './OpenBrainPage.tsx'),
  'utf8',
);
const styles = readFileSync(
  path.resolve(import.meta.dirname, '../../styles/index.css'),
  'utf8',
);

test('OpenBrain onboarding title uses prime-text; only action uses glass', () => {
  assert.match(source, /openbrain-onboarding-title text-lg font-bold leading-6 tracking-\[-0\.045em\]/);
  assert.doesNotMatch(source, /openbrain-onboarding-title \$\{OPENBRAIN_GRAPH_CAPSULE\}/);
  assert.match(source, /openbrain-onboarding-action no-drag \$\{OPENBRAIN_GRAPH_CAPSULE\}/);
  assert.doesNotMatch(source, /openbrain-onboarding-copy \$\{OPENBRAIN_GRAPH_CAPSULE\}/);
});

test('OpenBrain onboarding title and action default to prime-text; action hover stays brand green', () => {
  const titleBlock = styles.match(/\.openbrain-onboarding-title\s*\{[^}]*\}/)?.[0] ?? '';
  assert.match(titleBlock, /var\(--color-prime-text\)/);
  assert.doesNotMatch(titleBlock, /box-shadow/);

  const actionBlock = styles.match(
    /\.openbrain-onboarding-action\.openbrain-graph-capsule\.op-sg-capsule\s*\{[^}]*\}/,
  )?.[0] ?? '';
  assert.match(actionBlock, /var\(--color-prime-text\)/);

  const actionHoverBlock = styles.match(
    /\.openbrain-onboarding-action\.openbrain-graph-capsule\.op-sg-capsule:hover:not\(:disabled\)\s*\{[^}]*\}/,
  )?.[0] ?? '';
  assert.match(actionHoverBlock, /#2f8f6b/);
});

test('OpenBrain onboarding gates the graph with a dimming scrim and locks interaction', () => {
  assert.match(source, /<OpenBrainFlowGraph/);
  assert.match(source, /interactive=\{!showOnboardingOverlay\}/);
  assert.match(source, /key=\{graphFlowKey\}/);
  assert.match(source, /openbrain-onboarding-scrim absolute inset-0 z-\[30\]/);
  assert.match(source, /openbrain-onboarding-overlay absolute inset-0 z-40 flex items-center justify-center/);
  assert.match(
    source,
    /\.openbrain-flow-locked,\s*\.openbrain-flow-locked \*\s*\{[\s\S]*?pointer-events: none !important/,
  );
  assert.match(source, /onPointerDown=\{\(event\) => event\.stopPropagation\(\)\}/);
  assert.match(source, /openbrain-onboarding-action no-drag \$\{OPENBRAIN_GRAPH_CAPSULE\}/);
  assert.match(styles, /\.openbrain-onboarding-scrim\s*\{[^}]*z-index: 30/);
  assert.match(styles, /\.openbrain-onboarding-scrim\s*\{[^}]*pointer-events: auto/);
  assert.match(styles, /\.openbrain-onboarding-overlay\s*\{[^}]*z-index: 40/);
  assert.doesNotMatch(styles, /\.openbrain-onboarding-overlay\s*\{[^}]*pointer-events: none/);
  assert.doesNotMatch(source, /bottom-7/);
});

test('OpenBrain onboarding scrim dims the demo graph without blur', () => {
  const scrimBlock = styles.match(/\.openbrain-onboarding-scrim\s*\{[^}]*\}/)?.[0] ?? '';
  assert.match(scrimBlock, /rgba\(0,\s*0,\s*0,\s*0\.4\)/);
  assert.doesNotMatch(scrimBlock, /backdrop-filter/);
  assert.doesNotMatch(scrimBlock, /backdrop-blur/);
});

test('OpenBrain onboarding supports login, connect GitHub, and add source steps', () => {
  assert.match(source, /const providerStatusChecked = useOpenBrainStore\(\(state\) => state\.providerStatusChecked\);/);
  assert.match(source, /const authInitialized = useAuthStore\(\(state\) => state\.initialized\);/);
  assert.match(source, /const readinessKnown = authInitialized && \(provider !== 'cloud' \|\| providerStatusChecked\);/);
  assert.match(source, /const needsGitHubConnection = readinessKnown && loggedIn && provider === 'cloud' && !cloudReady;/);
  assert.match(source, /const showDemoGraph = authInitialized && \(!loggedIn \|\| needsGitHubConnection\);/);
  assert.match(source, /const showOnboardingOverlay = readinessKnown && \(!loggedIn \|\| needsGitHubConnection \|\| sources\.length === 0\);/);
  assert.match(source, /const graphFlowKey = /);
  assert.match(source, /disabled=\{onboardingBusy \|\| loading\}/);
  assert.match(source, /const hydrateCachedOpenBrains = useOpenBrainStore\(\(state\) => state\.hydrateCachedSources\);/);
  assert.match(source, /const refreshOpenBrainsInBackground = useOpenBrainStore\(\(state\) => state\.refreshInBackground\);/);
  assert.match(source, /const refreshProviderStatus = useOpenBrainStore\(\(state\) => state\.refreshProviderStatus\);/);
  assert.match(source, /const authRevision = useAuthStore\(\(state\) => state\.authRevision\);/);
  assert.match(source, /hydrateCachedOpenBrains\(\)\.catch\(\(\) => \{\}\)/);
  assert.match(source, /refreshOpenBrainsInBackground\(undefined, \{ force: authChanged \}\)\.catch\(\(\) => \{\}\)/);
  assert.match(source, /}, \[authRevision, hydrateCachedOpenBrains, refreshOpenBrainsInBackground\]\);/);
  assert.match(source, /providerStatusRefreshInFlightRef/);
  assert.match(source, /const status = await refreshProviderStatus\(\);[\s\S]*status\.cloudReady === true[\s\S]*refreshOpenBrainsInBackground\(undefined, \{ force: true \}\)/);
  assert.match(source, /window\.setInterval\(\(\) => \{[\s\S]*refreshCloudReadiness\(\);[\s\S]*\}, 2500\)/);
  assert.match(source, /onboardingStep === 'connect_github'/);
  assert.match(source, /showLoginRequiredDialog\('chat'\)/);
  assert.match(source, /openStorageBackendSettings\?\.\(\{ storageBackend: 'github' \}\)/);
  assert.match(source, /openBrainOnboarding\.connectGitHubTitle/);
  assert.match(source, /onboardingInlineError/);
});

test('OpenBrainPage omits duplicate chrome above the graph surface', () => {
  assert.match(
    source,
    /<div ref=\{viewportRef\} className="min-h-0 flex-1 overflow-hidden px-7 py-7">/,
  );
  assert.match(source, /OpenBrainFlowGraph/);
  assert.doesNotMatch(source, /import \{ OpenBrainLogo \} from '\.\.\/Icons';/);
  assert.doesNotMatch(source, /Cloud and local sources/);
  assert.doesNotMatch(source, /<div className="flex h-9 shrink-0 items-center gap-3 border-b px-\[18px\]"/);
  assert.doesNotMatch(source, /<div className="ui-tabbar flex shrink-0 items-center justify-center overflow-hidden px-2">/);
  assert.doesNotMatch(source, /aria-label="Refresh OpenBrain sources"/);
  assert.doesNotMatch(source, /aria-label="OpenBrain settings"/);
  assert.doesNotMatch(source, /className="flex h-12 shrink-0 items-center gap-3 border-b px-\[18px\]"/);
});

test('OpenBrainPage exposes a top-right forced graph refresh button', () => {
  assert.match(source, /import \{ IconButton \} from '\.\.\/IconButton';/);
  assert.match(source, /import \{ ChatLineIcon, RefreshIcon, TrashIcon \} from '\.\.\/Icons';/);
  assert.match(source, /const refreshing = useOpenBrainStore\(\(state\) => state\.refreshing\);/);
  assert.match(source, /const handleRefreshOpenBrainGraph = useCallback\(\(\) => \{/);
  assert.match(
    source,
    /refreshOpenBrainsInBackground\(undefined, \{ force: true \}\)\.catch\(\(err\) => \{[\s\S]*pushToast\(err instanceof Error \? err\.message : 'Failed to refresh OpenBrain graph\.'\);/,
  );
  assert.match(
    source,
    /<IconButton[\s\S]*className="no-drag absolute right-4 top-4 z-50"[\s\S]*title="Refresh OpenBrain graph"[\s\S]*aria-label="Refresh OpenBrain graph"[\s\S]*disabled=\{loading \|\| refreshing\}[\s\S]*onClick=\{handleRefreshOpenBrainGraph\}/,
  );
  assert.match(source, /<RefreshIcon className=\{`h-4 w-4\$\{refreshing \? ' animate-spin' : ''\}`\} \/>/);
});

test('MyGBrain add popover portals to document.body to escape stage paint containment', () => {
  assert.match(source, /addPopoverOpen \? createPortal\(/);
  assert.match(source, /<MyGBrainAddPopover[\s\S]*document\.body,/);
});

test('source sharing uses app dialog instead of native browser prompts', () => {
  assert.match(source, /<SourceShareDialog/);
  assert.match(source, /getPublicBrainProfile/);
  assert.match(source, /updatePublicBrainProfile/);
  assert.doesNotMatch(source, /window\.prompt/);
  assert.doesNotMatch(source, /window\.confirm/);
});
