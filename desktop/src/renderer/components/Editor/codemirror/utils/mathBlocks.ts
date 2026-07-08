import { EditorState, Range } from '@codemirror/state';
import { Decoration } from '@codemirror/view';
import { decorateLineRange } from './lineDecorations';

const MAX_MATH_SCAN_LINES = 100; // Limit how far we scan for $$ pairs (selection-based)

function isCursorOnLineOverlappingRange(state: EditorState, range: { from: number; to: number }): boolean {
  const selection = state.selection.main;
  const cursorLine = state.doc.lineAt(selection.head);
  return cursorLine.from <= range.to && cursorLine.to >= range.from;
}

export function findMathBlockAtLine(state: EditorState, lineNum: number): { from: number; to: number } | null {
  const doc = state.doc;
  const startLine = Math.max(1, lineNum - MAX_MATH_SCAN_LINES);
  const endLine = Math.min(doc.lines, lineNum + MAX_MATH_SCAN_LINES);
  let inBlock = false;
  let blockStart = 0;
  for (let i = startLine; i <= endLine; i++) {
    const line = doc.line(i);
    const idx = line.text.indexOf('$$');
    if (idx === -1) continue;
    const pos = line.from + idx;
    if (!inBlock) {
      const second = line.text.indexOf('$$', idx + 2);
      if (second !== -1) {
        const from = pos;
        const to = line.from + second + 2;
        if (lineNum === i) return { from, to };
        continue;
      }
      inBlock = true;
      blockStart = pos;
      continue;
    }
    const blockEnd = pos + 2;
    if (lineNum >= doc.lineAt(blockStart).number && lineNum <= i) {
      return { from: blockStart, to: blockEnd };
    }
    inBlock = false;
  }
  return null;
}

export function buildMathBlockDecorationsInRange(
  state: EditorState,
  rangeFrom: number,
  rangeTo: number,
  out: Range<Decoration>[]
): void {
  const doc = state.doc;
  const startLine = doc.lineAt(rangeFrom).number;
  const endLine = doc.lineAt(rangeTo).number;
  // Expand a bit to catch $$ pairs that start/end outside the range
  const scanStart = Math.max(1, startLine - 20);
  const scanEnd = Math.min(doc.lines, endLine + 20);
  let inBlock = false;
  let blockStart = 0;
  for (let i = scanStart; i <= scanEnd; i++) {
    const line = doc.line(i);
    const idx = line.text.indexOf('$$');
    if (idx === -1) continue;
    const pos = line.from + idx;
    if (!inBlock) {
      const second = line.text.indexOf('$$', idx + 2);
      if (second !== -1) {
        const from = pos;
        const to = line.from + second + 2;
        if (to >= rangeFrom && from <= rangeTo) {
          decorateLineRange(state, from, to, 'cm-md-math-block-line', out);
        }
        continue;
      }
      inBlock = true;
      blockStart = pos;
      continue;
    }
    const blockEnd = pos + 2;
    if (blockEnd >= rangeFrom && blockStart <= rangeTo) {
      decorateLineRange(state, blockStart, blockEnd, 'cm-md-math-block-line', out);
    }
    inBlock = false;
  }
}
