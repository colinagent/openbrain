import { forceParsing, syntaxTree, syntaxTreeAvailable } from '@codemirror/language';
import { StateEffect, Transaction, type Extension } from '@codemirror/state';
import { EditorView, ViewPlugin, ViewUpdate } from '@codemirror/view';
import {
  refreshLivePreviewDecorationsEffect,
  refreshLivePreviewViewportDecorationsEffect,
} from './livePreviewDecorationEffects';

const FORCE_PARSE_TIMEOUT_MS = 200;

function collectVisibleRanges(view: EditorView): Array<{ from: number; to: number }> {
  return view.visibleRanges.map((range) => ({ from: range.from, to: range.to }));
}

function getParseTarget(view: EditorView): number {
  const ranges = collectVisibleRanges(view);
  const visibleTo = ranges.reduce((to, range) => Math.max(to, range.to), 0);
  return Math.max(visibleTo, view.viewport.to);
}

function refreshViewportDecorations(view: EditorView, ranges = collectVisibleRanges(view)): void {
  const refreshRanges = ranges.length > 0 ? ranges : collectVisibleRanges(view);
  const effects: StateEffect<unknown>[] = [refreshLivePreviewDecorationsEffect.of(null)];
  if (refreshRanges.length > 0) {
    effects.push(refreshLivePreviewViewportDecorationsEffect.of(refreshRanges));
  }
  view.dispatch({
    effects,
    annotations: Transaction.addToHistory.of(false),
  });
}

export class LivePreviewParseRefreshPlugin {
  private frame = 0;
  private destroyed = false;
  private pendingForceParse = false;

  constructor(private readonly view: EditorView) {
    this.view.scrollDOM.addEventListener('scroll', this.handleScroll, { passive: true });
  }

  update(update: ViewUpdate): void {
    if (syntaxTree(update.state) !== syntaxTree(update.startState)) {
      this.scheduleRefresh(false);
    }

    if (update.viewportChanged) {
      this.scheduleRefresh(true);
    }
  }

  destroy(): void {
    this.destroyed = true;
    this.view.scrollDOM.removeEventListener('scroll', this.handleScroll);
    if (this.frame) {
      cancelAnimationFrame(this.frame);
      this.frame = 0;
    }
  }

  private readonly handleScroll = (): void => {
    this.scheduleRefresh(true);
  };

  private scheduleRefresh(shouldForceParse: boolean): void {
    this.pendingForceParse ||= shouldForceParse;
    if (this.frame) {
      return;
    }
    this.frame = requestAnimationFrame(() => {
      this.frame = 0;
      const forceParse = this.pendingForceParse;
      this.pendingForceParse = false;
      this.view.requestMeasure({
        key: this,
        read: (view) => ({
          parseTo: getParseTarget(view),
          ranges: collectVisibleRanges(view),
        }),
        write: ({ parseTo, ranges }, view) => {
          if (this.destroyed) {
            return;
          }
          if (forceParse && parseTo > 0 && !syntaxTreeAvailable(view.state, parseTo)) {
            forceParsing(view, parseTo, FORCE_PARSE_TIMEOUT_MS);
          }
          refreshViewportDecorations(view, ranges);
        },
      });
    });
  }
}

export function livePreviewParseRefresh(): Extension {
  return ViewPlugin.fromClass(LivePreviewParseRefreshPlugin);
}
