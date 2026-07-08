import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

import {
  buildOpenBrainFlow,
  DEFAULT_OPENBRAIN_PEER_LINKS,
  TEAM_BRAIN_CLUSTER_MEMBER_COUNT,
} from './openBrainFlow.ts';
import { resolveOpenBrainSourceDisplayState } from './openBrainSourceDisplay.ts';

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
const packageSource = readFileSync(
  path.resolve(import.meta.dirname, '../../../../package.json'),
  'utf8',
);

function center(node) {
  return {
    x: node.position.x + (node.width || 0) / 2,
    y: node.position.y + (node.height || 0) / 2,
  };
}

function distance(a, b) {
  return Math.hypot(a.x - b.x, a.y - b.y);
}

function normalizedAngle(a, b) {
  const angle = Math.atan2(b.y - a.y, b.x - a.x);
  return angle < 0 ? angle + Math.PI * 2 : angle;
}

function sideForDelta(dx, dy) {
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0 ? 'right' : 'left';
  }
  return dy >= 0 ? 'bottom' : 'top';
}

function oppositeSide(side) {
  if (side === 'top') {
    return 'bottom';
  }
  if (side === 'bottom') {
    return 'top';
  }
  if (side === 'left') {
    return 'right';
  }
  return 'left';
}

test('OpenBrainPage uses XYFlow instead of handwritten SVG graph geometry', () => {
  assert.match(pageSource, /OpenBrainFlowGraph/);
  assert.match(pageSource, /buildOpenBrainFlow/);
  assert.doesNotMatch(pageSource, /layoutOpenBrainGraph/);
  assert.doesNotMatch(pageSource, /computeOpenBrainStageScale/);
  assert.doesNotMatch(pageSource, /renderGraphLinkPaths/);
  assert.doesNotMatch(pageSource, /useEdgeAnchors/);
  assert.doesNotMatch(pageSource, /getBoundingClientRect/);
  assert.doesNotMatch(pageSource, /<svg/);
});

test('XYFlow renderer disables editing and uses a biased fit for screen fit', () => {
  assert.match(rendererSource, /from '@xyflow\/react'/);
  assert.match(rendererSource, /<ReactFlow/);
  assert.match(rendererSource, /openbrain-flow openbrain-stage-graph/);
  assert.match(rendererSource, /openbrain-flow-locked/);
  assert.match(rendererSource, /panOnDrag=\{interactive\}/);
  assert.match(rendererSource, /panOnScroll=\{interactive\}/);
  assert.match(rendererSource, /hiddenDirectionalHandles/);
  assert.match(rendererSource, /getViewportForBounds/);
  assert.match(rendererSource, /FLOW_FIT_TOP_PADDING_RATIO = 0\.08/);
  assert.match(rendererSource, /targetTop - currentTop/);
  assert.doesNotMatch(rendererSource, /\sfitView\s*$/m);
  assert.doesNotMatch(rendererSource, /fitViewOptions/);
  assert.match(rendererSource, /nodesDraggable=\{false\}/);
  assert.match(rendererSource, /nodesConnectable=\{false\}/);
  assert.match(rendererSource, /elementsSelectable=\{false\}/);
  assert.match(rendererSource, /zoomOnDoubleClick=\{false\}/);
});

test('OpenBrain desktop declares XYFlow and Dagre dependencies', () => {
  assert.match(packageSource, /"@xyflow\/react":/);
  assert.match(packageSource, /"dagre":/);
  assert.match(packageSource, /"@types\/dagre":/);
});

test('flow model uses Dagre and React Flow node-edge data instead of SVG paths', () => {
  assert.match(flowSource, /from 'dagre'/);
  assert.match(flowSource, /type \{ Edge, Node \} from '@xyflow\/react'/);
  assert.match(flowSource, /layoutPersonalOrbit/);
  assert.doesNotMatch(flowSource, /layoutDagre\(nodes, edges, 'LR'/);
  assert.doesNotMatch(flowSource, /\bd:\s*string/);
  assert.doesNotMatch(flowSource, /softTreePath/);
  assert.doesNotMatch(flowSource, /linkAnchors/);
});

test('demo flow includes personal sources, company root, departments, and members', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
    teamBrainClusterVisible: true,
  });
  const ids = new Set(flow.nodes.map((node) => node.id));
  assert.ok(ids.has('user-root'));
  assert.ok(ids.has('peerBrain'));
  assert.ok(ids.has('workspace:inbox'));
  assert.ok(ids.has('workspace:note'));
  assert.ok(ids.has('workspace:projects'));
  assert.ok(ids.has('workspace:research'));
  assert.ok(ids.has('enterprise'));
  assert.ok(ids.has('eng'));
  assert.ok(ids.has('hr'));
  assert.ok(ids.has('growth'));
  assert.equal(flow.nodes.filter((node) => node.data.kind === 'member').length, TEAM_BRAIN_CLUSTER_MEMBER_COUNT);
});

test('real source flow excludes the fake company cluster', () => {
  const flow = buildOpenBrainFlow([
    {
      workspaceID: 'note',
      orgID: 'cloud',
      name: 'note',
      path: '/tmp/note',
      updatedAt: '2026-06-25T12:00:00Z',
    },
  ], DEFAULT_OPENBRAIN_PEER_LINKS, {});
  const ids = new Set(flow.nodes.map((node) => node.id));
  assert.ok(ids.has('user-root'));
  assert.ok(ids.has('workspace:note'));
  assert.ok(!ids.has('enterprise'));
  assert.ok(!ids.has('eng'));
  assert.ok(!ids.has('hr'));
  assert.ok(!ids.has('growth'));
  assert.ok(!ids.has('team-restore'));
  assert.equal(flow.nodes.filter((node) => node.data.kind === 'department').length, 0);
  assert.equal(flow.nodes.filter((node) => node.data.kind === 'member').length, 0);
  assert.ok(flow.edges.every((edge) => edge.data?.kind !== 'hierarchy'));
});

test('real source flow uses a compact MyGBrain orbit instead of the demo radius', () => {
  const flow = buildOpenBrainFlow([
    {
      workspaceID: 'note',
      orgID: 'cloud',
      name: 'note',
      path: '/tmp/note',
      updatedAt: '2026-06-25T12:00:00Z',
    },
  ], DEFAULT_OPENBRAIN_PEER_LINKS, {});
  const byID = new Map(flow.nodes.map((node) => [node.id, node]));
  const root = byID.get('user-root');
  const note = byID.get('workspace:note');
  assert.ok(root);
  assert.ok(note);
  assert.ok(distance(center(root), center(note)) <= 230, 'real source link should be shorter than the demo orbit');
  assert.match(flowSource, /PERSONAL_REAL_ORBIT_MIN_RADIUS = 230/);
  assert.match(flowSource, /PERSONAL_DEMO_ORBIT_MIN_RADIUS = 250/);
});

test('real source flow labels public sources as public instead of connected', () => {
  const workspace = {
    workspaceID: 'openbrain',
    orgID: 'cloud',
    name: 'openbrain',
    publicAccess: true,
  };
  const flow = buildOpenBrainFlow([workspace], DEFAULT_OPENBRAIN_PEER_LINKS, {});
  const source = flow.nodes.find((node) => node.id === 'workspace:openbrain');
  assert.ok(source);
  assert.equal(source.data.subtitle, 'Connected');
  const display = resolveOpenBrainSourceDisplayState({
    sourceID: workspace.workspaceID,
    name: workspace.name,
    openable: true,
    publicAccess: true,
  }, { provider: 'cloud', uiLinked: true });
  assert.equal(display.statusText, 'Public');
});

test('real source flow labels unreachable runtime bindings as offline', () => {
  const workspace = {
    workspaceID: 'openbrain',
    orgID: 'cloud',
    name: 'openbrain',
    instanceID: 'host-remote',
    runtimeReachable: false,
  };
  const flow = buildOpenBrainFlow([workspace], DEFAULT_OPENBRAIN_PEER_LINKS, {});
  const source = flow.nodes.find((node) => node.id === 'workspace:openbrain');
  assert.ok(source);
  assert.equal(source.data.subtitle, 'Connected');
  const display = resolveOpenBrainSourceDisplayState({
    sourceID: workspace.workspaceID,
    name: workspace.name,
    openable: true,
    instanceID: workspace.instanceID,
    runtimeReachable: false,
  }, { provider: 'cloud', uiLinked: true });
  assert.equal(display.statusText, 'Runtime offline');
});

test('real source flow keys cloud sources by source identity, not runtime instance', () => {
  const flow = buildOpenBrainFlow([
    {
      sourceID: 'source-openbrain',
      workspaceID: 'openbrain',
      orgID: 'cloud',
      name: 'openbrain',
      instanceID: 'host-local',
    },
    {
      sourceID: 'source-openbrain',
      workspaceID: 'openbrain',
      orgID: 'cloud',
      name: 'openbrain',
      instanceID: 'host-remote',
    },
  ], DEFAULT_OPENBRAIN_PEER_LINKS, {});
  const matching = flow.nodes.filter((node) => node.id === 'workspace:openbrain');
  assert.equal(matching.length, 1);
  assert.equal(matching[0]?.data.instanceID, 'host-local');
  assert.doesNotMatch(flowSource, /workspace:\$\{explicit\}:\$\{workspace\.instanceID/);
});

test('MyGBrain renders as a larger React Flow orbit node with a centered avatar', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
  });
  const userRoot = flow.nodes.find((node) => node.id === 'user-root');
  assert.ok(userRoot);
  assert.ok((userRoot.width || 0) > 72, 'user node contains room for the 72px avatar plus orbit rings');
  assert.equal(userRoot.width, 160);
  assert.equal(userRoot.width, userRoot.height);
  assert.match(rendererSource, /openbrain-personal-orbit/);
  assert.match(rendererSource, /openbrain-personal-ring--outer/);
  assert.match(rendererSource, /openbrain-personal-ring--inner/);
  assert.match(pageSource, /\.openbrain-core-node\s*\{[\s\S]*transform: translate\(-50%, -50%\)/);
  assert.match(pageSource, /\.openbrain-personal-ring\s*\{[\s\S]*background: transparent/);
  assert.match(pageSource, /\.openbrain-personal-ring--outer\s*\{[\s\S]*border-color: rgba\(47, 138, 122, 0\.14\)/);
  assert.doesNotMatch(pageSource, /\.openbrain-core-node::before/);
  assert.doesNotMatch(pageSource, /\.openbrain-core-node::after/);
});

test('personal sources and peer brains orbit around MyGBrain at a balanced radius', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
  });
  const byID = new Map(flow.nodes.map((node) => [node.id, node]));
  const root = byID.get('user-root');
  const orbitNodes = ['peerBrain', 'workspace:inbox', 'workspace:note', 'workspace:projects', 'workspace:research']
    .map((id) => byID.get(id));
  assert.ok(root);
  assert.ok(orbitNodes.every(Boolean));

  const rootCenter = center(root);
  const distances = orbitNodes.map((node) => distance(rootCenter, center(node)));
  const averageDistance = distances.reduce((sum, value) => sum + value, 0) / distances.length;
  for (const value of distances) {
    assert.ok(Math.abs(value - averageDistance) <= 2, `orbit radius ${value} should match ${averageDistance}`);
  }

  const angles = orbitNodes.map((node) => normalizedAngle(rootCenter, center(node))).sort((a, b) => a - b);
  const angleKeys = new Set(angles.map((angle) => angle.toFixed(2)));
  assert.equal(angleKeys.size, orbitNodes.length, 'orbit nodes do not share duplicate angles');
  for (let index = 0; index < angles.length; index += 1) {
    const next = angles[(index + 1) % angles.length] + (index === angles.length - 1 ? Math.PI * 2 : 0);
    assert.ok(next - angles[index] > 0.6, 'orbit nodes are separated around the circle');
  }
});

test('personal edges use directional handles around the MyGBrain orbit', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
  });
  const personalIDs = new Set(['peerBrain', 'workspace:inbox', 'workspace:note', 'workspace:projects', 'workspace:research']);
  const byID = new Map(flow.nodes.map((node) => [node.id, node]));
  const root = byID.get('user-root');
  assert.ok(root);
  const rootCenter = center(root);
  const personalEdges = flow.edges.filter((edge) => personalIDs.has(edge.target));
  assert.equal(personalEdges.length, personalIDs.size);

  for (const edge of personalEdges) {
    const target = byID.get(edge.target);
    assert.ok(target);
    const targetCenter = center(target);
    const expectedSourceHandle = sideForDelta(targetCenter.x - rootCenter.x, targetCenter.y - rootCenter.y);
    assert.equal(edge.sourceHandle, expectedSourceHandle, `${edge.id} source handle`);
    assert.equal(edge.targetHandle, oppositeSide(expectedSourceHandle), `${edge.id} target handle`);
  }
  assert.ok(personalEdges.some((edge) => edge.sourceHandle !== 'right' || edge.targetHandle !== 'left'));
});

test('personal orbit edges render as floating edges from the MyGBrain ring', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
  });
  const personalIDs = new Set(['peerBrain', 'workspace:inbox', 'workspace:note', 'workspace:projects', 'workspace:research']);
  const personalEdges = flow.edges.filter((edge) => personalIDs.has(edge.target));

  assert.equal(personalEdges.length, personalIDs.size);
  assert.ok(personalEdges.every((edge) => edge.data?.floating === true));
  assert.match(rendererSource, /useStore/);
  assert.match(rendererSource, /floatingEdgeParams/);
  assert.match(rendererSource, /floatingSingleBendPath/);
  assert.match(rendererSource, /'Q'/);
  assert.match(rendererSource, /USER_INNER_RING_INSET = 28/);
  assert.match(rendererSource, /floatingBoundaryPoint/);
  assert.match(rendererSource, /pointOnCircle/);
  assert.doesNotMatch(rendererSource, /curvature: data\?\.floating/);
});

test('OpenBrainPage keeps personal orbit lines visible when demo nodes are unlinked', () => {
  assert.match(pageSource, /const isTeamPeerEdge = edge\.data\?\.kind === 'peer' && TEAM_BRAIN_CLUSTER_PEER_IDS\.has\(edge\.target\);/);
  assert.match(pageSource, /isTeamPeerEdge && \(!targetLinked \|\| !teamBrainClusterVisible\)/);
  assert.doesNotMatch(pageSource, /edge\.data\?\.kind === 'source' && !targetLinked/);
});

test('company hierarchy edges connect root to departments and departments to their members through handles', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
    teamBrainClusterVisible: true,
  });
  const hierarchyEdges = flow.edges.filter((edge) => edge.data?.kind === 'hierarchy');
  const rootEdges = hierarchyEdges.filter((edge) => edge.source === 'enterprise');
  assert.deepEqual(rootEdges.map((edge) => edge.target), ['eng', 'hr', 'growth']);
  for (const edge of rootEdges) {
    assert.equal(edge.sourceHandle, 'bottom');
    assert.equal(edge.targetHandle, 'top');
  }

  const memberEdges = hierarchyEdges.filter((edge) => edge.source !== 'enterprise');
  assert.deepEqual(memberEdges.map((edge) => [edge.source, edge.target]), [
    ['eng', 'ak'],
    ['eng', 'jl'],
    ['hr', 'rm'],
    ['hr', 'lw'],
    ['growth', 'st'],
  ]);
  for (const edge of memberEdges) {
    assert.equal(edge.sourceHandle, 'bottom');
    assert.equal(edge.targetHandle, 'top');
  }
});

test('departments and members are positioned by ownership, not fixed HR coordinates', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
    teamBrainClusterVisible: true,
  });
  const byID = new Map(flow.nodes.map((node) => [node.id, node]));
  const root = byID.get('enterprise');
  const departments = ['eng', 'hr', 'growth'].map((id) => byID.get(id));
  assert.ok(root);
  assert.ok(departments.every(Boolean));

  const rootCenter = center(root);
  const middleDepartment = departments[1];
  assert.ok(Math.abs(center(middleDepartment).x - rootCenter.x) <= 1, 'middle department is centered under company root');
  assert.ok(center(departments[0]).x < rootCenter.x, 'first department is left of company root');
  assert.ok(center(departments[2]).x > rootCenter.x, 'last department is right of company root');

  for (const department of departments) {
    const members = flow.nodes.filter((node) => node.data.kind === 'member' && node.data.teamID === department.id);
    assert.ok(members.length > 0, `${department.id} has members`);
    const averageMemberX = members.reduce((sum, node) => sum + center(node).x, 0) / members.length;
    assert.ok(Math.abs(averageMemberX - center(department).x) <= 1, `${department.id} members center below owning department`);
    for (const member of members) {
      assert.ok(center(member).y > center(department).y, `${member.id} sits below ${department.id}`);
    }
  }
});

test('personal brain is vertically balanced with the company department row in demo mode', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
    teamBrainClusterVisible: true,
  });
  const byID = new Map(flow.nodes.map((node) => [node.id, node]));
  const root = byID.get('user-root');
  const departments = ['eng', 'hr', 'growth'].map((id) => byID.get(id));
  assert.ok(root);
  assert.ok(departments.every(Boolean));

  const departmentCenterY = departments.reduce((sum, node) => sum + center(node).y, 0) / departments.length;
  assert.ok(Math.abs(center(root).y - departmentCenterY) <= 1, 'MyGBrain center should align with the company department row');
  assert.match(flowSource, /PERSONAL_ROOT_FALLBACK_CENTER_Y/);
});

test('hiding the team cluster hides demo company graph and exposes only the restore node', () => {
  const flow = buildOpenBrainFlow([], DEFAULT_OPENBRAIN_PEER_LINKS, {
    demoMode: true,
    teamBrainClusterVisible: false,
  });
  const byID = new Map(flow.nodes.map((node) => [node.id, node]));
  assert.equal(byID.get('enterprise')?.hidden, true);
  assert.equal(byID.get('eng')?.hidden, true);
  assert.equal(byID.get('hr')?.hidden, true);
  assert.equal(byID.get('growth')?.hidden, true);
  assert.equal(byID.get('team-restore')?.hidden, false);
  assert.ok(flow.edges.every((edge) => edge.data?.kind !== 'peer' || !['enterprise', 'eng', 'hr', 'growth', 'ak', 'jl', 'rm', 'lw', 'st'].includes(edge.target)));
  assert.ok(flow.edges.filter((edge) => edge.data?.kind === 'hierarchy').every((edge) => edge.hidden));
});
