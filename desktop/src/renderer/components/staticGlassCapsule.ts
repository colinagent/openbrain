/**
 * Static Glass class exports — static frost pills/tabs and Activity Panel Done chip.
 *
 * Fill = static frost (milk + noise), not backdrop-filter.
 * Activity Panel shell border is separate from pill border:
 * - Panel outline → lighter mix of `--op-sg-border` on `.op-activity-panel` (via `--op-glass-border`)
 * - `OP_SG_CAPSULE_ON_ACTIVITY_HEADER` sets Done chip substrate only, not shell border
 */
export const OP_SG_CAPSULE = 'op-sg-capsule';
export const OP_SG_FROST_SURFACE = 'op-sg-frost-surface';
/** Popup / context menus — frost fill + glass edge (no backdrop-filter) */
export const OP_POPUP_MENU = 'op-sg-frost-surface op-popup-menu';
export const OP_SG_CAPSULE_ON_SIDEBAR = 'op-sg-capsule--on-sidebar';
export const OP_SG_CAPSULE_ON_TITLEBAR = 'op-sg-capsule--on-titlebar';
export const OP_SG_CAPSULE_ON_EDITOR = 'op-sg-capsule--on-editor';
/** Done/Running status chip — substrate = opaque `--op-sg-panel-solid`; shell is `.op-activity-panel` */
export const OP_SG_CAPSULE_ON_ACTIVITY_HEADER = 'op-sg-capsule--on-activity-header';
export const UI_PILL_BTN_PRIMARY = 'ui-pill-btn-primary';
export const UI_PILL_BTN_SECONDARY = 'ui-pill-btn-secondary';
/** Compact pill — inline adjunct; width >= 2x height. */
export const UI_PILL_BTN_COMPACT = 'ui-pill-btn--compact';
/** Dialog footer paired CTAs — 112px min-width. */
export const UI_PILL_BTN_DIALOG = 'ui-pill-btn--dialog';
/** Content-width pills — no min-width floor (file-tree .agent, status chips). */
export const UI_PILL_BTN_FIT = 'ui-pill-btn--fit';
/** OpenBrain graph pills on editor canvas */
export const OPENBRAIN_GRAPH_CAPSULE = `${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR} openbrain-graph-capsule`;
