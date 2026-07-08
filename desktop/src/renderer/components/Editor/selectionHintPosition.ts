import type { EditorView } from '@codemirror/view';

type HintPosition = { left: number; top: number } | null;

export function getAddToChatShortcutLabel(): string {
  const platform = typeof navigator !== 'undefined' ? (navigator.platform || '').toLowerCase() : '';
  return platform.includes('mac') ? '⌘L' : 'Ctrl+L';
}

export function resolveSelectionHintPosition(view: EditorView): HintPosition {
  const selection = view.state.selection.main;
  if (!selection || selection.empty) {
    return null;
  }

  const start = view.coordsAtPos(selection.from);
  const end = view.coordsAtPos(selection.to);
  const anchor = end || start;
  if (!anchor) {
    return null;
  }

  const viewportWidth = window.innerWidth;
  const top = anchor.top <= 40 ? anchor.bottom + 36 : anchor.top - 12;
  const left = Math.min(Math.max(anchor.left, 72), viewportWidth - 72);

  return { left, top };
}
