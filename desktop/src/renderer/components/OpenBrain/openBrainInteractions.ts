/**
 * OpenBrain graph click semantics — reconciles 脑图 PRD with source权限-个人共享 PRD.
 *
 * - MyGBrain avatar: single click opens GBrain chat; right-click opens Add popover.
 * - Source pill: single click opens source-scoped GBrain chat; right-click opens source chat and management.
 * - Public brain: single click opens public-brain-scoped GBrain chat; right-click opens public brain chat and management.
 * - Team/demo nodes: single click opens GBrain chat; right-click controls graph management.
 */

export const OPENBRAIN_GRAPH_CLICK = {
  centerSingle: 'open-chat',
  centerContext: 'add-popover',
  sourceSingle: 'open-chat-source-scope',
  sourceContext: 'source-management',
  peerSingle: 'open-chat-public-brain-scope',
  peerContext: 'public-brain-management',
  clusterSingle: 'open-chat',
  clusterContext: 'graph-management',
} as const;
