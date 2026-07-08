import React, { memo, useEffect } from 'react';
import {
  Background,
  BaseEdge,
  Handle,
  Position,
  ReactFlow,
  ReactFlowProvider,
  getBezierPath,
  getNodesBounds,
  getViewportForBounds,
  useReactFlow,
  useStore,
  type EdgeProps,
  type InternalNode,
  type NodeProps,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { OpenBrainLogo } from '../Icons';
import { OPENBRAIN_GRAPH_CAPSULE, OP_SG_CAPSULE, OP_SG_CAPSULE_ON_EDITOR } from '../staticGlassCapsule';
import { TEAM_BRAIN_CLUSTER_MEMBER_COUNT, type OpenBrainFlowEdge, type OpenBrainFlowNode, type OpenBrainFlowNodeData } from './openBrainFlow';

export type OpenBrainFlowNodeAction = (
  node: OpenBrainFlowNodeData & { id: string },
  event: React.MouseEvent<Element>,
) => void;

export type OpenBrainFlowSourceContextMenuEvent =
  | React.MouseEvent<Element>
  | React.KeyboardEvent<Element>;

export type OpenBrainFlowSourceContextMenu = (
  node: OpenBrainFlowNodeData & { id: string },
  event: OpenBrainFlowSourceContextMenuEvent,
) => void;

type GraphNodeData = OpenBrainFlowNodeData & {
  coreRef?: React.Ref<HTMLButtonElement>;
  onNodeAction?: OpenBrainFlowNodeAction;
  onNodeContextMenu?: OpenBrainFlowNodeAction;
  onSourceContextMenu?: OpenBrainFlowSourceContextMenu;
  sourceContextMenuEnabled?: boolean;
  sourceContextMenuDisabled?: boolean;
  sourceContextMenuOpen?: boolean;
  onRemoveTeamBrainCluster?: (event: React.MouseEvent<HTMLButtonElement>) => void;
  onRestoreTeamBrainCluster?: () => void;
};

export type OpenBrainRenderedFlowNode = OpenBrainFlowNode & {
  data: GraphNodeData;
};

type OpenBrainFlowGraphProps = {
  nodes: OpenBrainRenderedFlowNode[];
  edges: OpenBrainFlowEdge[];
  graphSignature: string;
  /** When false, graph pan/zoom and node hit targets are disabled (e.g. onboarding overlay). */
  interactive?: boolean;
};

type Point = {
  x: number;
  y: number;
};

type NodeBox = Point & {
  width: number;
  height: number;
};

type InternalOpenBrainNode = InternalNode<OpenBrainRenderedFlowNode>;

const FLOATING_EDGE_MAX_BEND = 38;
const FLOATING_EDGE_MIN_BEND = 10;
const FLOW_FIT_PADDING = 0.16;
const FLOW_FIT_TOP_PADDING_RATIO = 0.08;
const USER_INNER_RING_INSET = 28;

// Remembered across OpenBrainFlowGraph remounts (navigating away and back to the
// OpenBrain page) so we can restore the previous viewport without re-fitting.
let lastFitSignature: string | null = null;
let lastFitViewport: { x: number; y: number; zoom: number } | null = null;

function memberSourceLabel(count: number): string {
  return `${count} source${count === 1 ? '' : 's'}`;
}

function teamMemberLabel(data: GraphNodeData): string {
  const members = data.teamMembers || [];
  const linkedCount = members.filter((member) => member.defaultLinked).length;
  if (data.linked) {
    return `${members.length} members · team linked to you`;
  }
  return `${members.length} members · ${linkedCount} linked to you`;
}

function hiddenHandle(type: 'source' | 'target', position: Position, id: string) {
  return (
    <Handle
      id={id}
      type={type}
      position={position}
      isConnectable={false}
      className="openbrain-flow-handle"
    />
  );
}

function hiddenDirectionalHandles(type: 'source' | 'target') {
  return (
    <>
      {hiddenHandle(type, Position.Top, 'top')}
      {hiddenHandle(type, Position.Right, 'right')}
      {hiddenHandle(type, Position.Bottom, 'bottom')}
      {hiddenHandle(type, Position.Left, 'left')}
    </>
  );
}

function nodeTitle(data: GraphNodeData): string {
  return [data.label, data.subtitle].filter(Boolean).join(' - ');
}

function openSourceContextMenu(
  node: GraphNodeData & { id: string },
  event: OpenBrainFlowSourceContextMenuEvent,
) {
  event.preventDefault();
  event.stopPropagation();
  node.onSourceContextMenu?.(node, event);
}

function SourceGraphNode({ id, data }: { id: string; data: GraphNodeData }) {
  const node = { ...data, id };
  const sourceContextMenuEnabled = Boolean(data.sourceContextMenuEnabled && data.onSourceContextMenu);
  const menuDisabled = Boolean(data.sourceContextMenuDisabled);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    data.onNodeAction?.(node, event);
  };

  const openMenu = (event: React.MouseEvent<HTMLButtonElement>) => {
    if (menuDisabled) {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    openSourceContextMenu(node, event);
  };

  return (
    <div className="openbrain-flow-node openbrain-flow-source no-drag">
      {hiddenDirectionalHandles('target')}
      <button
        type="button"
        className={`openbrain-source-node openbrain-flow-node-button nodrag nopan ${OPENBRAIN_GRAPH_CAPSULE} flex cursor-pointer items-center text-left outline-none${data.publicAccess ? ' openbrain-source-public' : ''}${data.linked ? '' : ' openbrain-source-unlinked'}${data.sourceContextMenuOpen ? ' is-menu-open' : ''}`}
        aria-pressed={Boolean(data.linked)}
        aria-label={nodeTitle(data)}
        title={nodeTitle(data)}
        onClick={handleClick}
        onContextMenu={sourceContextMenuEnabled ? openMenu : undefined}
      >
        <span
          className="openbrain-source-dot"
          style={{ background: data.accentColor }}
        >
          {data.initial}
        </span>
        <span className="openbrain-source-copy min-w-0 flex-1">
          <span className="block truncate text-xs font-bold leading-4 tracking-[-0.01em]">{data.label}</span>
          {data.subtitle ? (
            <span className="openbrain-source-subtitle block truncate text-[8px] leading-[10px]">{data.subtitle}</span>
          ) : null}
        </span>
      </button>
    </div>
  );
}

function PeerBrainDot({ data }: { data: GraphNodeData }) {
  const avatar = (data.avatar || '').trim();
  const [imageFailed, setImageFailed] = React.useState(false);

  useEffect(() => {
    setImageFailed(false);
  }, [avatar]);

  return (
    <span
      className="openbrain-peer-brain-dot"
      style={{ background: data.accentColor }}
    >
      {avatar && !imageFailed ? (
        <img
          src={avatar}
          alt=""
          className="openbrain-peer-brain-img"
          draggable={false}
          referrerPolicy="no-referrer"
          onError={() => setImageFailed(true)}
        />
      ) : (
        <span className="openbrain-peer-brain-initial">{data.initial}</span>
      )}
    </span>
  );
}

function GraphNode({ id, data }: NodeProps<OpenBrainRenderedFlowNode>) {
  const node = { ...data, id };
  if (data.kind === 'user') {
    return (
      <div className="openbrain-flow-node openbrain-flow-user no-drag">
        {hiddenDirectionalHandles('source')}
        <div className="openbrain-personal-orbit" aria-hidden="true">
          <span className="openbrain-personal-ring openbrain-personal-ring--outer" />
          <span className="openbrain-personal-ring openbrain-personal-ring--inner" />
        </div>
        <button
          type="button"
          ref={data.coreRef}
          className="openbrain-core-node nodrag nopan grid cursor-pointer place-items-center border-0 bg-transparent p-0 text-center outline-none"
          aria-label={`${data.label}. Click to open chat. Right-click to add sources or public brains.`}
          title="Click to open chat · Right-click to add sources or public brains"
          onClick={(event) => data.onNodeAction?.(node, event)}
          onContextMenu={(event) => data.onNodeContextMenu?.(node, event)}
        >
          <span className="openbrain-avatar-wrap">
            <span className={`openbrain-avatar ${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR}`}>{data.label}</span>
          </span>
        </button>
      </div>
    );
  }

  if (data.kind === 'source') {
    return <SourceGraphNode id={id} data={data} />;
  }

  if (data.kind === 'peer') {
    return (
      <div className="openbrain-flow-node openbrain-flow-peer no-drag">
        {hiddenDirectionalHandles('target')}
        <button
          type="button"
          className={`openbrain-peer-brain-node openbrain-flow-node-button nodrag nopan grid cursor-pointer place-items-center border p-0 text-center outline-none${data.linked ? ' openbrain-peer-linked' : ''}`}
          aria-pressed={Boolean(data.linked)}
          title={nodeTitle(data)}
          onClick={(event) => data.onNodeAction?.(node, event)}
          onContextMenu={(event) => data.onNodeContextMenu?.(node, event)}
        >
          <PeerBrainDot data={data} />
          <span className="openbrain-peer-brain-copy">
            <strong style={{ color: data.accentColor2 || data.accentColor }}>{data.label}</strong>
            <span>{data.subtitle}</span>
          </span>
        </button>
      </div>
    );
  }

  if (data.kind === 'companyRoot') {
    return (
      <div className="openbrain-flow-company-root">
        {hiddenHandle('target', Position.Left, 'left')}
        {hiddenHandle('source', Position.Bottom, 'bottom')}
        <div className="openbrain-cluster-enterprise-copy" aria-hidden="true">
          <h2>{data.label}</h2>
          <p>{data.subtitle}</p>
        </div>
        <button
          type="button"
          className={`openbrain-cluster-enterprise ${OPENBRAIN_GRAPH_CAPSULE} cursor-pointer p-0 outline-none${data.linked ? ' openbrain-peer-linked' : ''}`}
          aria-pressed={Boolean(data.linked)}
          aria-label={nodeTitle(data)}
          onClick={(event) => data.onNodeAction?.(node, event)}
          onContextMenu={(event) => data.onNodeContextMenu?.(node, event)}
        >
          <OpenBrainLogo className="h-[52px] w-[52px]" />
        </button>
        <div className={`openbrain-cluster-enterprise-panel ${OPENBRAIN_GRAPH_CAPSULE}`} role="tooltip">
          <span className="openbrain-cluster-enterprise-count">
            {TEAM_BRAIN_CLUSTER_MEMBER_COUNT} members
          </span>
          <button
            type="button"
            className={`openbrain-cluster-delete-btn ${OPENBRAIN_GRAPH_CAPSULE} cursor-pointer outline-none`}
            onClick={data.onRemoveTeamBrainCluster}
          >
            Remove
          </button>
        </div>
      </div>
    );
  }

  if (data.kind === 'department') {
    return (
      <div className="openbrain-flow-node openbrain-flow-department">
        {hiddenHandle('target', Position.Top, 'top')}
        {hiddenHandle('target', Position.Left, 'left')}
        {hiddenHandle('source', Position.Bottom, 'bottom')}
        <button
          type="button"
          className={`openbrain-cluster-team openbrain-flow-node-button ${OPENBRAIN_GRAPH_CAPSULE} cursor-pointer outline-none${data.linked ? ' openbrain-peer-linked' : ''}`}
          aria-pressed={Boolean(data.linked)}
          aria-label={nodeTitle(data)}
          onClick={(event) => data.onNodeAction?.(node, event)}
          onContextMenu={(event) => data.onNodeContextMenu?.(node, event)}
        >
          <strong>{data.label}</strong>
          <span className="openbrain-cluster-team-meta">{teamMemberLabel(data)}</span>
        </button>
      </div>
    );
  }

  if (data.kind === 'member') {
    const memberBackground = `linear-gradient(135deg, ${data.accentColor}, ${data.accentColor2})`;
    const memberShadow = data.linked
      ? `0 0 0 3px rgba(${data.accentRgb}, 0.22), 0 10px 24px rgba(${data.accentRgb}, 0.24)`
      : `0 8px 20px rgba(${data.accentRgb}, 0.18)`;
    return (
      <div className="openbrain-cluster-person-wrap openbrain-flow-node">
        {hiddenHandle('target', Position.Top, 'top')}
        {hiddenHandle('target', Position.Left, 'left')}
        <span className={`openbrain-cluster-person-tip ${OPENBRAIN_GRAPH_CAPSULE}`}>{memberSourceLabel(data.sourceCount || 0)}</span>
        <button
          type="button"
          className={`openbrain-cluster-person absolute inset-0 cursor-pointer p-0 outline-none${data.linked ? ' openbrain-peer-linked' : ''}`}
          style={{
            background: memberBackground,
            boxShadow: memberShadow,
          }}
          aria-pressed={Boolean(data.linked)}
          aria-label={nodeTitle(data)}
          onClick={(event) => data.onNodeAction?.(node, event)}
          onContextMenu={(event) => data.onNodeContextMenu?.(node, event)}
        >
          {data.label}
        </button>
      </div>
    );
  }

  return (
    <button
      type="button"
      className={`openbrain-cluster-restore-btn ${OPENBRAIN_GRAPH_CAPSULE} openbrain-flow-node cursor-pointer outline-none`}
      aria-label="Add OpenBrain's Brain back to the map"
      title="Add OpenBrain's Brain"
      onClick={() => data.onRestoreTeamBrainCluster?.()}
    >
      +
    </button>
  );
}

function internalNodeBox(node: InternalOpenBrainNode): NodeBox | null {
  const width = node.measured.width ?? node.width ?? node.internals.userNode.width ?? 0;
  const height = node.measured.height ?? node.height ?? node.internals.userNode.height ?? 0;
  if (width <= 0 || height <= 0) {
    return null;
  }
  return {
    x: node.internals.positionAbsolute.x,
    y: node.internals.positionAbsolute.y,
    width,
    height,
  };
}

function boxCenter(box: NodeBox): Point {
  return {
    x: box.x + box.width / 2,
    y: box.y + box.height / 2,
  };
}

function positionForVector(dx: number, dy: number): Position {
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0 ? Position.Right : Position.Left;
  }
  return dy >= 0 ? Position.Bottom : Position.Top;
}

function pointOnCircle(box: NodeBox, toward: Point, inset = 0): Point {
  const center = boxCenter(box);
  const dx = toward.x - center.x;
  const dy = toward.y - center.y;
  const length = Math.hypot(dx, dy) || 1;
  const radius = Math.max(0, Math.min(box.width, box.height) / 2 - inset);
  return {
    x: center.x + (dx / length) * radius,
    y: center.y + (dy / length) * radius,
  };
}

function pointOnRect(box: NodeBox, toward: Point): Point {
  const center = boxCenter(box);
  const dx = toward.x - center.x;
  const dy = toward.y - center.y;
  if (dx === 0 && dy === 0) {
    return center;
  }
  const scaleX = dx === 0 ? Number.POSITIVE_INFINITY : (box.width / 2) / Math.abs(dx);
  const scaleY = dy === 0 ? Number.POSITIVE_INFINITY : (box.height / 2) / Math.abs(dy);
  const scale = Math.min(scaleX, scaleY);
  return {
    x: center.x + dx * scale,
    y: center.y + dy * scale,
  };
}

function floatingBoundaryPoint(node: InternalOpenBrainNode, box: NodeBox, toward: Point): Point {
  const kind = node.internals.userNode.data.kind;
  if (kind === 'user') {
    return pointOnCircle(box, toward, USER_INNER_RING_INSET);
  }
  if (kind === 'peer' || kind === 'companyRoot' || kind === 'member' || kind === 'teamRestore') {
    return pointOnCircle(box, toward);
  }
  return pointOnRect(box, toward);
}

function floatingEdgeParams(sourceNode: InternalOpenBrainNode, targetNode: InternalOpenBrainNode) {
  const sourceBox = internalNodeBox(sourceNode);
  const targetBox = internalNodeBox(targetNode);
  if (!sourceBox || !targetBox) {
    return null;
  }
  const sourceCenter = boxCenter(sourceBox);
  const targetCenter = boxCenter(targetBox);
  const sourcePoint = floatingBoundaryPoint(sourceNode, sourceBox, targetCenter);
  const targetPoint = floatingBoundaryPoint(targetNode, targetBox, sourceCenter);
  return {
    sourceX: sourcePoint.x,
    sourceY: sourcePoint.y,
    sourcePosition: positionForVector(targetCenter.x - sourceCenter.x, targetCenter.y - sourceCenter.y),
    targetX: targetPoint.x,
    targetY: targetPoint.y,
    targetPosition: positionForVector(sourceCenter.x - targetCenter.x, sourceCenter.y - targetCenter.y),
  };
}

function coordinate(value: number): string {
  return (Math.round(value * 100) / 100).toString();
}

function floatingSingleBendPath(params: {
  sourceX: number;
  sourceY: number;
  targetX: number;
  targetY: number;
}): string {
  const { sourceX, sourceY, targetX, targetY } = params;
  const dx = targetX - sourceX;
  const dy = targetY - sourceY;
  const distance = Math.hypot(dx, dy) || 1;
  const bend = Math.min(FLOATING_EDGE_MAX_BEND, Math.max(FLOATING_EDGE_MIN_BEND, distance * 0.12));
  const sign = dx * dy >= 0 ? 1 : -1;
  const controlX = (sourceX + targetX) / 2 + (-dy / distance) * bend * sign;
  const controlY = (sourceY + targetY) / 2 + (dx / distance) * bend * sign;
  return [
    'M',
    coordinate(sourceX),
    coordinate(sourceY),
    'Q',
    coordinate(controlX),
    coordinate(controlY),
    coordinate(targetX),
    coordinate(targetY),
  ].join(' ');
}

function SoftEdge(props: EdgeProps<OpenBrainFlowEdge>) {
  const {
    id,
    source,
    target,
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    data,
  } = props;
  const { sourceNode, targetNode } = useStore((store) => ({
    sourceNode: store.nodeLookup.get(source) as InternalOpenBrainNode | undefined,
    targetNode: store.nodeLookup.get(target) as InternalOpenBrainNode | undefined,
  }));
  const floatingParams = data?.floating && sourceNode && targetNode
    ? floatingEdgeParams(sourceNode, targetNode)
    : null;
  const path = floatingParams
    ? floatingSingleBendPath(floatingParams)
    : getBezierPath({
      sourceX,
      sourceY,
      sourcePosition,
      targetX,
      targetY,
      targetPosition,
      curvature: 0.24,
    })[0];
  const color = data?.color || '#2f8f6b';
  return (
    <>
      <BaseEdge id={`${id}-glow`} path={path} style={{ stroke: color, strokeOpacity: 0.1, strokeWidth: data?.strong ? 8 : 7 }} />
      <BaseEdge id={id} path={path} style={{ stroke: color, strokeOpacity: data?.strong ? 0.52 : 0.42, strokeWidth: data?.strong ? 2.4 : 2 }} />
      <path className="openbrain-flow-edge-pulse" d={path} stroke={color} strokeOpacity={0.38} />
    </>
  );
}

const nodeTypes = {
  openbrainNode: memo(GraphNode),
};

const edgeTypes = {
  openbrainSoft: memo(SoftEdge),
};

function BiasedFitOnChange({ graphSignature }: { graphSignature: string }) {
  const { getNodes, setViewport, getViewport } = useReactFlow<OpenBrainRenderedFlowNode, OpenBrainFlowEdge>();
  const width = useStore((state) => state.width);
  const height = useStore((state) => state.height);
  const minZoom = useStore((state) => state.minZoom);
  const maxZoom = useStore((state) => state.maxZoom);
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      const visibleNodes = getNodes().filter((node) => !node.hidden);
      if (visibleNodes.length === 0 || width <= 0 || height <= 0) {
        return;
      }
      // Reopening the same graph: restore the prior viewport instantly instead of
      // re-running the 180ms fit animation every time the page is shown.
      if (graphSignature === lastFitSignature && lastFitViewport) {
        void setViewport(lastFitViewport, { duration: 0 });
        return;
      }
      const bounds = getNodesBounds(visibleNodes);
      const viewport = getViewportForBounds(bounds, width, height, minZoom, maxZoom, FLOW_FIT_PADDING);
      const currentTop = bounds.y * viewport.zoom + viewport.y;
      const targetTop = Math.round(height * FLOW_FIT_TOP_PADDING_RATIO);
      const nextViewport = { ...viewport, y: viewport.y + targetTop - currentTop };
      lastFitSignature = graphSignature;
      lastFitViewport = nextViewport;
      void setViewport(nextViewport, { duration: 180 });
    });
    return () => {
      window.cancelAnimationFrame(frame);
      // Capture the user's current pan/zoom so a remount with the same graph lays
      // out exactly where they left it rather than the original fit target.
      if (graphSignature === lastFitSignature) {
        try {
          lastFitViewport = getViewport();
        } catch {
          // React Flow store already torn down during unmount; keep last fit.
        }
      }
    };
  }, [getNodes, getViewport, graphSignature, height, maxZoom, minZoom, setViewport, width]);
  return null;
}

function OpenBrainFlowInner({ nodes, edges, graphSignature, interactive = true }: OpenBrainFlowGraphProps) {
  const handleNodeClick = (
    event: React.MouseEvent,
    node: OpenBrainRenderedFlowNode,
  ) => {
    if (node.data.kind !== 'source' || !node.data.onNodeAction) {
      return;
    }
    node.data.onNodeAction({ ...node.data, id: node.id }, event);
  };

  const handleNodeContextMenu = (
    event: React.MouseEvent,
    node: OpenBrainRenderedFlowNode,
  ) => {
    if (node.data.kind !== 'source' || !node.data.sourceContextMenuEnabled || !node.data.onSourceContextMenu) {
      return;
    }
    openSourceContextMenu({ ...node.data, id: node.id }, event);
  };

  return (
    <ReactFlow<OpenBrainRenderedFlowNode, OpenBrainFlowEdge>
      className={`openbrain-flow openbrain-stage-graph${interactive ? '' : ' openbrain-flow-locked'}`}
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      edgeTypes={edgeTypes}
      minZoom={0.42}
      maxZoom={1.2}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable={false}
      zoomOnDoubleClick={false}
      panOnDrag={interactive}
      panOnScroll={interactive}
      preventScrolling={false}
      onNodeClick={handleNodeClick}
      onNodeContextMenu={handleNodeContextMenu}
      proOptions={{ hideAttribution: true }}
    >
      <Background className="openbrain-flow-background" gap={76} size={1} color="rgba(47, 143, 107, 0.08)" />
      <BiasedFitOnChange graphSignature={graphSignature} />
    </ReactFlow>
  );
}

export function OpenBrainFlowGraph(props: OpenBrainFlowGraphProps) {
  return (
    <ReactFlowProvider>
      <OpenBrainFlowInner {...props} />
    </ReactFlowProvider>
  );
}
