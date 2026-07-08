import React, { useCallback, useEffect, useMemo, useState } from 'react';
import type { NavItem } from 'epubjs';
import { getRenderHandleForPhysicalPath } from '../../services/resourceService';
import { writeClipboardText } from '../../services/clipboardService';
import { useAppStore } from '../../store/appStore';
import { useToastStore } from '../../store/toastStore';
import { ChevronRightIcon, CopyIcon, FileIcon, ListIcon, MoreHorizontalIcon, PlusIcon, RefreshIcon } from '../Icons';
import { IconButton } from '../IconButton';
import { PopupMenu, PopupMenuItem, PopupMenuSeparator } from '../PopupMenu';
import {
  appendBookHighlightNote,
  buildBookHighlightEntry,
  filterBookHighlightNotesForTarget,
  getBookNotePath,
  hasHighlightText,
  parseBookHighlightNotes,
  parseBookHighlightNoteBlock,
  removeBookHighlightNote,
  type BookHighlightFormat,
  type BookHighlightNoteRemovalTarget,
  type ParsedBookHighlightNote,
} from './bookNotes';
import {
  EpubBookView,
  type EpubDisplayRequest,
  type EpubHighlightRequest,
  type EpubHighlightContextAction,
  type EpubOpenMode,
  type EpubProgress,
  type EpubReaderSettings,
  type EpubSelection,
} from './EpubBookView';
import { PdfBookView, type PdfHighlightContextAction, type PdfHighlightRequest, type PdfSelection } from './PdfBookView';
import type { ResourceMeta, RenderHandle } from '../../core/resource/uri';

type BookReaderEditorProps = {
  tabId?: string | null;
};

type ReaderSelection = (EpubSelection | PdfSelection) & {
  text: string;
  locator: string;
};

const MIN_ZOOM = 0.6;
const MAX_ZOOM = 2.4;
const ZOOM_STEP = 0.15;
const MIN_EPUB_FONT = 0.82;
const MAX_EPUB_FONT = 1.45;
const EPUB_FONT_STEP = 0.08;
const EPUB_READER_SETTINGS_KEY = 'openbrain.epub.readerSettings.v1';
const EPUB_LOCATION_KEY_PREFIX = 'openbrain.epub.location.v1:';
const RECT_EPSILON = 0.00002;

function bookErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unable to load book';
}

function resolveBookFormat(path: string): BookHighlightFormat | null {
  const lower = path.toLowerCase();
  if (lower.endsWith('.epub')) {
    return 'epub';
  }
  if (lower.endsWith('.pdf')) {
    return 'pdf';
  }
  return null;
}

function clampZoom(value: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, value));
}

function clampEpubFontScale(value: number): number {
  return Math.min(MAX_EPUB_FONT, Math.max(MIN_EPUB_FONT, value));
}

function loadEpubReaderSettings(): EpubReaderSettings {
  if (typeof window === 'undefined') {
    return { fontScale: 1, theme: 'paper' };
  }
  try {
    const raw = window.localStorage.getItem(EPUB_READER_SETTINGS_KEY);
    const parsed = raw ? JSON.parse(raw) as Partial<EpubReaderSettings> : {};
    const theme = parsed.theme === 'sepia' || parsed.theme === 'night' ? parsed.theme : 'paper';
    const fontScale = typeof parsed.fontScale === 'number' && Number.isFinite(parsed.fontScale)
      ? clampEpubFontScale(parsed.fontScale)
      : 1;
    return { fontScale, theme };
  } catch {
    return { fontScale: 1, theme: 'paper' };
  }
}

function saveEpubReaderSettings(settings: EpubReaderSettings) {
  if (typeof window === 'undefined') {
    return;
  }
  window.localStorage.setItem(EPUB_READER_SETTINGS_KEY, JSON.stringify(settings));
}

function locationStorageKey(filePath: string): string {
  return `${EPUB_LOCATION_KEY_PREFIX}${filePath}`;
}

function loadSavedEpubCfi(filePath: string): string | null {
  if (typeof window === 'undefined' || !filePath) {
    return null;
  }
  try {
    const raw = window.localStorage.getItem(locationStorageKey(filePath));
    const parsed = raw ? JSON.parse(raw) as { cfi?: unknown } : null;
    return typeof parsed?.cfi === 'string' && parsed.cfi.trim() ? parsed.cfi : null;
  } catch {
    return null;
  }
}

function saveEpubProgress(filePath: string, progress: EpubProgress) {
  if (typeof window === 'undefined' || !filePath || !progress.cfi) {
    return;
  }
  window.localStorage.setItem(locationStorageKey(filePath), JSON.stringify({
    cfi: progress.cfi,
    locator: progress.locator,
    percentage: progress.percentage,
    updatedAt: new Date().toISOString(),
  }));
}

function resolveEpubOpen(meta: ResourceMeta | null, handle: RenderHandle | null, url: string): { url: string; mode: EpubOpenMode } {
  if (meta?.entryType === 'directory' || handle?.entryType === 'directory') {
    const packagePath = (handle?.epubPackagePath || meta?.epubPackagePath || '').trim();
    if (packagePath) {
      return { url: new URL(packagePath, url).toString(), mode: 'opf' };
    }
    return { url, mode: 'directory' };
  }
  return { url, mode: 'epub' };
}

function flattenToc(items: NavItem[], depth = 0): Array<NavItem & { depth: number }> {
  const rows: Array<NavItem & { depth: number }> = [];
  for (const item of items) {
    rows.push({ ...item, depth });
    if (item.subitems?.length) {
      rows.push(...flattenToc(item.subitems, depth + 1));
    }
  }
  return rows;
}

function isEpubSelection(selection: ReaderSelection): selection is EpubSelection & ReaderSelection {
  return 'cfiRange' in selection;
}

function isPdfSelection(selection: ReaderSelection): selection is PdfSelection & ReaderSelection {
  return 'page' in selection;
}

function sameHighlightRect(
  left: { top: number; left: number; width: number; height: number },
  right: { top: number; left: number; width: number; height: number }
): boolean {
  return Math.abs(left.top - right.top) <= RECT_EPSILON
    && Math.abs(left.left - right.left) <= RECT_EPSILON
    && Math.abs(left.width - right.width) <= RECT_EPSILON
    && Math.abs(left.height - right.height) <= RECT_EPSILON;
}

function sameHighlightRects(
  left: Array<{ top: number; left: number; width: number; height: number }>,
  right: Array<{ top: number; left: number; width: number; height: number }>
): boolean {
  if (left.length !== right.length) {
    return false;
  }
  return left.every((rect, index) => sameHighlightRect(rect, right[index]));
}

function selectionMatchesHighlightNote(selection: ReaderSelection, note: ParsedBookHighlightNote): boolean {
  if (isEpubSelection(selection)) {
    return note.format === 'epub' && note.cfi === selection.cfiRange;
  }
  if (isPdfSelection(selection)) {
    return note.format === 'pdf'
      && note.page === selection.page
      && sameHighlightRects(note.rects, selection.rects);
  }
  return false;
}

function removalTargetFromSelection(sourcePath: string, selection: ReaderSelection): BookHighlightNoteRemovalTarget {
  if (isEpubSelection(selection)) {
    return {
      sourcePath,
      format: 'epub',
      cfi: selection.cfiRange,
    };
  }
  return {
    sourcePath,
    format: 'pdf',
    page: selection.page,
    rects: selection.rects,
  };
}

export const BookReaderEditor: React.FC<BookReaderEditorProps> = ({ tabId = null }) => {
  const documents = useAppStore((state) => state.documents);
  const activeTabId = useAppStore((state) => state.activeTabId);
  const openFile = useAppStore((state) => state.openFile);
  const readTextFile = useAppStore((state) => state.readTextFile);
  const statPath = useAppStore((state) => state.statPath);
  const writeTextFile = useAppStore((state) => state.writeTextFile);
  const setPendingBookTarget = useAppStore((state) => state.setPendingBookTarget);
  const pushToast = useToastStore((state) => state.pushToast);
  const [renderUrl, setRenderUrl] = useState('');
  const [resourceMeta, setResourceMeta] = useState<ResourceMeta | null>(null);
  const [renderHandle, setRenderHandle] = useState<RenderHandle | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [refreshSeq, setRefreshSeq] = useState(0);
  const [selection, setSelection] = useState<ReaderSelection | null>(null);
  const [locator, setLocator] = useState('');
  const [page, setPage] = useState(1);
  const [pageCount, setPageCount] = useState(0);
  const [zoom, setZoom] = useState(1);
  const [epubProgress, setEpubProgress] = useState<EpubProgress>({ locator: '', cfi: '', percentage: null });
  const [toc, setToc] = useState<NavItem[]>([]);
  const [tocOpen, setTocOpen] = useState(true);
  const [epubDisplayRequest, setEpubDisplayRequest] = useState<EpubDisplayRequest | null>(null);
  const [epubHighlightRequest, setEpubHighlightRequest] = useState<EpubHighlightRequest | null>(null);
  const [pdfHighlightRequest, setPdfHighlightRequest] = useState<PdfHighlightRequest | null>(null);
  const [highlightNotes, setHighlightNotes] = useState<ParsedBookHighlightNote[]>([]);
  const [epubReaderSettings, setEpubReaderSettings] = useState<EpubReaderSettings>(() => loadEpubReaderSettings());
  const [readerMenuOpen, setReaderMenuOpen] = useState(false);
  const readerMenuRef = React.useRef<HTMLDivElement | null>(null);

  const tab = useMemo(() => {
    const id = tabId || activeTabId || '';
    return documents.find((item) => item.id === id) || null;
  }, [activeTabId, documents, tabId]);

  const filePath = (tab?.filePath || '').trim();
  const resourceVersion = tab?.resourceVersion ?? 0;
  const missing = Boolean(tab?.missing);
  const format = resolveBookFormat(filePath);
  const notePath = filePath ? getBookNotePath(filePath) : '';
  const pendingBookTarget = tab?.pendingBookTarget || null;

  useEffect(() => {
    let cancelled = false;
    setRenderUrl('');
    setResourceMeta(null);
    setRenderHandle(null);
    setError(null);
    setSelection(null);
    setLocator('');
    setEpubProgress({ locator: '', cfi: '', percentage: null });
    setToc([]);
    setEpubDisplayRequest(null);
    setEpubHighlightRequest(null);
    setPdfHighlightRequest(null);
    setHighlightNotes([]);

    if (!filePath) {
      setLoading(false);
      setError('Book path is missing');
      return () => {
        cancelled = true;
      };
    }

    if (!format) {
      setLoading(false);
      setError('Unsupported book format');
      return () => {
        cancelled = true;
      };
    }

    if (missing) {
      setLoading(false);
      setError('File was deleted on disk');
      return () => {
        cancelled = true;
      };
    }

    setLoading(true);
    getRenderHandleForPhysicalPath(filePath)
      .then(({ url, meta, handle }) => {
        if (cancelled) return;
        setRenderUrl(url);
        setResourceMeta(meta);
        setRenderHandle(handle);
        setError(null);
      })
      .catch((nextError) => {
        if (cancelled) return;
        setError(bookErrorMessage(nextError));
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [filePath, format, missing, refreshSeq, resourceVersion]);

  useEffect(() => {
    let cancelled = false;
    setHighlightNotes([]);

    if (!filePath || !format || !notePath || missing) {
      return () => {
        cancelled = true;
      };
    }

    readTextFile(notePath)
      .then(async (content) => {
        if (cancelled) {
          return;
        }
        if (content === null) {
          const stat = await statPath(notePath);
          if (!cancelled && stat.error && !/not found|no such file/i.test(stat.error)) {
            console.warn('Failed to read highlight notes:', stat.error);
          }
          return;
        }
        if (cancelled) {
          return;
        }
        setHighlightNotes(filterBookHighlightNotesForTarget(parseBookHighlightNotes(content), { sourcePath: filePath, format }));
      })
      .catch((nextError) => {
        if (!cancelled) {
          console.warn('Failed to load highlight notes:', nextError);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [filePath, format, missing, notePath, readTextFile, refreshSeq, resourceVersion, statPath]);

  useEffect(() => {
    saveEpubReaderSettings(epubReaderSettings);
  }, [epubReaderSettings]);

  const handleHighlight = useCallback(async (targetSelection: ReaderSelection | null = selection) => {
    if (!targetSelection || !format || !filePath || !hasHighlightText(targetSelection.text)) {
      pushToast('Select text to highlight');
      return;
    }

    try {
      const existing = await readTextFile(notePath);
      if (existing === null) {
        const stat = await statPath(notePath);
        if (stat.error && !/not found|no such file/i.test(stat.error)) {
          throw new Error(stat.error);
        }
        if (!stat.error) {
          throw new Error('Highlight notes file could not be read');
        }
      }
      const entryInput = {
        sourcePath: filePath,
        sourceTitle: tab?.title || null,
        format,
        text: targetSelection.text,
        locator: targetSelection.locator,
        cfi: isEpubSelection(targetSelection) ? targetSelection.cfiRange : null,
        page: isPdfSelection(targetSelection) ? targetSelection.page : null,
        rects: isPdfSelection(targetSelection) ? targetSelection.rects : null,
        createdAt: new Date(),
      };
      const entry = buildBookHighlightEntry(entryInput);
      const content = appendBookHighlightNote(existing || '', entryInput);
      const saved = await writeTextFile(notePath, content);
      if (!saved) {
        throw new Error('Failed to write highlight notes');
      }
      const parsed = parseBookHighlightNoteBlock(entry);
      if (parsed) {
        setHighlightNotes((current) => filterBookHighlightNotesForTarget([...current, parsed], { sourcePath: filePath, format }));
      }
      targetSelection.applyHighlight();
      setSelection(null);
      pushToast('Highlight saved');
    } catch (nextError) {
      pushToast(bookErrorMessage(nextError));
    }
  }, [filePath, format, notePath, pushToast, readTextFile, selection, statPath, tab?.title, writeTextFile]);

  const handleRemoveHighlight = useCallback(async (target: BookHighlightNoteRemovalTarget): Promise<boolean> => {
    if (!format || !filePath || !notePath) {
      return false;
    }

    try {
      const existing = await readTextFile(notePath);
      if (existing === null) {
        const stat = await statPath(notePath);
        if (stat.error && !/not found|no such file/i.test(stat.error)) {
          throw new Error(stat.error);
        }
        pushToast('Highlight not found');
        return false;
      }

      const result = removeBookHighlightNote(existing, target);
      if (result.removed === 0) {
        pushToast('Highlight not found');
        return false;
      }

      const saved = await writeTextFile(notePath, result.content);
      if (!saved) {
        throw new Error('Failed to write highlight notes');
      }

      setHighlightNotes(filterBookHighlightNotesForTarget(parseBookHighlightNotes(result.content), { sourcePath: filePath, format }));
      setSelection(null);
      pushToast('Highlight removed');
      return true;
    } catch (nextError) {
      pushToast(bookErrorMessage(nextError));
      return false;
    }
  }, [filePath, format, notePath, pushToast, readTextFile, statPath, writeTextFile]);

  const handleHighlightContextAction = useCallback((action: EpubHighlightContextAction | PdfHighlightContextAction) => {
    if (!filePath) {
      return;
    }

    if (action.type === 'save') {
      const target = removalTargetFromSelection(filePath, action.selection);
      if (highlightNotes.some((note) => selectionMatchesHighlightNote(action.selection, note))) {
        void handleRemoveHighlight(target);
      } else {
        void handleHighlight(action.selection);
      }
      return;
    }

    if ('cfi' in action) {
      void handleRemoveHighlight({ sourcePath: filePath, format: 'epub', cfi: action.cfi });
      return;
    }

    void handleRemoveHighlight({ sourcePath: filePath, format: 'pdf', page: action.page, rects: action.rects });
  }, [filePath, handleHighlight, handleRemoveHighlight, highlightNotes]);

  const handleHighlightAndClose = useCallback(async () => {
    await handleHighlight();
    setReaderMenuOpen(false);
  }, [handleHighlight]);

  const handleOpenNotes = useCallback(() => {
    if (notePath) {
      void openFile(notePath);
    }
    setReaderMenuOpen(false);
  }, [notePath, openFile]);

  const handleCopyPath = useCallback(async () => {
    if (!filePath) {
      return;
    }
    try {
      await writeClipboardText(filePath);
      pushToast('Path copied');
    } catch (nextError) {
      pushToast(bookErrorMessage(nextError));
    }
    setReaderMenuOpen(false);
  }, [filePath, pushToast]);

  const handleRefresh = useCallback(() => {
    setRefreshSeq((current) => current + 1);
    setReaderMenuOpen(false);
  }, []);

  const handleError = useCallback((message: string) => {
    setError(message);
  }, []);

  const handleZoomOut = useCallback(() => {
    setZoom((current) => clampZoom(current - ZOOM_STEP));
  }, []);

  const handleZoomIn = useCallback(() => {
    setZoom((current) => clampZoom(current + ZOOM_STEP));
  }, []);

  const handleEpubProgressChange = useCallback((progress: EpubProgress) => {
    setEpubProgress(progress);
    setLocator(progress.locator);
    saveEpubProgress(filePath, progress);
  }, [filePath]);

  const handleEpubFontOut = useCallback(() => {
    setEpubReaderSettings((current) => ({ ...current, fontScale: clampEpubFontScale(current.fontScale - EPUB_FONT_STEP) }));
  }, []);

  const handleEpubFontIn = useCallback(() => {
    setEpubReaderSettings((current) => ({ ...current, fontScale: clampEpubFontScale(current.fontScale + EPUB_FONT_STEP) }));
  }, []);

  const handleEpubTheme = useCallback((theme: EpubReaderSettings['theme']) => {
    setEpubReaderSettings((current) => ({ ...current, theme }));
  }, []);

  const handleTocNavigate = useCallback((target: string) => {
    const trimmed = target.trim();
    if (!trimmed) {
      return;
    }
    setEpubDisplayRequest((current) => ({
      target: trimmed,
      seq: (current?.seq ?? 0) + 1,
    }));
  }, []);

  useEffect(() => {
    if (!pendingBookTarget || !format || pendingBookTarget.format !== format) {
      return;
    }
    if (pendingBookTarget.format === 'epub') {
      const cfi = pendingBookTarget.cfi.trim();
      if (cfi) {
        setEpubHighlightRequest((current) => ({ cfi, seq: (current?.seq ?? 0) + 1 }));
      }
    } else if (pendingBookTarget.format === 'pdf') {
      setPage(pendingBookTarget.page);
      setPdfHighlightRequest((current) => ({
        page: pendingBookTarget.page,
        rects: pendingBookTarget.rects || [],
        seq: (current?.seq ?? 0) + 1,
      }));
    }
    setPendingBookTarget(null);
  }, [format, pendingBookTarget, setPendingBookTarget]);

  useEffect(() => {
    if (!readerMenuOpen) {
      return;
    }

    const handlePointerDown = (event: MouseEvent) => {
      if (readerMenuRef.current?.contains(event.target as Node)) {
        return;
      }
      setReaderMenuOpen(false);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setReaderMenuOpen(false);
      }
    };

    window.addEventListener('mousedown', handlePointerDown);
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('mousedown', handlePointerDown);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [readerMenuOpen]);

  const statusLabel = format === 'pdf'
    ? pageCount > 0 ? `${page} / ${pageCount}` : 'PDF'
    : epubProgress.percentage !== null ? `${Math.round(epubProgress.percentage * 100)}%` : locator || 'EPUB';
  const epubSource = renderUrl && format === 'epub'
    ? resolveEpubOpen(resourceMeta, renderHandle, renderUrl)
    : null;
  const tocRows = useMemo(() => flattenToc(toc), [toc]);
  const initialEpubCfi = useMemo(() => (filePath ? loadSavedEpubCfi(filePath) : null), [filePath, refreshSeq, resourceVersion]);
  const canHighlight = Boolean(selection && hasHighlightText(selection.text));

  const readerMenu = (
    <div ref={readerMenuRef} className={`absolute right-3 top-3 z-30 ${format === 'epub' && tocOpen ? 'md:right-[17rem]' : ''}`}>
      <IconButton
        size={28}
        className={`bg-editor-bg/95 text-secondary-text shadow-sm hover:text-primary-text ${readerMenuOpen ? 'bg-hover-bg text-primary-text' : ''}`}
        onClick={() => setReaderMenuOpen((open) => !open)}
        title="Reader actions"
        aria-label="Reader actions"
        aria-expanded={readerMenuOpen}
      >
        <MoreHorizontalIcon className="h-4 w-4" />
      </IconButton>
      {readerMenuOpen ? (
        <PopupMenu className="absolute right-0 top-8 w-64 text-secondary-text">
          <div className="flex items-center justify-between px-2 py-1.5 text-xs text-tertiary-text">
            <span>Reading</span>
            <span className="tabular-nums text-secondary-text">{statusLabel}</span>
          </div>
          <PopupMenuSeparator />
          {format === 'epub' ? (
            <>
              <PopupMenuItem onClick={() => setTocOpen((current) => !current)} active={tocOpen}>
                <ListIcon className="h-3.5 w-3.5" />
                <span>{tocOpen ? 'Hide contents' : 'Show contents'}</span>
              </PopupMenuItem>
              <div className="flex items-center justify-between gap-2 px-2 py-1.5 text-sm">
                <span className="text-secondary-text">Text</span>
                <div className="flex items-center gap-1">
                  <IconButton size={24} className="text-secondary-text" onClick={handleEpubFontOut} title="Smaller text" aria-label="Smaller text">
                    <span className="text-[11px] leading-none">A</span>
                  </IconButton>
                  <span className="w-10 text-center text-xs tabular-nums text-secondary-text">{Math.round(epubReaderSettings.fontScale * 100)}%</span>
                  <IconButton size={24} className="text-secondary-text" onClick={handleEpubFontIn} title="Larger text" aria-label="Larger text">
                    <span className="text-base leading-none">A</span>
                  </IconButton>
                </div>
              </div>
              <div className="px-2 py-1">
                <div className="flex h-7 overflow-hidden rounded border border-border">
                  {(['paper', 'sepia', 'night'] as const).map((theme) => (
                    <button
                      key={theme}
                      type="button"
                      className={`min-w-0 flex-1 px-2 text-xs capitalize ${epubReaderSettings.theme === theme ? 'bg-hover-bg text-primary-text' : 'text-secondary-text hover:bg-hover-bg hover:text-primary-text'}`}
                      onClick={() => handleEpubTheme(theme)}
                    >
                      {theme}
                    </button>
                  ))}
                </div>
              </div>
              <PopupMenuSeparator />
            </>
          ) : null}
          {format === 'pdf' ? (
            <>
              <div className="flex items-center justify-between gap-2 px-2 py-1.5 text-sm">
                <span className="text-secondary-text">Zoom</span>
                <div className="flex items-center gap-1">
                  <IconButton size={24} className="text-secondary-text" onClick={handleZoomOut} title="Zoom out" aria-label="Zoom out">
                    <span className="text-base leading-none">-</span>
                  </IconButton>
                  <span className="w-10 text-center text-xs tabular-nums text-secondary-text">{Math.round(zoom * 100)}%</span>
                  <IconButton size={24} className="text-secondary-text" onClick={handleZoomIn} title="Zoom in" aria-label="Zoom in">
                    <PlusIcon className="h-3.5 w-3.5" />
                  </IconButton>
                </div>
              </div>
              <PopupMenuSeparator />
            </>
          ) : null}
          <PopupMenuItem onClick={() => void handleHighlightAndClose()} disabled={!canHighlight}>
            <PlusIcon className="h-3.5 w-3.5" />
            <span>Save highlight</span>
          </PopupMenuItem>
          <PopupMenuItem onClick={handleOpenNotes} disabled={!notePath}>
            <FileIcon className="h-3.5 w-3.5" />
            <span>Open notes</span>
          </PopupMenuItem>
          <PopupMenuItem onClick={handleRefresh}>
            <RefreshIcon className="h-3.5 w-3.5" />
            <span>Refresh</span>
          </PopupMenuItem>
          <PopupMenuItem onClick={() => void handleCopyPath()} disabled={!filePath}>
            <CopyIcon className="h-3.5 w-3.5" />
            <span>Copy path</span>
          </PopupMenuItem>
        </PopupMenu>
      ) : null}
    </div>
  );

  return (
    <div className="relative h-full min-h-0 bg-editor-bg text-primary-text">
      {readerMenuOpen ? (
        <div
          className="fixed inset-0 z-20"
          aria-hidden="true"
          onPointerDown={() => setReaderMenuOpen(false)}
        />
      ) : null}
      {readerMenu}
      <div className="h-full min-h-0 bg-editor-bg">
        {loading ? (
          <div className="flex h-full items-center justify-center text-sm text-secondary-text">Loading book...</div>
        ) : error ? (
          <div className="flex h-full items-center justify-center p-6">
            <div className="max-w-[520px] rounded border border-border bg-editor-bg px-3 py-2 text-sm text-secondary-text">
              {error}
            </div>
          </div>
        ) : epubSource ? (
          <div className="flex h-full min-h-0">
            <div className="min-w-0 flex-1">
              <EpubBookView
                url={epubSource.url}
                openMode={epubSource.mode}
                refreshKey={refreshSeq + resourceVersion}
                initialCfi={initialEpubCfi}
                displayRequest={epubDisplayRequest}
                highlightRequest={epubHighlightRequest}
                highlightNotes={highlightNotes}
                readerSettings={epubReaderSettings}
                onSelectionChange={setSelection}
                onHighlightContextAction={handleHighlightContextAction}
                onProgressChange={handleEpubProgressChange}
                onTocChange={setToc}
                onError={handleError}
              />
            </div>
            {tocOpen ? (
              <aside className="hidden w-64 shrink-0 border-l border-border bg-editor-bg md:flex md:flex-col">
                <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border px-2 text-xs font-medium text-primary-text">
                  <ListIcon className="h-3.5 w-3.5" />
                  <span className="truncate">Contents</span>
                </div>
                <div className="min-h-0 flex-1 overflow-auto py-1">
                  {tocRows.length > 0 ? tocRows.map((item, index) => (
                    <button
                      key={`${item.href}-${index}`}
                      type="button"
                      className="flex min-h-[28px] w-full items-center gap-1 px-2 text-left text-xs text-secondary-text hover:bg-hover-bg hover:text-primary-text"
                      style={{ paddingLeft: `${8 + item.depth * 14}px` }}
                      onClick={() => handleTocNavigate(item.href)}
                    >
                      <ChevronRightIcon className="h-3 w-3 shrink-0" />
                      <span className="truncate">{item.label || item.href}</span>
                    </button>
                  )) : (
                    <div className="px-3 py-2 text-xs text-secondary-text">No contents</div>
                  )}
                </div>
              </aside>
            ) : null}
          </div>
        ) : renderUrl && format === 'pdf' ? (
          <PdfBookView
            url={renderUrl}
            refreshKey={refreshSeq + resourceVersion}
            page={page}
            zoom={zoom}
            highlightRequest={pdfHighlightRequest}
            highlightNotes={highlightNotes}
            onPageChange={setPage}
            onPageCountChange={setPageCount}
            onSelectionChange={setSelection}
            onHighlightContextAction={handleHighlightContextAction}
            onError={handleError}
          />
        ) : null}
      </div>
    </div>
  );
};
