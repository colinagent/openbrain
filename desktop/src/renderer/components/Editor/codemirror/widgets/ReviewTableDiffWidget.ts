import { EditorSelection } from '@codemirror/state';
import { EditorView, WidgetType } from '@codemirror/view';
import { openTableSourceEffect } from '../tableSourceState';
import {
  parseTableCellInlineMarkdown,
  type TableCellInlineSegment,
} from '../utils/tableCellInlineMarkdown';
import type { TableAlignment } from '../utils/tableParser';
import type { ReviewTableDiff, ReviewTableDiffCell, ReviewTableDiffRow } from '../utils/reviewTableDiff';
import { CM_MD_INLINE_CODE } from '../markdownInlinePill';
import { EDIT_SOURCE_ICON_SVG } from './editSourceIcon';

function textAlignFor(alignment: TableAlignment | undefined): string {
  if (alignment === 'center') return 'center';
  if (alignment === 'right') return 'right';
  return 'left';
}

function applyAlignmentToCell(cell: HTMLElement, alignment: TableAlignment | undefined): void {
  cell.style.textAlign = textAlignFor(alignment);
}

function appendRenderedInlineSegments(
  parent: Node,
  segments: readonly TableCellInlineSegment[]
): void {
  for (const segment of segments) {
    switch (segment.type) {
      case 'text':
        parent.appendChild(document.createTextNode(segment.text));
        break;
      case 'lineBreak':
        parent.appendChild(document.createElement('br'));
        break;
      case 'code': {
        const code = document.createElement('code');
        code.className = `cm-md-code ${CM_MD_INLINE_CODE}`;
        code.textContent = segment.text;
        parent.appendChild(code);
        break;
      }
      case 'strong': {
        const strong = document.createElement('strong');
        strong.className = 'cm-md-strong';
        appendRenderedInlineSegments(strong, segment.children);
        parent.appendChild(strong);
        break;
      }
      case 'emphasis': {
        const em = document.createElement('em');
        em.className = 'cm-md-emphasis';
        appendRenderedInlineSegments(em, segment.children);
        parent.appendChild(em);
        break;
      }
      case 'strikethrough': {
        const del = document.createElement('del');
        del.className = 'cm-md-strikethrough';
        appendRenderedInlineSegments(del, segment.children);
        parent.appendChild(del);
        break;
      }
      case 'highlight': {
        const mark = document.createElement('mark');
        mark.className = 'cm-md-highlight';
        appendRenderedInlineSegments(mark, segment.children);
        parent.appendChild(mark);
        break;
      }
    }
  }
}

function appendInlineMarkdown(parent: HTMLElement, text: string): void {
  appendRenderedInlineSegments(parent, parseTableCellInlineMarkdown(text));
  if (parent.childNodes.length === 0) {
    parent.appendChild(document.createTextNode(''));
  }
}

function appendReviewCellContent(cell: HTMLElement, diffCell: ReviewTableDiffCell): void {
  if (diffCell.status !== 'modified') {
    appendInlineMarkdown(cell, diffCell.status === 'removed' ? diffCell.oldText : diffCell.newText);
    return;
  }

  const oldValue = document.createElement('span');
  oldValue.className = 'cm-review-table-cell-old';
  appendInlineMarkdown(oldValue, diffCell.oldText);
  cell.appendChild(oldValue);

  const newValue = document.createElement('span');
  newValue.className = 'cm-review-table-cell-new';
  appendInlineMarkdown(newValue, diffCell.newText);
  cell.appendChild(newValue);
}

function appendReviewRow(
  parent: HTMLElement,
  row: ReviewTableDiffRow,
  alignments: readonly TableAlignment[],
  cellTag: 'th' | 'td'
): void {
  const tr = document.createElement('tr');
  tr.className = `cm-review-table-row-${row.status}`;
  for (let columnIndex = 0; columnIndex < row.cells.length; columnIndex += 1) {
    const cell = document.createElement(cellTag);
    const diffCell = row.cells[columnIndex];
    cell.className = `cm-review-table-cell-${diffCell.status}`;
    appendReviewCellContent(cell, diffCell);
    applyAlignmentToCell(cell, alignments[columnIndex]);
    tr.appendChild(cell);
  }
  parent.appendChild(tr);
}

export class ReviewTableDiffWidget extends WidgetType {
  constructor(
    private readonly diff: ReviewTableDiff,
    private readonly alignments: TableAlignment[],
    private readonly srcFrom: number,
    private readonly srcTo: number,
    private readonly showSourceButton: boolean = true
  ) {
    super();
  }

  eq(other: ReviewTableDiffWidget): boolean {
    return this.srcFrom === other.srcFrom
      && this.srcTo === other.srcTo
      && this.showSourceButton === other.showSourceButton
      && JSON.stringify(this.alignments) === JSON.stringify(other.alignments)
      && JSON.stringify(this.diff) === JSON.stringify(other.diff);
  }

  ignoreEvent(): boolean {
    return true;
  }

  toDOM(view: EditorView): HTMLElement {
    const element = document.createElement('div');
    element.className = 'cm-md-table-block cm-md-table-block-static cm-md-table-review-block';

    const wrapper = document.createElement('div');
    wrapper.className = 'cm-md-table-wrapper';

    if (this.showSourceButton) {
      const sourceRail = document.createElement('div');
      sourceRail.className = 'cm-md-table-src-rail';
      const button = document.createElement('button');
      button.className = 'cm-md-edit-src-btn cm-md-table-src-btn';
      button.type = 'button';
      button.title = 'Edit source';
      button.setAttribute('aria-label', 'Edit source');
      button.innerHTML = EDIT_SOURCE_ICON_SVG;
      button.addEventListener('mousedown', (event) => {
        event.preventDefault();
        event.stopPropagation();
      });
      button.addEventListener('click', (event) => {
        event.preventDefault();
        event.stopPropagation();
        view.dispatch({
          effects: openTableSourceEffect.of(this.srcFrom),
          selection: EditorSelection.cursor(this.srcFrom),
          scrollIntoView: true,
        });
      });
      sourceRail.appendChild(button);
      element.appendChild(sourceRail);
    }

    const table = document.createElement('table');
    const thead = document.createElement('thead');
    const tbody = document.createElement('tbody');

    appendReviewRow(thead, this.diff.header, this.alignments, 'th');
    for (const row of this.diff.rows) {
      appendReviewRow(tbody, row, this.alignments, 'td');
    }

    table.appendChild(thead);
    table.appendChild(tbody);
    wrapper.appendChild(table);
    element.appendChild(wrapper);
    return element;
  }
}
