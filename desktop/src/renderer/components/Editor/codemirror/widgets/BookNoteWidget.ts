import { WidgetType } from '@codemirror/view';
import { useAppStore } from '../../../../store/appStore';
import type { BookOpenTarget } from '../../../../store/appStore';
import { resolveMarkdownPath } from '../../../../utils/markdownMedia';
import type { ParsedBookHighlightNote } from '../../bookNotes';

function formatCreated(value: string): string {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function targetFromNote(note: ParsedBookHighlightNote): BookOpenTarget | null {
  if (note.format === 'epub') {
    return note.cfi ? { format: 'epub', cfi: note.cfi, text: note.text } : null;
  }
  return note.page !== null ? {
    format: 'pdf',
    page: note.page,
    rects: note.rects,
    text: note.text,
  } : null;
}

export class BookNoteWidget extends WidgetType {
  constructor(
    private note: ParsedBookHighlightNote,
    private documentPath: string | null,
    private srcFrom: number,
    private srcTo: number,
  ) {
    super();
  }

  eq(other: BookNoteWidget): boolean {
    return other.documentPath === this.documentPath
      && other.srcFrom === this.srcFrom
      && other.srcTo === this.srcTo
      && JSON.stringify(other.note) === JSON.stringify(this.note);
  }

  ignoreEvent(event: Event): boolean {
    return event.type !== 'wheel';
  }

  toDOM(): HTMLElement {
    const element = document.createElement('div');
    element.className = 'cm-md-book-note';

    const header = document.createElement('div');
    header.className = 'cm-md-book-note-header';

    const meta = document.createElement('div');
    meta.className = 'cm-md-book-note-meta';
    const title = document.createElement('div');
    title.className = 'cm-md-book-note-title';
    title.textContent = this.note.title || this.note.source;
    const detail = document.createElement('div');
    detail.className = 'cm-md-book-note-detail';
    detail.textContent = [this.note.format.toUpperCase(), this.note.locator, formatCreated(this.note.created)]
      .filter(Boolean)
      .join(' | ');
    meta.append(title, detail);

    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'cm-md-book-note-open';
    button.textContent = 'Open';
    button.title = 'Open in book';
    button.setAttribute('aria-label', 'Open in book');
    button.addEventListener('mousedown', (event) => {
      event.preventDefault();
      event.stopPropagation();
    });
    button.addEventListener('click', (event) => {
      event.preventDefault();
      event.stopPropagation();
      const resolved = resolveMarkdownPath(this.documentPath, this.note.source, false);
      const bookTarget = targetFromNote(this.note);
      if (!resolved || !bookTarget) {
        return;
      }
      void useAppStore.getState().openFile(resolved, { bookTarget });
    });

    header.append(meta, button);

    const quote = document.createElement('blockquote');
    quote.className = 'cm-md-book-note-quote';
    quote.textContent = this.note.text;
    element.append(header, quote);
    return element;
  }
}
