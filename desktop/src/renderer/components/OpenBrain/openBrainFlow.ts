import dagre from 'dagre';
import type { Edge, Node } from '@xyflow/react';

export type OpenBrainFlowNodeKind =
  | 'user'
  | 'source'
  | 'peer'
  | 'companyRoot'
  | 'department'
  | 'member'
  | 'teamRestore';

export type DemoNodeColorToken = {
  color: string;
  color2: string;
  rgb: string;
};

export type OpenBrainFlowWorkspace = {
  sourceID?: string;
  workspaceID?: string;
  orgID?: string;
  slug?: string;
  name: string;
  path?: string;
  instanceID?: string;
  runtimeReachable?: boolean;
  bindingStatus?: 'connected' | 'needs_binding';
  bindingReason?: 'unbound' | 'moved' | 'mismatch';
  disabledQueries?: boolean;
  publicAccess?: boolean;
  effectivePermission?: 'read' | 'write' | 'admin';
  canMutateSource?: boolean;
  publicOwnerUID?: string;
  bindingMode?: 'own' | 'granted';
  updatedAt?: string;
};

export type OpenBrainFlowPublicBrain = {
  ownerUID: string;
  name: string;
  username: string;
  ownerInitial?: string;
  avatar?: string;
  activeSourceCount?: number;
  colorKey: string;
  sources: OpenBrainFlowPublicBrainSource[];
};

export type OpenBrainFlowPublicBrainSource = {
  sourceID: string;
  name?: string;
  workspaceID?: string;
  orgID?: string;
};

export type OpenBrainDemoOrbitNodeKind = 'source' | 'peer';

export type OpenBrainDemoOrbitNode = {
  id: string;
  kind: OpenBrainDemoOrbitNodeKind;
  label: string;
  subtitle: string;
  initial: string;
  colorKey: string;
  defaultLinked: boolean;
};

export type OpenBrainPeerLinkState = Record<string, boolean>;

export type OpenBrainTeamMember = {
  id: string;
  teamID: string;
  label: string;
  sourceCount: number;
  defaultLinked: boolean;
  accentColor: string;
  accentColor2: string;
  accentRgb: string;
};

export type OpenBrainTeam = {
  id: string;
  label: string;
  defaultLinked: boolean;
  members: OpenBrainTeamMember[];
};

export type OpenBrainFlowNodeData = {
  kind: OpenBrainFlowNodeKind;
  label: string;
  subtitle?: string;
  initial?: string;
  sourceID?: string;
  workspaceID?: string;
  orgID?: string;
  path?: string;
  instanceID?: string;
  runtimeReachable?: boolean;
  publicAccess?: boolean;
  effectivePermission?: 'read' | 'write' | 'admin';
  canMutateSource?: boolean;
  publicOwnerUID?: string;
  publicBrainSources?: OpenBrainFlowPublicBrainSource[];
  bindingMode?: 'own' | 'granted';
  ownerUID?: string;
  username?: string;
  avatar?: string;
  colorKey?: string;
  accentColor?: string;
  accentColor2?: string;
  accentRgb?: string;
  sourceCount?: number;
  teamID?: string;
  teamMembers?: OpenBrainTeamMember[];
  defaultLinked?: boolean;
  linked?: boolean;
  hidden?: boolean;
};

export type OpenBrainFlowNode = Node<OpenBrainFlowNodeData, 'openbrainNode'>;

export type OpenBrainFlowEdgeData = {
  kind: 'source' | 'peer' | 'hierarchy';
  color: string;
  floating?: boolean;
  strong?: boolean;
};

export type OpenBrainFlowEdge = Edge<OpenBrainFlowEdgeData>;

export type OpenBrainFlowLayout = {
  nodes: OpenBrainFlowNode[];
  edges: OpenBrainFlowEdge[];
  width: number;
  height: number;
};

const USER_NODE_SIZE = 160;
const SOURCE_NODE_WIDTH = 120;
const SOURCE_NODE_HEIGHT = 36;
const PEER_NODE_SIZE = 64;
const COMPANY_ROOT_SIZE = 88;
const DEPARTMENT_NODE_WIDTH = 150;
const DEPARTMENT_NODE_HEIGHT = 72;
const MEMBER_NODE_SIZE = 52;
const RESTORE_NODE_SIZE = 40;
const CLUSTER_GAP = 190;
const GRAPH_LEFT_OFFSET = 96;
const TEAM_GRAPH_TOP_OFFSET = 64;
const PERSONAL_ROOT_FALLBACK_CENTER_Y = 266;
const PERSONAL_DEMO_ORBIT_MIN_RADIUS = 250;
const PERSONAL_REAL_ORBIT_MIN_RADIUS = 230;
const PERSONAL_ORBIT_NODE_GAP = 54;
const PERSONAL_DEMO_ORBIT_PADDING = 32;
const PERSONAL_REAL_ORBIT_PADDING = 16;
const PERSONAL_ORBIT_START_ANGLE = -Math.PI / 2;

type FlowHandleSide = 'top' | 'right' | 'bottom' | 'left';

export const DEMO_GRAPH_COLORS = {
  focus: '#17604d',
  green: '#2f8f6b',
  greenDeep: '#17604d',
  mint: '#dff3ec',
  rose: '#9b6f63',
  mutedText: '#2f8f6b',
};

const DEMO_NODE_PALETTE: DemoNodeColorToken[] = [
  { color: '#2f8f6b', color2: '#84d8c5', rgb: '47, 143, 107' },
  { color: '#8a7d2e', color2: '#c9b85d', rgb: '138, 125, 46' },
  { color: '#9b6f63', color2: '#d4a096', rgb: '155, 111, 99' },
  { color: '#4f8d7c', color2: '#9ed8c8', rgb: '79, 141, 124' },
  { color: '#17604d', color2: '#65a893', rgb: '23, 96, 77' },
  { color: '#477f88', color2: '#93c7ce', rgb: '71, 127, 136' },
  { color: '#72805a', color2: '#c2cd9d', rgb: '114, 128, 90' },
  { color: '#a06f3a', color2: '#d9b36a', rgb: '160, 111, 58' },
  { color: '#a26372', color2: '#d9a3ae', rgb: '162, 99, 114' },
  { color: '#3f7568', color2: '#89bfb0', rgb: '63, 117, 104' },
];

export const DEMO_NODE_COLORS: Record<string, DemoNodeColorToken> = {
  'source:note': { color: '#5f8d7c', color2: '#a7cdbd', rgb: '95, 141, 124' },
  'brain:alex': { color: '#c36a73', color2: '#f0a1a8', rgb: '195, 106, 115' },
  'team:brain': { color: '#2f8f6b', color2: '#84d8c5', rgb: '47, 143, 107' },
};

export const DEMO_ORBIT_NODES: OpenBrainDemoOrbitNode[] = [
  { id: 'peerBrain', kind: 'peer', label: "Alex's Brain", subtitle: 'personal brain', initial: 'A', colorKey: 'brain:alex', defaultLinked: true },
  { id: 'inbox', kind: 'source', label: 'inbox', subtitle: 'capture queue', initial: 'I', colorKey: 'source:inbox', defaultLinked: true },
  { id: 'note', kind: 'source', label: 'note', subtitle: 'personal notes', initial: 'N', colorKey: 'source:note', defaultLinked: true },
  { id: 'projects', kind: 'source', label: 'projects', subtitle: 'local workspace', initial: 'P', colorKey: 'source:projects', defaultLinked: true },
  { id: 'research', kind: 'source', label: 'research', subtitle: 'knowledge source', initial: 'R', colorKey: 'source:research', defaultLinked: true },
];

const CLOUD_TEAM_INPUTS = [
  {
    id: 'eng',
    label: 'Engineering',
    defaultLinked: true,
    members: [
      { id: 'ak', label: 'AK', sourceCount: 5, defaultLinked: false },
      { id: 'jl', label: 'JL', sourceCount: 3, defaultLinked: false },
    ],
  },
  {
    id: 'hr',
    label: 'HR',
    defaultLinked: false,
    members: [
      { id: 'rm', label: 'RM', sourceCount: 4, defaultLinked: false },
      { id: 'lw', label: 'LW', sourceCount: 2, defaultLinked: false },
    ],
  },
  {
    id: 'growth',
    label: 'Growth',
    defaultLinked: false,
    members: [
      { id: 'st', label: 'ST', sourceCount: 1, defaultLinked: false },
    ],
  },
] as const;

export const TEAM_BRAIN_CLUSTER_PEER_IDS = new Set<string>([
  'enterprise',
  ...CLOUD_TEAM_INPUTS.flatMap((team) => [team.id, ...team.members.map((member) => member.id)]),
]);

export const TEAM_BRAIN_CLUSTER_MEMBER_COUNT = CLOUD_TEAM_INPUTS.reduce(
  (count, team) => count + team.members.length,
  0,
);

export const DEFAULT_OPENBRAIN_PEER_LINKS: OpenBrainPeerLinkState = CLOUD_TEAM_INPUTS.reduce<OpenBrainPeerLinkState>(
  (links, team) => {
    links[team.id] = Boolean(team.defaultLinked);
    team.members.forEach((member) => {
      links[member.id] = Boolean(member.defaultLinked);
    });
    return links;
  },
  { enterprise: false, peerBrain: true },
);

function hashString(value: string): number {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

export function colorForDemoKey(colorKey: string): DemoNodeColorToken {
  return DEMO_NODE_COLORS[colorKey] || DEMO_NODE_PALETTE[hashString(colorKey) % DEMO_NODE_PALETTE.length] || DEMO_NODE_PALETTE[0];
}

function workspaceNodeKey(workspace: OpenBrainFlowWorkspace): string {
  const explicit = workspace.workspaceID || workspace.sourceID || workspace.path || workspace.name || 'workspace';
  return `workspace:${explicit}`;
}

function workspaceRuntimeScore(workspace: OpenBrainFlowWorkspace): number {
  let score = 0;
  if (workspace.runtimeReachable !== false) {
    score += 4;
  }
  if (workspace.bindingStatus !== 'needs_binding') {
    score += 2;
  }
  if (workspace.path) {
    score += 1;
  }
  return score;
}

function dedupeFlowWorkspaces(workspaces: OpenBrainFlowWorkspace[]): OpenBrainFlowWorkspace[] {
  const byKey = new Map<string, OpenBrainFlowWorkspace>();
  for (const workspace of workspaces) {
    const key = workspaceNodeKey(workspace);
    const existing = byKey.get(key);
    if (!existing || workspaceRuntimeScore(workspace) > workspaceRuntimeScore(existing)) {
      byKey.set(key, workspace);
    }
  }
  return Array.from(byKey.values());
}

function workspaceLabel(workspace: OpenBrainFlowWorkspace): string {
  return workspace.name || workspace.path?.split(/[\\/]/).filter(Boolean).pop() || 'source';
}

function workspaceInitial(label: string): string {
  return (label.trim().charAt(0) || 'S').toUpperCase();
}

function nodeSize(node: OpenBrainFlowNode): { width: number; height: number } {
  if (node.data.kind === 'user') {
    return { width: USER_NODE_SIZE, height: USER_NODE_SIZE };
  }
  if (node.data.kind === 'source') {
    return { width: SOURCE_NODE_WIDTH, height: SOURCE_NODE_HEIGHT };
  }
  if (node.data.kind === 'peer') {
    return { width: PEER_NODE_SIZE, height: PEER_NODE_SIZE };
  }
  if (node.data.kind === 'companyRoot') {
    return { width: COMPANY_ROOT_SIZE, height: COMPANY_ROOT_SIZE };
  }
  if (node.data.kind === 'department') {
    return { width: DEPARTMENT_NODE_WIDTH, height: DEPARTMENT_NODE_HEIGHT };
  }
  if (node.data.kind === 'member') {
    return { width: MEMBER_NODE_SIZE, height: MEMBER_NODE_SIZE };
  }
  return { width: RESTORE_NODE_SIZE, height: RESTORE_NODE_SIZE };
}

function makeNode(
  id: string,
  data: OpenBrainFlowNodeData,
): OpenBrainFlowNode {
  const size = nodeSize({ id, type: 'openbrainNode', position: { x: 0, y: 0 }, data });
  return {
    id,
    type: 'openbrainNode',
    data,
    position: { x: 0, y: 0 },
    style: { width: size.width, height: size.height },
    width: size.width,
    height: size.height,
    draggable: false,
    selectable: false,
  };
}

function makeEdge(
  id: string,
  source: string,
  target: string,
  data: OpenBrainFlowEdgeData,
  handles?: { sourceHandle?: string; targetHandle?: string },
): OpenBrainFlowEdge {
  return {
    id,
    source,
    target,
    sourceHandle: handles?.sourceHandle,
    targetHandle: handles?.targetHandle,
    type: 'openbrainSoft',
    data,
    selectable: false,
    focusable: false,
  };
}

function bounds(nodes: OpenBrainFlowNode[]): { minX: number; minY: number; maxX: number; maxY: number } {
  return nodes.reduce((box, node) => {
    const size = nodeSize(node);
    return {
      minX: Math.min(box.minX, node.position.x),
      minY: Math.min(box.minY, node.position.y),
      maxX: Math.max(box.maxX, node.position.x + size.width),
      maxY: Math.max(box.maxY, node.position.y + size.height),
    };
  }, { minX: Infinity, minY: Infinity, maxX: -Infinity, maxY: -Infinity });
}

function offsetNodes(nodes: OpenBrainFlowNode[], offsetX: number, offsetY: number): OpenBrainFlowNode[] {
  return nodes.map((node) => ({
    ...node,
    position: {
      x: Math.round(node.position.x + offsetX),
      y: Math.round(node.position.y + offsetY),
    },
  }));
}

function nodeCenter(node: OpenBrainFlowNode): { x: number; y: number } {
  const size = nodeSize(node);
  return {
    x: node.position.x + size.width / 2,
    y: node.position.y + size.height / 2,
  };
}

function flowSideForVector(dx: number, dy: number): FlowHandleSide {
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0 ? 'right' : 'left';
  }
  return dy >= 0 ? 'bottom' : 'top';
}

function oppositeFlowSide(side: FlowHandleSide): FlowHandleSide {
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

function personalEdgeHandles(root: OpenBrainFlowNode, target: OpenBrainFlowNode): { sourceHandle: FlowHandleSide; targetHandle: FlowHandleSide } {
  const rootCenter = nodeCenter(root);
  const targetCenter = nodeCenter(target);
  const sourceHandle = flowSideForVector(targetCenter.x - rootCenter.x, targetCenter.y - rootCenter.y);
  return {
    sourceHandle,
    targetHandle: oppositeFlowSide(sourceHandle),
  };
}

function personalOrbitRadius(nodes: OpenBrainFlowNode[], options: { demoMode?: boolean }): number {
  if (nodes.length === 0) {
    return 0;
  }
  const minRadius = options.demoMode ? PERSONAL_DEMO_ORBIT_MIN_RADIUS : PERSONAL_REAL_ORBIT_MIN_RADIUS;
  const padding = options.demoMode ? PERSONAL_DEMO_ORBIT_PADDING : PERSONAL_REAL_ORBIT_PADDING;
  const largestOrbitNode = Math.max(...nodes.map((node) => {
    const size = nodeSize(node);
    return Math.max(size.width, size.height);
  }));
  const chordTarget = largestOrbitNode + PERSONAL_ORBIT_NODE_GAP;
  const chordRadius = nodes.length > 1
    ? chordTarget / (2 * Math.sin(Math.PI / nodes.length))
    : 0;
  const centerClearance = USER_NODE_SIZE / 2 + largestOrbitNode / 2 + padding;
  return Math.ceil(Math.max(minRadius, chordRadius, centerClearance));
}

function layoutPersonalOrbit(nodes: OpenBrainFlowNode[], options: { demoMode?: boolean }): OpenBrainFlowNode[] {
  const root = nodes.find((node) => node.id === 'user-root');
  if (!root) {
    return nodes;
  }
  const orbitNodes = nodes.filter((node) => node.id !== root.id);
  if (orbitNodes.length === 0) {
    return nodes.map((node) => (node.id === root.id ? { ...node, position: { x: 0, y: 0 } } : node));
  }

  const radius = personalOrbitRadius(orbitNodes, options);
  const maxOrbitWidth = Math.max(...orbitNodes.map((node) => nodeSize(node).width));
  const maxOrbitHeight = Math.max(...orbitNodes.map((node) => nodeSize(node).height));
  const padding = options.demoMode ? PERSONAL_DEMO_ORBIT_PADDING : PERSONAL_REAL_ORBIT_PADDING;
  const center = {
    x: Math.round(radius + maxOrbitWidth / 2 + padding),
    y: Math.round(radius + maxOrbitHeight / 2 + padding),
  };
  const rootSize = nodeSize(root);
  let orbitIndex = 0;

  return nodes.map((node) => {
    const size = nodeSize(node);
    if (node.id === root.id) {
      return {
        ...node,
        position: {
          x: Math.round(center.x - rootSize.width / 2),
          y: Math.round(center.y - rootSize.height / 2),
        },
      };
    }

    const angle = PERSONAL_ORBIT_START_ANGLE + (Math.PI * 2 * orbitIndex) / orbitNodes.length;
    orbitIndex += 1;
    return {
      ...node,
      position: {
        x: Math.round(center.x + Math.cos(angle) * radius - size.width / 2),
        y: Math.round(center.y + Math.sin(angle) * radius - size.height / 2),
      },
    };
  });
}

function layoutDagre(
  nodes: OpenBrainFlowNode[],
  edges: OpenBrainFlowEdge[],
  direction: 'LR' | 'TB',
  options: { ranksep: number; nodesep: number },
): OpenBrainFlowNode[] {
  const graph = new dagre.graphlib.Graph();
  graph.setDefaultEdgeLabel(() => ({}));
  graph.setGraph({
    rankdir: direction,
    ranksep: options.ranksep,
    nodesep: options.nodesep,
    marginx: 0,
    marginy: 0,
  });
  nodes.forEach((node) => {
    const size = nodeSize(node);
    graph.setNode(node.id, size);
  });
  edges.forEach((edge) => {
    graph.setEdge(edge.source, edge.target);
  });
  dagre.layout(graph);
  return nodes.map((node) => {
    const point = graph.node(node.id);
    const size = nodeSize(node);
    return {
      ...node,
      position: {
        x: Math.round(point.x - size.width / 2),
        y: Math.round(point.y - size.height / 2),
      },
    };
  });
}

function buildPersonalGraph(
  brains: OpenBrainFlowWorkspace[],
  options: { demoMode?: boolean; publicBrains?: OpenBrainFlowPublicBrain[] },
): { nodes: OpenBrainFlowNode[]; edges: OpenBrainFlowEdge[] } {
  const nodes: OpenBrainFlowNode[] = [
    makeNode('user-root', {
      kind: 'user',
      label: 'MyGBrain',
      subtitle: 'personal brain',
      accentColor: DEMO_GRAPH_COLORS.green,
      accentColor2: DEMO_NODE_COLORS['team:brain'].color2,
      accentRgb: DEMO_NODE_COLORS['team:brain'].rgb,
      linked: true,
    }),
  ];
  const addPeer = (
    id: string,
    label: string,
    subtitle: string,
    initial: string,
    colorKey: string,
    defaultLinked: boolean,
    ownerUID?: string,
    avatar?: string,
    publicBrainSources?: OpenBrainFlowPublicBrainSource[],
    username?: string,
  ) => {
    const color = colorForDemoKey(colorKey);
    nodes.push(makeNode(id, {
      kind: 'peer',
      label,
      subtitle,
      initial,
      colorKey,
      ownerUID,
      username,
      avatar,
      publicBrainSources,
      defaultLinked,
      accentColor: color.color,
      accentColor2: color.color2,
      accentRgb: color.rgb,
    }));
  };
  const addSource = (
    id: string,
    label: string,
    subtitle: string,
    initial: string,
    colorKey: string,
    defaultLinked: boolean,
    workspace?: OpenBrainFlowWorkspace,
  ) => {
    const color = colorForDemoKey(colorKey);
    nodes.push(makeNode(id, {
      kind: 'source',
      label,
      subtitle,
      initial,
      sourceID: workspace?.sourceID,
      workspaceID: workspace?.workspaceID,
      orgID: workspace?.orgID,
      path: workspace?.path,
      instanceID: workspace?.instanceID,
      runtimeReachable: workspace?.runtimeReachable,
      publicAccess: workspace?.publicAccess,
      effectivePermission: workspace?.effectivePermission,
      canMutateSource: workspace?.canMutateSource,
      publicOwnerUID: workspace?.publicOwnerUID,
      bindingMode: workspace?.bindingMode,
      defaultLinked,
      accentColor: color.color,
      accentColor2: color.color2,
      accentRgb: color.rgb,
    }));
  };

  if (options.demoMode) {
    DEMO_ORBIT_NODES.forEach((node) => {
      if (node.kind === 'peer') {
        addPeer(node.id, node.label, node.subtitle, node.initial, node.colorKey, node.defaultLinked);
      } else {
        addSource(`workspace:${node.id}`, node.label, node.subtitle, node.initial, node.colorKey, node.defaultLinked);
      }
    });
  } else {
    dedupeFlowWorkspaces(brains).forEach((workspace, index) => {
      const label = workspaceLabel(workspace);
      addSource(
        workspaceNodeKey(workspace),
        label,
        'Connected',
        workspaceInitial(label),
        `source:${workspace.workspaceID || workspace.path || workspace.name || index}`,
        true,
        workspace,
      );
    });
    (options.publicBrains || []).forEach((brain) => {
      addPeer(
        `public:${brain.ownerUID}`,
        brain.name,
        `@${brain.username} · ${brain.activeSourceCount ?? 0} public sources`,
        brain.ownerInitial || workspaceInitial(brain.name),
        brain.colorKey,
        true,
        brain.ownerUID,
        brain.avatar,
        brain.sources,
        brain.username,
      );
    });
  }

  const layoutedNodes = layoutPersonalOrbit(nodes, { demoMode: options.demoMode });
  const root = layoutedNodes.find((node) => node.id === 'user-root');
  const edges: OpenBrainFlowEdge[] = root
    ? layoutedNodes
      .filter((node) => node.id !== root.id && (node.data.kind === 'peer' || node.data.kind === 'source'))
      .map((node) => {
        const kind = node.data.kind === 'peer' ? 'peer' : 'source';
        return makeEdge(
          `${kind}:${node.id}`,
          root.id,
          node.id,
          { kind, color: node.data.accentColor || DEMO_GRAPH_COLORS.green, floating: true },
          personalEdgeHandles(root, node),
        );
      })
    : [];

  return {
    nodes: layoutedNodes,
    edges,
  };
}

function buildTeamGraph(): { nodes: OpenBrainFlowNode[]; edges: OpenBrainFlowEdge[]; teams: OpenBrainTeam[] } {
  const teamColors = CLOUD_TEAM_INPUTS.map((team) => colorForDemoKey(`peer:${team.id}`));
  const teams: OpenBrainTeam[] = CLOUD_TEAM_INPUTS.map((team, teamIndex) => {
    const teamColor = teamColors[teamIndex] || DEMO_NODE_PALETTE[teamIndex % DEMO_NODE_PALETTE.length] || DEMO_NODE_PALETTE[0];
    return {
      id: team.id,
      label: team.label,
      defaultLinked: team.defaultLinked,
      members: team.members.map((member) => {
        const memberColor = colorForDemoKey(`person:${member.id}`);
        return {
          ...member,
          teamID: team.id,
          accentColor: memberColor.color,
          accentColor2: memberColor.color2,
          accentRgb: memberColor.rgb,
        };
      }),
      accentColor: teamColor.color,
    };
  });
  const nodes: OpenBrainFlowNode[] = [
    makeNode('enterprise', {
      kind: 'companyRoot',
      label: "OpenBrain's Brain",
      subtitle: 'Shared brain for teams',
      defaultLinked: false,
      accentColor: DEMO_NODE_COLORS['team:brain'].color,
      accentColor2: DEMO_NODE_COLORS['team:brain'].color2,
      accentRgb: DEMO_NODE_COLORS['team:brain'].rgb,
    }),
    makeNode('team-restore', {
      kind: 'teamRestore',
      label: '+',
      subtitle: "Add OpenBrain's Brain",
      hidden: true,
    }),
  ];
  const edges: OpenBrainFlowEdge[] = [];
  teams.forEach((team) => {
    nodes.push(makeNode(team.id, {
      kind: 'department',
      label: team.label,
      defaultLinked: team.defaultLinked,
      teamMembers: team.members,
      accentColor: colorForDemoKey(`peer:${team.id}`).color,
      accentColor2: colorForDemoKey(`peer:${team.id}`).color2,
      accentRgb: colorForDemoKey(`peer:${team.id}`).rgb,
    }));
    edges.push(makeEdge(`company:${team.id}`, 'enterprise', team.id, { kind: 'hierarchy', color: DEMO_GRAPH_COLORS.green, strong: true }, { sourceHandle: 'bottom', targetHandle: 'top' }));
    team.members.forEach((member) => {
      nodes.push(makeNode(member.id, {
        kind: 'member',
        label: member.label,
        sourceCount: member.sourceCount,
        teamID: team.id,
        defaultLinked: member.defaultLinked,
        accentColor: member.accentColor,
        accentColor2: member.accentColor2,
        accentRgb: member.accentRgb,
      }));
      edges.push(makeEdge(`member:${team.id}:${member.id}`, team.id, member.id, { kind: 'hierarchy', color: member.accentColor }, { sourceHandle: 'bottom', targetHandle: 'top' }));
    });
  });
  return {
    nodes: layoutDagre(nodes.filter((node) => node.id !== 'team-restore'), edges, 'TB', { ranksep: 78, nodesep: 46 }),
    edges,
    teams,
  };
}

export function buildOpenBrainFlow(
  brains: OpenBrainFlowWorkspace[],
  peerLinks: OpenBrainPeerLinkState = DEFAULT_OPENBRAIN_PEER_LINKS,
  options: { demoMode?: boolean; publicBrains?: OpenBrainFlowPublicBrain[]; teamBrainClusterVisible?: boolean } = {},
): OpenBrainFlowLayout {
  const personal = buildPersonalGraph(brains, options);
  const personalBox = bounds(personal.nodes);
  if (!options.demoMode) {
    const personalNodes = offsetNodes(
      personal.nodes,
      GRAPH_LEFT_OFFSET - personalBox.minX,
      GRAPH_LEFT_OFFSET - personalBox.minY,
    );
    const nodes = personalNodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        linked: node.data.kind === 'user'
          ? true
          : node.data.kind === 'peer'
            ? Boolean(peerLinks[node.id] ?? node.data.defaultLinked)
            : node.data.linked,
      },
    }));
    const box = bounds(nodes);
    return {
      nodes,
      edges: personal.edges,
      width: Math.round(box.maxX - box.minX),
      height: Math.round(box.maxY - box.minY),
    };
  }

  const team = buildTeamGraph();
  const teamBox = bounds(team.nodes);
  const teamOffsetX = Math.round(GRAPH_LEFT_OFFSET - teamBox.minX + (personalBox.maxX - personalBox.minX) + CLUSTER_GAP);
  const teamOffsetY = TEAM_GRAPH_TOP_OFFSET - teamBox.minY;
  const teamNodes = offsetNodes(team.nodes, teamOffsetX, teamOffsetY);
  const departmentNodes = teamNodes.filter((node) => node.data.kind === 'department');
  const personalRoot = personal.nodes.find((node) => node.id === 'user-root');
  const personalRootTargetCenterY = departmentNodes.length > 0
    ? departmentNodes.reduce((sum, node) => sum + nodeCenter(node).y, 0) / departmentNodes.length
    : PERSONAL_ROOT_FALLBACK_CENTER_Y;
  const personalOffsetY = personalRoot
    ? Math.round(personalRootTargetCenterY - nodeCenter(personalRoot).y)
    : Math.round(PERSONAL_ROOT_FALLBACK_CENTER_Y - personalBox.minY);
  const personalNodes = offsetNodes(personal.nodes, GRAPH_LEFT_OFFSET - personalBox.minX, personalOffsetY);
  const visibleTeamNodes = teamNodes.map((node) => ({
    ...node,
    data: {
      ...node.data,
      hidden: options.teamBrainClusterVisible === false,
      linked: Boolean(peerLinks[node.id]),
    },
    hidden: options.teamBrainClusterVisible === false,
  }));
  const enterprise = visibleTeamNodes.find((node) => node.id === 'enterprise');
  const restore = makeNode('team-restore', {
    kind: 'teamRestore',
    label: '+',
    subtitle: "Add OpenBrain's Brain",
    hidden: options.teamBrainClusterVisible !== false,
  });
  restore.position = enterprise
    ? {
      x: Math.round(enterprise.position.x + COMPANY_ROOT_SIZE / 2 - RESTORE_NODE_SIZE / 2),
      y: Math.round(enterprise.position.y + COMPANY_ROOT_SIZE / 2 - RESTORE_NODE_SIZE / 2),
    }
    : { x: teamOffsetX, y: teamOffsetY };
  restore.hidden = options.teamBrainClusterVisible !== false;
  const peerEdges = (options.teamBrainClusterVisible === false ? [] : visibleTeamNodes)
    .filter((node) => TEAM_BRAIN_CLUSTER_PEER_IDS.has(node.id) && Boolean(peerLinks[node.id]))
    .map((node) => makeEdge(
      `peer:${node.id}`,
      'user-root',
      node.id,
      { kind: 'peer', color: node.data.accentColor || DEMO_GRAPH_COLORS.green, floating: true },
      { sourceHandle: 'right', targetHandle: 'left' },
    ));
  const teamEdges = team.edges.map((edge) => ({
    ...edge,
    hidden: options.teamBrainClusterVisible === false,
  }));
  const nodes = [
    ...personalNodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        linked: node.data.kind === 'user'
          ? true
          : node.data.kind === 'peer'
            ? Boolean(peerLinks[node.id] ?? node.data.defaultLinked)
            : node.data.linked,
      },
    })),
    ...visibleTeamNodes,
    restore,
  ];
  const allEdges = [
    ...personal.edges,
    ...teamEdges,
    ...peerEdges,
  ];
  const box = bounds(nodes.filter((node) => !node.hidden));
  return {
    nodes,
    edges: allEdges,
    width: Math.round(box.maxX - box.minX),
    height: Math.round(box.maxY - box.minY),
  };
}
