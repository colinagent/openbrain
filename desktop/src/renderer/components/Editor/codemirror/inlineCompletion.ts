import {
  EditorState,
  Prec,
  StateEffect,
  StateField,
  type Extension,
} from '@codemirror/state';
import {
  Decoration,
  EditorView,
  keymap,
  ViewPlugin,
  type DecorationSet,
  type ViewUpdate,
  WidgetType,
} from '@codemirror/view';

import type {
  EditorCompletionBlock,
  EditorCompletionResult,
} from '../../../store/appStore';

export type InlineCompletionRequest = {
  requestID: string;
  editorKind: string;
  languageId?: string | null;
  documentPath?: string | null;
  cursorOffset: number;
  prefix: string;
  suffix: string;
  currentBlock: EditorCompletionBlock | null;
  previousBlock: EditorCompletionBlock | null;
  nextBlock: EditorCompletionBlock | null;
  maxOutputTokens: number;
};

export type InlineCompletionOptions = {
  editorKind: string;
  languageId?: string | null;
  documentPath?: string | null;
  enabled: () => boolean;
  request: (
    payload: InlineCompletionRequest,
  ) => Promise<EditorCompletionResult | null>;
  cancel?: (requestID: string) => void;
};

type InlineCompletionState = {
  requestID: string;
  from: number;
  text: string;
  displayText: string;
};

const setInlineCompletionEffect =
  StateEffect.define<InlineCompletionState | null>();

function buildDisplayText(text: string): string {
  const normalized = text.replace(/\r\n/g, '\n');
  const lines = normalized.split('\n');
  const firstLine = lines[0] || '';
  const firstNonEmptyLine = lines.find((line) => line.trim()) || '';
  if (firstLine.trim()) {
    const trimmed = firstLine.trimEnd();
    return lines.length > 1 ? `${trimmed} …` : trimmed;
  }
  if (firstNonEmptyLine) {
    const trimmed = firstNonEmptyLine.trimEnd();
    return `↵ ${trimmed}${lines.length > 1 ? ' …' : ''}`;
  }
  return '…';
}

class InlineCompletionWidget extends WidgetType {
  constructor(private readonly text: string) {
    super();
  }

  eq(other: InlineCompletionWidget): boolean {
    return other.text === this.text;
  }

  toDOM(): HTMLElement {
    const span = document.createElement('span');
    span.className = 'cm-inline-ai-completion';
    span.textContent = this.text;
    return span;
  }

  ignoreEvent(): boolean {
    return true;
  }
}

const inlineCompletionStateField =
  StateField.define<InlineCompletionState | null>({
    create: () => null,
    update(value, tr) {
      for (const effect of tr.effects) {
        if (effect.is(setInlineCompletionEffect)) {
          return effect.value;
        }
      }
      if (!value) {
        return null;
      }
      if (tr.docChanged || tr.selection) {
        return null;
      }
      return value;
    },
  });

const inlineCompletionDecorations = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(value, tr) {
    value = value.map(tr.changes);
    for (const effect of tr.effects) {
      if (effect.is(setInlineCompletionEffect)) {
        const completion = effect.value;
        if (!completion || !completion.text) {
          return Decoration.none;
        }
        return Decoration.set([
          Decoration.widget({
            widget: new InlineCompletionWidget(completion.displayText),
            side: 1,
          }).range(completion.from),
        ]);
      }
    }
    if (tr.docChanged || tr.selection) {
      return Decoration.none;
    }
    return value;
  },
  provide: (field) => EditorView.decorations.from(field),
});

function buildRequestID(): string {
  return `cmp-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
}

function lineRangeBlock(
  state: EditorState,
  lineNumber: number,
): EditorCompletionBlock | null {
  if (lineNumber < 1 || lineNumber > state.doc.lines) {
    return null;
  }
  const line = state.doc.line(lineNumber);
  const lines = [line];
  let startLine = lineNumber;
  let endLine = lineNumber;

  for (let n = lineNumber - 1; n >= 1; n -= 1) {
    const current = state.doc.line(n);
    if (!current.text.trim()) {
      break;
    }
    startLine = n;
    lines.unshift(current);
    if (lineNumber - startLine >= 24) {
      break;
    }
  }

  for (let n = lineNumber + 1; n <= state.doc.lines; n += 1) {
    const current = state.doc.line(n);
    if (!current.text.trim()) {
      break;
    }
    endLine = n;
    lines.push(current);
    if (endLine - lineNumber >= 24) {
      break;
    }
  }

  const from = state.doc.line(startLine).from;
  const to = state.doc.line(endLine).to;
  return {
    text: lines.map((item) => item.text).join('\n'),
    start: from,
    end: to,
  };
}

function buildCompletionRequest(
  view: EditorView,
  options: InlineCompletionOptions,
  requestID: string,
): InlineCompletionRequest | null {
  const selection = view.state.selection.main;
  if (!selection.empty) {
    return null;
  }
  const cursor = selection.head;
  if (cursor <= 0) {
    return null;
  }
  const line = view.state.doc.lineAt(cursor);
  const prefixFrom = Math.max(0, cursor - 8000);
  const suffixTo = Math.min(view.state.doc.length, cursor + 4000);
  const currentBlock = lineRangeBlock(view.state, line.number);
  return {
    requestID,
    editorKind: options.editorKind,
    languageId: options.languageId || null,
    documentPath: options.documentPath || null,
    cursorOffset: cursor,
    prefix: view.state.doc.sliceString(prefixFrom, cursor),
    suffix: view.state.doc.sliceString(cursor, suffixTo),
    currentBlock,
    previousBlock: currentBlock
      ? lineRangeBlock(
          view.state,
          view.state.doc.lineAt(currentBlock.start ?? cursor).number - 1,
        )
      : null,
    nextBlock: currentBlock
      ? lineRangeBlock(
          view.state,
          view.state.doc.lineAt(currentBlock.end ?? cursor).number + 1,
        )
      : null,
    maxOutputTokens: 96,
  };
}

function normalizeInsertText(value: string | null | undefined): string {
  return (value || '').replace(/\r\n/g, '\n');
}

class InlineCompletionPlugin {
  private timer: number | null = null;
  private activeRequestID: string | null = null;

  constructor(
    private readonly view: EditorView,
    private readonly options: InlineCompletionOptions,
  ) {
    this.schedule();
  }

  update(update: ViewUpdate): void {
    if (update.docChanged || update.selectionSet) {
      // Doc or selection actually changed: discard in-flight request + ghost text.
      this.clearCompletion();
      this.schedule();
    } else if (update.focusChanged) {
      if (!this.view.hasFocus) {
        // Lost focus: cancel everything.
        this.clearCompletion();
      } else if (!this.activeRequestID) {
        // Gained focus with no in-flight request: schedule.
        this.schedule();
      }
      // Gained focus while request in-flight: do NOT cancel, let it complete.
    }
  }

  destroy(): void {
    this.cancelTimer();
    this.cancelActive();
  }

  schedule(): void {
    this.cancelTimer();
    if (!this.view.hasFocus || !this.options.enabled()) {
      return;
    }
    this.timer = window.setTimeout(() => {
      void this.requestCompletion();
    }, 350);
  }

  private async requestCompletion(): Promise<void> {
    this.timer = null;
    if (!this.view.hasFocus || !this.options.enabled()) {
      return;
    }
    const requestID = buildRequestID();
    const request = buildCompletionRequest(this.view, this.options, requestID);
    if (!request) {
      return;
    }
    this.cancelActive();
    this.activeRequestID = requestID;
    const expectedDoc = this.view.state.doc;
    const expectedCursor = this.view.state.selection.main.head;
    const result = await this.options.request(request);
    if (this.activeRequestID !== requestID) {
      return;
    }
    this.activeRequestID = null;
    if (!result || result.requestID !== requestID) {
      return;
    }
    if (
      this.view.state.doc !== expectedDoc ||
      this.view.state.selection.main.head !== expectedCursor
    ) {
      return;
    }
    if (result.replaceFrom !== expectedCursor || result.replaceTo !== expectedCursor) {
      return;
    }
    const insertText = normalizeInsertText(result.insertText);
    if (!insertText.trim()) {
      return;
    }
    this.view.dispatch({
      effects: setInlineCompletionEffect.of({
        requestID,
        from: expectedCursor,
        text: insertText,
        displayText: buildDisplayText(insertText),
      }),
    });
  }

  private clearCompletion(): void {
    this.cancelActive();
    if (this.view.state.field(inlineCompletionStateField, false)) {
      this.view.dispatch({ effects: setInlineCompletionEffect.of(null) });
    }
  }

  private cancelTimer(): void {
    if (this.timer !== null) {
      window.clearTimeout(this.timer);
      this.timer = null;
    }
  }

  private cancelActive(): void {
    if (!this.activeRequestID) {
      return;
    }
    this.options.cancel?.(this.activeRequestID);
    this.activeRequestID = null;
  }
}

function acceptInlineCompletion(view: EditorView): boolean {
  const completion = view.state.field(inlineCompletionStateField, false);
  if (!completion || !completion.text) {
    return false;
  }
  view.dispatch({
    changes: {
      from: completion.from,
      insert: completion.text,
    },
    selection: { anchor: completion.from + completion.text.length },
    effects: setInlineCompletionEffect.of(null),
    scrollIntoView: true,
    userEvent: 'input.complete',
  });
  return true;
}

function clearInlineCompletion(view: EditorView): boolean {
  const completion = view.state.field(inlineCompletionStateField, false);
  if (!completion) {
    return false;
  }
  view.dispatch({ effects: setInlineCompletionEffect.of(null) });
  return true;
}

export function inlineCompletion(options: InlineCompletionOptions): Extension {
  const plugin = ViewPlugin.define(
    (view) => new InlineCompletionPlugin(view, options),
  );
  return [
    inlineCompletionStateField,
    inlineCompletionDecorations,
    plugin,
    Prec.highest(
      keymap.of([
        { key: 'Tab', run: acceptInlineCompletion },
        { key: 'Escape', run: clearInlineCompletion },
      ]),
    ),
  ];
}
