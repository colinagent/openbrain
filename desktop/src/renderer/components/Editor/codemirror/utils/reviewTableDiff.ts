import {
  parseTableLine,
  splitTableCells,
  type ParsedTable,
} from './tableParser';

export type ReviewTableCellStatus = 'unchanged' | 'added' | 'removed' | 'modified';
export type ReviewTableRowKind = 'header' | 'body';

export type ReviewTableDiffCell = {
  status: ReviewTableCellStatus;
  oldText: string;
  newText: string;
};

export type ReviewTableDiffRow = {
  kind: ReviewTableRowKind;
  status: ReviewTableCellStatus;
  cells: ReviewTableDiffCell[];
};

export type ReviewTableDiff = {
  header: ReviewTableDiffRow;
  rows: ReviewTableDiffRow[];
};

export type ReviewTableHunk = {
  oldStartLine: number;
  oldLineCount: number;
  newStartLine: number;
  newLineCount: number;
  removedLines?: string[];
  addedLines?: string[];
};

export type ReviewTableBlockRange = {
  startLineNumber: number;
  endLineNumber: number;
};

type ParsedReviewTableRow = {
  lineNumber: number;
  cells: string[];
};

type ReviewTableTarget = {
  kind: ReviewTableRowKind;
  bodyIndex: number;
};

function normalizeCells(cells: readonly string[], columnCount: number): string[] {
  const normalized = cells.slice(0, columnCount);
  while (normalized.length < columnCount) {
    normalized.push('');
  }
  return normalized;
}

function getColumnCount(parsed: ParsedTable): number {
  return Math.max(parsed.headers.length, 0, ...parsed.rows.map((row) => row.length));
}

function isTableDataLine(text: string): boolean {
  const parsed = parseTableLine(text);
  return Boolean(parsed && !parsed.isSeparator && splitTableCells(text).length >= 2);
}

function parseReviewTableRows(
  lines: readonly string[] | null | undefined,
  startLine: number,
  columnCount: number
): ParsedReviewTableRow[] {
  if (!lines || lines.length === 0) {
    return [];
  }
  const rows: ParsedReviewTableRow[] = [];
  for (let index = 0; index < lines.length; index += 1) {
    const line = String(lines[index] ?? '');
    if (!isTableDataLine(line)) {
      continue;
    }
    rows.push({
      lineNumber: startLine + index,
      cells: normalizeCells(splitTableCells(line), columnCount),
    });
  }
  return rows;
}

export function reviewHunkHasTableRows(hunk: ReviewTableHunk): boolean {
  return [...(hunk.removedLines || []), ...(hunk.addedLines || [])].some(isTableDataLine);
}

export function reviewHunkCanTouchTableBlock(
  hunk: ReviewTableHunk,
  block: ReviewTableBlockRange
): boolean {
  const span = Math.max(1, hunk.newLineCount || 0);
  const startLine = hunk.newStartLine;
  const endLine = hunk.newStartLine + span - 1;
  return startLine <= block.endLineNumber + 1 && endLine >= block.startLineNumber;
}

function getTargetForLine(
  block: ReviewTableBlockRange,
  lineNumber: number,
  bodyRowCount: number
): ReviewTableTarget | null {
  if (lineNumber === block.startLineNumber) {
    return { kind: 'header', bodyIndex: -1 };
  }
  if (lineNumber < block.startLineNumber + 2 || lineNumber > block.endLineNumber) {
    return null;
  }
  const bodyIndex = lineNumber - block.startLineNumber - 2;
  if (bodyIndex < 0 || bodyIndex >= bodyRowCount) {
    return null;
  }
  return { kind: 'body', bodyIndex };
}

function bodyInsertionIndexForLine(
  block: ReviewTableBlockRange,
  lineNumber: number,
  bodyRowCount: number
): number {
  if (lineNumber <= block.startLineNumber + 2) {
    return 0;
  }
  return Math.max(0, Math.min(bodyRowCount, lineNumber - block.startLineNumber - 2));
}

function buildUnchangedRow(kind: ReviewTableRowKind, cells: readonly string[], columnCount: number): ReviewTableDiffRow {
  return {
    kind,
    status: 'unchanged',
    cells: normalizeCells(cells, columnCount).map((text) => ({
      status: 'unchanged',
      oldText: text,
      newText: text,
    })),
  };
}

function buildAddedRow(kind: ReviewTableRowKind, cells: readonly string[], columnCount: number): ReviewTableDiffRow {
  return {
    kind,
    status: 'added',
    cells: normalizeCells(cells, columnCount).map((text) => ({
      status: 'added',
      oldText: '',
      newText: text,
    })),
  };
}

function buildRemovedRow(kind: ReviewTableRowKind, cells: readonly string[], columnCount: number): ReviewTableDiffRow {
  return {
    kind,
    status: 'removed',
    cells: normalizeCells(cells, columnCount).map((text) => ({
      status: 'removed',
      oldText: text,
      newText: '',
    })),
  };
}

function buildPairedRow(
  kind: ReviewTableRowKind,
  oldCells: readonly string[],
  newCells: readonly string[],
  columnCount: number
): ReviewTableDiffRow {
  let rowStatus: ReviewTableCellStatus = 'unchanged';
  const normalizedOld = normalizeCells(oldCells, columnCount);
  const normalizedNew = normalizeCells(newCells, columnCount);
  const cells = normalizedNew.map((newText, index) => {
    const oldText = normalizedOld[index] ?? '';
    if (oldText === newText) {
      return { status: 'unchanged' as const, oldText, newText };
    }
    rowStatus = 'modified';
    return { status: 'modified' as const, oldText, newText };
  });
  return { kind, status: rowStatus, cells };
}

function hasVisibleChange(row: ReviewTableDiffRow): boolean {
  return row.status !== 'unchanged' || row.cells.some((cell) => cell.status !== 'unchanged');
}

export function buildReviewTableDiffForBlock(
  hunks: readonly ReviewTableHunk[] | null | undefined,
  parsed: ParsedTable,
  block: ReviewTableBlockRange
): ReviewTableDiff | null {
  if (!hunks || hunks.length === 0) {
    return null;
  }

  const columnCount = getColumnCount(parsed);
  if (columnCount === 0) {
    return null;
  }

  let changed = false;
  let header = buildUnchangedRow('header', parsed.headers, columnCount);
  const rows = parsed.rows.map((row) => buildUnchangedRow('body', row, columnCount));
  const removedBefore = new Map<number, ReviewTableDiffRow[]>();

  const insertRemovedRows = (beforeIndex: number, removedRows: ParsedReviewTableRow[]) => {
    if (removedRows.length === 0) {
      return;
    }
    const clampedIndex = Math.max(0, Math.min(rows.length, beforeIndex));
    const existing = removedBefore.get(clampedIndex) ?? [];
    for (const removedRow of removedRows) {
      existing.push(buildRemovedRow('body', removedRow.cells, columnCount));
    }
    removedBefore.set(clampedIndex, existing);
    changed = true;
  };

  for (const hunk of hunks) {
    if (!reviewHunkHasTableRows(hunk) || !reviewHunkCanTouchTableBlock(hunk, block)) {
      continue;
    }

    const oldRows = parseReviewTableRows(hunk.removedLines, hunk.oldStartLine, columnCount);
    const newRows = parseReviewTableRows(hunk.addedLines, hunk.newStartLine, columnCount)
      .map((row) => ({ row, target: getTargetForLine(block, row.lineNumber, rows.length) }))
      .filter((entry): entry is { row: ParsedReviewTableRow; target: ReviewTableTarget } => entry.target !== null);

    if (oldRows.length === 0 && newRows.length === 0) {
      continue;
    }

    const pairedCount = Math.min(oldRows.length, newRows.length);
    let lastBodyTarget = -1;

    for (let index = 0; index < pairedCount; index += 1) {
      const oldRow = oldRows[index];
      const { row: newRow, target } = newRows[index];
      const diffRow = buildPairedRow(target.kind, oldRow.cells, newRow.cells, columnCount);
      if (target.kind === 'header') {
        header = diffRow;
      } else {
        rows[target.bodyIndex] = diffRow;
        lastBodyTarget = target.bodyIndex;
      }
      changed = changed || hasVisibleChange(diffRow);
    }

    for (let index = pairedCount; index < newRows.length; index += 1) {
      const { row, target } = newRows[index];
      const diffRow = buildAddedRow(target.kind, row.cells, columnCount);
      if (target.kind === 'header') {
        header = diffRow;
      } else {
        rows[target.bodyIndex] = diffRow;
        lastBodyTarget = target.bodyIndex;
      }
      changed = true;
    }

    if (oldRows.length > pairedCount) {
      const insertionIndex = lastBodyTarget >= 0
        ? lastBodyTarget + 1
        : bodyInsertionIndexForLine(block, hunk.newStartLine, rows.length);
      insertRemovedRows(insertionIndex, oldRows.slice(pairedCount));
    }
  }

  if (!changed) {
    return null;
  }

  const mergedRows: ReviewTableDiffRow[] = [];
  for (let index = 0; index <= rows.length; index += 1) {
    const removedRows = removedBefore.get(index);
    if (removedRows) {
      mergedRows.push(...removedRows);
    }
    if (index < rows.length) {
      mergedRows.push(rows[index]);
    }
  }

  return { header, rows: mergedRows };
}
