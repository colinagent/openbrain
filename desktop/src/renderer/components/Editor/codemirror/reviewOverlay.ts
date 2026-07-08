import { EditorState, StateEffect, StateField, type Extension, type Range } from '@codemirror/state';
import { Decoration, EditorView, ViewPlugin, type DecorationSet, type ViewUpdate, WidgetType } from '@codemirror/view';
import { tableSourceBlockField } from './tableSourceState';
import { parseTableBlock } from './utils/tableParser';
import { getBlockStructureIndex } from './utils/blockStructureIndex';
import {
  buildReviewTableDiffForBlock,
  reviewHunkCanTouchTableBlock,
  reviewHunkHasTableRows,
} from './utils/reviewTableDiff';

export type ReviewLineRange = {
  startLine: number;
  endLine: number;
};

export type ReviewHunk = {
  oldStartLine: number;
  oldLineCount: number;
  newStartLine: number;
  newLineCount: number;
  removedLines?: string[];
  addedLines?: string[];
};

export type ReviewOverlay = {
  filePath: string;
  threadID: string;
  turnID: string;
  chatPath: string;
  changedRanges?: ReviewLineRange[] | null;
  hunks?: ReviewHunk[] | null;
};

export type ReviewOverlayDecision = 'keepFile' | 'undoFile';

export type ReviewOverlayActions = {
  onDecision?: (decision: ReviewOverlayDecision, overlay: ReviewOverlay) => void;
};

export type ReviewOverlayState = {
  overlay: ReviewOverlay;
  actions: ReviewOverlayActions | null;
};

const setReviewOverlayEffect = StateEffect.define<ReviewOverlayState | null>();
const reviewLineDecoration = Decoration.line({ class: 'cm-review-added-line cm-review-new-line' });

function normalizePositiveLine(value: number): number {
  if (!Number.isFinite(value)) {
    return 1;
  }
  return Math.max(1, Math.floor(value));
}

function normalizeCount(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.max(0, Math.floor(value));
}

function normalizeReviewRanges(ranges: ReviewLineRange[] | null | undefined): ReviewLineRange[] {
  if (!ranges || ranges.length === 0) {
    return [];
  }
  const normalized = ranges
    .filter((range) => Number.isFinite(range.startLine) && Number.isFinite(range.endLine))
    .map((range) => ({
      startLine: Math.max(1, Math.floor(Math.min(range.startLine, range.endLine))),
      endLine: Math.max(1, Math.floor(Math.max(range.startLine, range.endLine))),
    }))
    .sort((a, b) => a.startLine - b.startLine);

  if (normalized.length === 0) {
    return [];
  }

  const merged: ReviewLineRange[] = [normalized[0]];
  for (let index = 1; index < normalized.length; index += 1) {
    const current = normalized[index];
    const prev = merged[merged.length - 1];
    if (current.startLine <= prev.endLine + 1) {
      prev.endLine = Math.max(prev.endLine, current.endLine);
      continue;
    }
    merged.push(current);
  }
  return merged;
}

export function normalizeReviewHunks(overlay: ReviewOverlay): ReviewHunk[] {
  const rawHunks = overlay.hunks || [];
  const hunks = rawHunks
    .filter((hunk) => Number.isFinite(hunk.newStartLine) || Number.isFinite(hunk.oldStartLine))
    .map((hunk) => ({
      oldStartLine: normalizePositiveLine(hunk.oldStartLine),
      oldLineCount: normalizeCount(hunk.oldLineCount),
      newStartLine: normalizePositiveLine(hunk.newStartLine),
      newLineCount: normalizeCount(hunk.newLineCount),
      removedLines: (hunk.removedLines || []).map((line) => String(line)),
      addedLines: (hunk.addedLines || []).map((line) => String(line)),
    }))
    .filter((hunk) => hunk.oldLineCount > 0 || hunk.newLineCount > 0 || (hunk.removedLines || []).length > 0);

  if (hunks.length > 0) {
    return hunks.sort((a, b) => a.newStartLine - b.newStartLine || a.oldStartLine - b.oldStartLine);
  }

  return normalizeReviewRanges(overlay.changedRanges).map((range) => ({
    oldStartLine: range.startLine,
    oldLineCount: 0,
    newStartLine: range.startLine,
    newLineCount: range.endLine - range.startLine + 1,
    removedLines: [],
    addedLines: [],
  }));
}

function hunkAnchorPos(state: EditorState, hunk: ReviewHunk): number {
  if (state.doc.lines === 0) {
    return 0;
  }
  if (hunk.newStartLine <= state.doc.lines) {
    return state.doc.line(hunk.newStartLine).from;
  }
  return state.doc.length;
}

function appendRemovedTextLine(parent: HTMLElement, lineText: string): void {
  const line = document.createElement('div');
  line.className = 'cm-review-removed-line';
  line.textContent = `- ${lineText}`;
  parent.appendChild(line);
}

function isReviewHunkHandledByTablePreview(state: EditorState, hunk: ReviewHunk): boolean {
  if (!reviewHunkHasTableRows(hunk)) {
    return false;
  }
  const index = getBlockStructureIndex(state);
  const activeSourceTableFrom = state.field(tableSourceBlockField, false) ?? null;
  for (const block of index.tables) {
    if (activeSourceTableFrom === block.from || !reviewHunkCanTouchTableBlock(hunk, block)) {
      continue;
    }
    const parsed = parseTableBlock(state.doc.sliceString(block.from, block.to));
    if (parsed && buildReviewTableDiffForBlock([hunk], parsed, block)) {
      return true;
    }
  }
  return false;
}

function buildReviewDecorations(state: EditorState, value: ReviewOverlayState | null): DecorationSet {
  if (!value || state.doc.lines === 0) {
    return Decoration.none;
  }
  const hunks = normalizeReviewHunks(value.overlay);
  if (hunks.length === 0) {
    return Decoration.none;
  }

  const ranges: Range<Decoration>[] = [];
  const visibleHunks = hunks.filter((hunk) => !isReviewHunkHandledByTablePreview(state, hunk));

  visibleHunks.forEach((hunk, index) => {
    const anchorPos = hunkAnchorPos(state, hunk);
    ranges.push(
      Decoration.widget({
        widget: new ReviewHunkWidget({
          hunk,
          index,
          total: visibleHunks.length,
        }),
        block: true,
        side: -1,
      }).range(anchorPos)
    );
  });

  hunks.forEach((hunk) => {
    if (hunk.newLineCount <= 0) {
      return;
    }
    const startLine = Math.min(hunk.newStartLine, state.doc.lines);
    const endLine = Math.min(hunk.newStartLine + hunk.newLineCount - 1, state.doc.lines);
    for (let lineNumber = startLine; lineNumber <= endLine; lineNumber += 1) {
      const line = state.doc.line(lineNumber);
      ranges.push(reviewLineDecoration.range(line.from));
    }
  });

  return Decoration.set(ranges, true);
}

class ReviewHunkWidget extends WidgetType {
  private readonly hunk: ReviewHunk;
  private readonly index: number;
  private readonly total: number;

  constructor(options: {
    hunk: ReviewHunk;
    index: number;
    total: number;
  }) {
    super();
    this.hunk = options.hunk;
    this.index = options.index;
    this.total = options.total;
  }

  eq(other: ReviewHunkWidget): boolean {
    return this.index === other.index
      && this.total === other.total
      && JSON.stringify(this.hunk) === JSON.stringify(other.hunk);
  }

  ignoreEvent(): boolean {
    return true;
  }

  toDOM(): HTMLElement {
    const root = document.createElement('div');
    root.className = 'cm-review-hunk-widget';

    root.addEventListener('mousedown', (event) => {
      event.preventDefault();
      event.stopPropagation();
    });

    const header = document.createElement('div');
    header.className = 'cm-review-hunk-header';

    const label = document.createElement('span');
    label.className = 'cm-review-hunk-count';
    label.textContent = `${this.index + 1} of ${this.total}`;
    header.appendChild(label);
    root.appendChild(header);

    const removedLines = this.hunk.removedLines || [];
    if (removedLines.length > 0) {
      const removedBlock = document.createElement('div');
      removedBlock.className = 'cm-review-removed-block';
      removedLines.forEach((lineText) => appendRemovedTextLine(removedBlock, lineText));
      root.appendChild(removedBlock);
    }

    return root;
  }
}

export const reviewOverlayField = StateField.define<ReviewOverlayState | null>({
  create() {
    return null;
  },
  update(value, tr) {
    let next = value;
    for (const effect of tr.effects) {
      if (effect.is(setReviewOverlayEffect)) {
        next = effect.value;
      }
    }
    return next;
  },
});

class ReviewFileToolbarPlugin {
  private readonly dom: HTMLElement;
  private readonly undoButton: HTMLButtonElement;
  private readonly keepButton: HTMLButtonElement;
  private value: ReviewOverlayState | null = null;

  constructor(view: EditorView) {
    this.dom = document.createElement('div');
    this.dom.className = 'cm-review-file-toolbar';
    this.dom.addEventListener('mousedown', (event) => {
      event.preventDefault();
      event.stopPropagation();
    });
    this.undoButton = this.createDecisionButton('Undo file', 'undoFile');
    this.keepButton = this.createDecisionButton('Keep file', 'keepFile');
    this.dom.append(this.undoButton, this.keepButton);
    view.dom.appendChild(this.dom);
    this.updateToolbar(view.state.field(reviewOverlayField));
  }

  update(update: ViewUpdate): void {
    this.updateToolbar(update.state.field(reviewOverlayField));
  }

  destroy(): void {
    this.dom.remove();
  }

  private updateToolbar(value: ReviewOverlayState | null): void {
    this.value = value;
    const enabled = Boolean(value?.actions?.onDecision);
    this.dom.classList.toggle('is-visible', Boolean(value));
    this.undoButton.disabled = !enabled;
    this.keepButton.disabled = !enabled;
  }

  private createDecisionButton(
    label: string,
    decision: ReviewOverlayDecision
  ): HTMLButtonElement {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = `cm-review-action cm-review-action-${decision}`;
    button.textContent = label;
    button.addEventListener('click', (event) => {
      event.preventDefault();
      event.stopPropagation();
      if (button.disabled || !this.value?.actions?.onDecision) {
        return;
      }
      this.value.actions.onDecision(decision, this.value.overlay);
    });
    return button;
  }
}

export function reviewOverlayExtensions(): Extension[] {
  return [
    reviewOverlayField,
    ViewPlugin.fromClass(ReviewFileToolbarPlugin),
    EditorView.decorations.compute([reviewOverlayField, tableSourceBlockField], (state) =>
      buildReviewDecorations(state, state.field(reviewOverlayField))
    ),
  ];
}

export function applyReviewOverlay(
  view: EditorView,
  overlay: ReviewOverlay | null | undefined,
  actions: ReviewOverlayActions | null = null
): void {
  view.dispatch({
    effects: setReviewOverlayEffect.of(overlay ? { overlay, actions } : null),
  });
}

export function applyReviewRanges(view: EditorView, ranges: ReviewLineRange[] | null | undefined): void {
  const normalized = normalizeReviewRanges(ranges);
  applyReviewOverlay(view, normalized.length > 0 ? {
    filePath: '',
    threadID: '',
    turnID: '',
    chatPath: '',
    changedRanges: normalized,
    hunks: [],
  } : null);
}
