import assert from 'node:assert/strict';
import test from 'node:test';

import { markdown } from '@codemirror/lang-markdown';
import { ensureSyntaxTree } from '@codemirror/language';
import { EditorState, type TransactionSpec } from '@codemirror/state';
import type { EditorView } from '@codemirror/view';

import {
  refreshLivePreviewDecorationsEffect,
  refreshLivePreviewViewportDecorationsEffect,
} from './livePreviewDecorationEffects';
import { LivePreviewParseRefreshPlugin } from './livePreviewParseRefresh';

type FakeView = EditorView & {
  dispatches: TransactionSpec[];
};

type TestMeasureRequest<T> = {
  read: (view: EditorView) => T;
  write?: (value: T, view: EditorView) => void;
};

function buildLongMarkdownDoc(sectionCount: number): string {
  const lines: string[] = ['# Long document'];
  for (let i = 0; i < sectionCount; i += 1) {
    lines.push('');
    lines.push(`## Section ${i}`);
    lines.push('');
    lines.push(`Paragraph ${i} with **strong** and *emphasis*.`);
  }
  return lines.join('\n');
}

function createFakeView(
  state: EditorState,
  ranges: Array<{ from: number; to: number }>
): FakeView {
  const scrollDOM = new EventTarget();
  const view = {
    dispatches: [] as TransactionSpec[],
    scrollDOM,
    state,
    visibleRanges: ranges,
    viewport: ranges[0] ?? { from: 0, to: 0 },
    dispatch(spec: TransactionSpec) {
      this.dispatches.push(spec);
    },
    requestMeasure<T>(request?: TestMeasureRequest<T>) {
      if (!request) {
        return;
      }
      const value = request.read(this as unknown as EditorView);
      request.write?.(value, this as unknown as EditorView);
    },
  };
  return view as unknown as FakeView;
}

function getEffects(spec: TransactionSpec) {
  if (!spec.effects) {
    return [];
  }
  return Array.isArray(spec.effects) ? spec.effects : [spec.effects];
}

function withAnimationFrame(callback: (flush: () => void) => void): void {
  const originalRequestAnimationFrame = globalThis.requestAnimationFrame;
  const originalCancelAnimationFrame = globalThis.cancelAnimationFrame;
  const frames: FrameRequestCallback[] = [];

  globalThis.requestAnimationFrame = ((frame: FrameRequestCallback) => {
    frames.push(frame);
    return frames.length;
  }) as typeof requestAnimationFrame;
  globalThis.cancelAnimationFrame = (() => undefined) as typeof cancelAnimationFrame;

  try {
    callback(() => {
      const pending = frames.splice(0);
      for (const frame of pending) {
        frame(0);
      }
    });
  } finally {
    globalThis.requestAnimationFrame = originalRequestAnimationFrame;
    globalThis.cancelAnimationFrame = originalCancelAnimationFrame;
  }
}

test('scroll event dispatches live preview refresh effects on next frame', () => {
  const doc = '# Title\n\nParagraph with **strong** text.';
  const state = EditorState.create({ doc, extensions: [markdown()] });
  ensureSyntaxTree(state, doc.length, 1000);
  const ranges = [{ from: 0, to: doc.length }];
  const view = createFakeView(state, ranges);
  const plugin = new LivePreviewParseRefreshPlugin(view);

  withAnimationFrame((flush) => {
    view.scrollDOM.dispatchEvent(new Event('scroll'));
    view.scrollDOM.dispatchEvent(new Event('scroll'));
    assert.equal(view.dispatches.length, 0);

    flush();

    assert.equal(view.dispatches.length, 1);
    const effects = getEffects(view.dispatches[0]);
    assert.ok(effects.some((effect) => effect.is(refreshLivePreviewDecorationsEffect)));
    const viewportEffect = effects.find((effect) => effect.is(refreshLivePreviewViewportDecorationsEffect));
    assert.deepEqual(viewportEffect?.value, ranges);
  });

  plugin.destroy();
});

test('syntax tree change refreshes decorations without forcing another parse step', () => {
  const startState = EditorState.create({
    doc: '# Start',
    extensions: [markdown()],
  });
  const doc = buildLongMarkdownDoc(140);
  const state = EditorState.create({
    doc,
    extensions: [markdown()],
  });
  const target = doc.indexOf('## Section 100');
  const view = createFakeView(state, [{ from: target, to: target + 80 }]);
  const plugin = new LivePreviewParseRefreshPlugin(view);

  withAnimationFrame((flush) => {
    plugin.update({
      startState,
      state,
      viewportChanged: false,
    } as unknown as Parameters<LivePreviewParseRefreshPlugin['update']>[0]);

    flush();

    assert.equal(view.dispatches.length, 1);
    const effects = getEffects(view.dispatches[0]);
    assert.ok(effects.some((effect) => effect.is(refreshLivePreviewDecorationsEffect)));
  });

  plugin.destroy();
});
