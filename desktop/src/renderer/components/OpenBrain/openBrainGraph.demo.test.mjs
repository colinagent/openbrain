import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const flowSource = readFileSync(
  path.resolve(import.meta.dirname, './openBrainFlow.ts'),
  'utf8',
);
const pageSource = readFileSync(
  path.resolve(import.meta.dirname, './OpenBrainPage.tsx'),
  'utf8',
);
const rendererSource = readFileSync(
  path.resolve(import.meta.dirname, './OpenBrainFlowGraph.tsx'),
  'utf8',
);
const interactionsSource = readFileSync(
  path.resolve(import.meta.dirname, './openBrainInteractions.ts'),
  'utf8',
);
const indexCssSource = readFileSync(
  path.resolve(import.meta.dirname, '../../styles/index.css'),
  'utf8',
);

test('OpenBrain source pills show connected and disabled query subtitles', () => {
  assert.match(pageSource, /resolveOpenBrainSourceDisplayState/);
  assert.match(pageSource, /sourceDisplay\?\.statusText/);
  assert.doesNotMatch(flowSource, /runtimeLabel/);
  assert.doesNotMatch(flowSource, /export function resolveSourceNodeSubtitle/);
  assert.doesNotMatch(flowSource, /function workspaceSubtitle/);
  assert.match(rendererSource, /openbrain-source-subtitle block truncate text-\[8px\] leading-\[10px\]/);
  assert.match(rendererSource, /openbrain-source-public/);
  assert.match(indexCssSource, /\.openbrain-source-node\.openbrain-graph-capsule \.openbrain-source-subtitle \{[\s\S]*color: #2f8f6b/);
  assert.match(indexCssSource, /\.openbrain-source-node\.openbrain-source-public\.openbrain-graph-capsule \.openbrain-source-subtitle \{[\s\S]*color: var\(--color-preview-callout-warning\)/);
  assert.match(indexCssSource, /\.openbrain-source-node\.openbrain-source-unlinked\.openbrain-graph-capsule \.openbrain-source-subtitle \{[\s\S]*color: var\(--color-secondary-text\)/);
});

test('demo graph spec matches the OpenBrain brain map sample', () => {
  assert.match(flowSource, /label: "Alex's Brain"/);
  assert.match(flowSource, /subtitle: 'personal notes'/);
  assert.match(flowSource, /subtitle: 'capture queue'/);
  assert.doesNotMatch(flowSource, /Journal/);
  assert.match(flowSource, /label: "OpenBrain's Brain"/);
  assert.match(flowSource, /subtitle: 'Shared brain for teams'/);
  assert.match(flowSource, /peerBrain: true/);
  assert.match(flowSource, /kind: 'peer'/);
  assert.match(flowSource, /label: 'MyGBrain'/);
  assert.match(flowSource, /label: 'Growth'/);
});

test('OpenBrainPage uses XYFlow demo mode and keeps onboarding/popover behavior', () => {
  assert.doesNotMatch(pageSource, /DEMO_OPENBRAIN_WORKSPACES/);
  assert.doesNotMatch(pageSource, /#f7f4ed/);
  assert.match(pageSource, /demoMode: true/);
  assert.match(pageSource, /buildOpenBrainFlow/);
  assert.match(pageSource, /OpenBrainFlowGraph/);
  assert.doesNotMatch(pageSource, /layoutOpenBrainGraph/);
  assert.doesNotMatch(pageSource, /computeOpenBrainStageScale/);
  assert.match(pageSource, /openBrainOnboarding\.loginTitle/);
  assert.match(pageSource, /teamBrainClusterVisible/);
  assert.match(pageSource, /MyGBrainAddPopover/);
});

test('real OpenBrain graph does not request the demo team cluster', () => {
  assert.match(pageSource, /buildOpenBrainFlow\(\[\], peerLinks, \{ demoMode: true, teamBrainClusterVisible \}\)/);
  assert.match(pageSource, /buildOpenBrainFlow\(sources\.map\(toGraphWorkspace\), peerLinks, \{ publicBrains: graphPublicBrains \}\)/);
  assert.match(pageSource, /\.filter\(\(brain\) => brain\.activeSourceCount > 0\)/);
  assert.match(pageSource, /avatar: brain\.avatar/);
  assert.match(pageSource, /brainID: brain\.brainID/);
  assert.match(flowSource, /avatar\?: string;/);
  assert.match(flowSource, /brainID: string;/);
  assert.doesNotMatch(flowSource, /OpenBrainFlowPublicBrainSource/);
});

test('OpenBrain graph click semantics use real cloud disconnect actions', () => {
  assert.match(interactionsSource, /centerSingle: 'open-chat'/);
  assert.match(interactionsSource, /centerContext: 'add-popover'/);
  assert.match(interactionsSource, /sourceSingle: 'open-chat-source-scope'/);
  assert.match(interactionsSource, /sourceContext: 'source-management'/);
  assert.match(interactionsSource, /peerSingle: 'open-chat-public-brain-scope'/);
  assert.match(interactionsSource, /peerContext: 'public-brain-management'/);
  assert.doesNotMatch(interactionsSource, /single click selects the source only/);
  assert.doesNotMatch(interactionsSource, /double-click/);
  assert.doesNotMatch(interactionsSource, /isDoubleClick/);
  assert.match(pageSource, /handleNodeContextMenu/);
  assert.match(pageSource, /setAddPopoverOpen\(true\)/);
  assert.match(pageSource, /setSourceLinked\(sourceKey, !isSourceLinked\(sourceKey\)\)/);
  assert.match(pageSource, /disconnectPublicBrain\(node\)/);
  assert.match(pageSource, /startPublicBrainChat\(node\)/);
  assert.match(pageSource, /setHostedChatBrain/);
  assert.match(pageSource, /gbrainSourceScopeForWorkspace\(workspace\)/);
  assert.doesNotMatch(pageSource, /publicBrainScopeForNode/);
  assert.doesNotMatch(pageSource, /resolvePublicBrainScopeForNode/);
  assert.doesNotMatch(pageSource, /resolveOpenBrainPublicBrainSources/);
  assert.doesNotMatch(pageSource, /openingPublicBrainOwnerUIDRef/);
  assert.doesNotMatch(pageSource, /Public brain source IDs are not loaded yet/);
  assert.doesNotMatch(pageSource, /toggleCloudSourceQueries/);
  assert.match(pageSource, /unfollowPublicBrain\(ownerUID\)/);
  assert.doesNotMatch(pageSource, /scheduleNodeSingleClick/);
  assert.doesNotMatch(pageSource, /isDoubleClick/);
  assert.match(rendererSource, /data\.onNodeAction\?\.\(node, event\);/);
  assert.match(rendererSource, /onContextMenu=\{\(event\) => data\.onNodeContextMenu\?\.\(node, event\)\}/);
  assert.doesNotMatch(rendererSource, /if \(event\.detail > 1\)/);
});

test('OpenBrainFlowGraph keeps the established node styling hooks', () => {
  assert.match(rendererSource, /openbrain-peer-brain-node/);
  assert.match(rendererSource, /openbrain-peer-brain-img/);
  assert.match(rendererSource, /openbrain-avatar/);
  assert.match(rendererSource, /Click to open chat/);
  assert.match(rendererSource, /Right-click to add sources or public brains/);
  assert.match(rendererSource, /OPENBRAIN_GRAPH_CAPSULE/);
  assert.match(rendererSource, /openbrain-cluster-enterprise-panel/);
  assert.match(rendererSource, /openbrain-cluster-restore-btn/);
  assert.match(rendererSource, /getBezierPath/);
  assert.doesNotMatch(rendererSource, /getSmoothStepPath/);
  assert.doesNotMatch(rendererSource, /openbrain-core-tip/);
  assert.doesNotMatch(pageSource, /openbrain-avatar-wrap::after/);
  assert.doesNotMatch(pageSource, /content: '\\+'/);
});
