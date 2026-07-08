import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { Book, Contents, Location, NavItem, Rendition } from 'epubjs';
import type { ParsedBookHighlightNote } from './bookNotes';

export type EpubSelection = {
  text: string;
  cfiRange: string;
  locator: string;
  applyHighlight: () => void;
};

export type EpubHighlightContextAction =
  | { type: 'save'; selection: EpubSelection }
  | { type: 'remove'; cfi: string };

export type EpubOpenMode = 'epub' | 'directory' | 'opf';

export type EpubReaderSettings = {
  fontScale: number;
  theme: 'paper' | 'sepia' | 'night';
};

export type EpubProgress = {
  locator: string;
  cfi: string;
  percentage: number | null;
};

export type EpubDisplayRequest = {
  target: string;
  seq: number;
};

export type EpubHighlightRequest = {
  cfi: string;
  seq: number;
};

type EpubFactory = typeof import('epubjs').default;
type EpubViewWithPane = {
  pane?: {
    render?: () => void;
  };
};
type RenditionWithAnnotationInternals = {
  getContents?: () => Contents[] | Contents | null;
  views?: () => EpubViewWithPane[];
};

const WHEEL_PAGE_THRESHOLD = 80;
const WHEEL_PAGE_COOLDOWN_MS = 650;
const WHEEL_LISTENER_OPTIONS: AddEventListenerOptions = { passive: false };

let epubFactoryPromise: Promise<EpubFactory> | null = null;

async function loadEpubFactory(): Promise<EpubFactory> {
  if (!epubFactoryPromise) {
    epubFactoryPromise = import('epubjs').then((module) => module.default);
  }
  return epubFactoryPromise;
}

type EpubBookViewProps = {
  url: string;
  openMode: EpubOpenMode;
  refreshKey: number;
  initialCfi?: string | null;
  displayRequest?: EpubDisplayRequest | null;
  highlightRequest?: EpubHighlightRequest | null;
  highlightNotes?: ParsedBookHighlightNote[];
  readerSettings: EpubReaderSettings;
  onSelectionChange: (selection: EpubSelection | null) => void;
  onHighlightContextAction?: (action: EpubHighlightContextAction) => void;
  onProgressChange: (progress: EpubProgress) => void;
  onTocChange: (toc: NavItem[]) => void;
  onError: (message: string) => void;
};

function locationLabel(location: Location | null | undefined): string {
  if (!location?.start) {
    return '';
  }
  const displayed = location.start.displayed;
  if (displayed?.page && displayed?.total) {
    return `Page ${displayed.page} of ${displayed.total}`;
  }
  if (typeof location.start.percentage === 'number' && Number.isFinite(location.start.percentage)) {
    return `${Math.round(location.start.percentage * 100)}%`;
  }
  return location.start.href || location.start.cfi || '';
}

function selectedTextFromContents(contents: Contents | null | undefined): string {
  const selection = contents?.window?.getSelection?.();
  return (selection?.toString() || '').replace(/\s+/g, ' ').trim();
}

function renditionContents(rendition: Rendition | null | undefined): Contents[] {
  const contents = (rendition as unknown as RenditionWithAnnotationInternals | null | undefined)?.getContents?.();
  if (!contents) {
    return [];
  }
  return Array.isArray(contents) ? contents : [contents];
}

function clearRenditionSelections(rendition: Rendition | null | undefined) {
  window.getSelection()?.removeAllRanges();
  for (const contents of renditionContents(rendition)) {
    contents.window?.getSelection?.()?.removeAllRanges();
  }
}

function rectContainsPoint(rect: DOMRect, clientX: number, clientY: number): boolean {
  const tolerance = 2;
  return clientX >= rect.left - tolerance
    && clientX <= rect.right + tolerance
    && clientY >= rect.top - tolerance
    && clientY <= rect.bottom + tolerance;
}

function renderAnnotationPanes(rendition: Rendition | null | undefined) {
  const views = (rendition as unknown as RenditionWithAnnotationInternals | null | undefined)?.views?.() || [];
  for (const view of views) {
    view.pane?.render?.();
  }
}

function isEditableEventTarget(target: EventTarget | null): boolean {
  const element = target as { closest?: (selector: string) => Element | null } | null;
  return Boolean(element?.closest?.('input, textarea, select, [contenteditable="true"], [contenteditable=""]'));
}

function normalizedWheelDelta(delta: number, deltaMode: number): number {
  switch (deltaMode) {
    case WheelEvent.DOM_DELTA_LINE:
      return delta * 16;
    case WheelEvent.DOM_DELTA_PAGE:
      return delta * 800;
    default:
      return delta;
  }
}

function themePalette(theme: EpubReaderSettings['theme']): { body: string; text: string } {
  switch (theme) {
    case 'night':
      return { body: '#151515', text: '#e8e2d8' };
    case 'sepia':
      return { body: '#f4ecd8', text: '#31291f' };
    default:
      return { body: '#fbfaf6', text: '#252525' };
  }
}

function applyReaderTheme(rendition: Rendition, settings: EpubReaderSettings) {
  const palette = themePalette(settings.theme);
  rendition.themes.default({
    body: {
      background: `${palette.body} !important`,
      color: `${palette.text} !important`,
      'font-size': `${Math.round(settings.fontScale * 100)}% !important`,
      'line-height': '1.62 !important',
      'font-family': 'ui-serif, Georgia, Cambria, "Times New Roman", serif !important',
    },
    p: {
      'line-height': '1.62 !important',
    },
    a: {
      color: `${settings.theme === 'night' ? '#8fb7ff' : '#245f9d'} !important`,
    },
    ':target, *:target': {
      background: 'transparent !important',
      'background-color': 'transparent !important',
      outline: 'none !important',
      'box-shadow': 'none !important',
    },
    '::selection': {
      background: '#f6d75f',
    },
  });
}

export const EpubBookView: React.FC<EpubBookViewProps> = ({
  url,
  openMode,
  refreshKey,
  initialCfi = null,
  displayRequest = null,
  highlightRequest = null,
  highlightNotes = [],
  readerSettings,
  onSelectionChange,
  onHighlightContextAction,
  onProgressChange,
  onTocChange,
  onError,
}) => {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const renditionRef = useRef<Rendition | null>(null);
  const readerSettingsRef = useRef(readerSettings);
  const wheelDeltaRef = useRef(0);
  const lastWheelTurnRef = useRef(0);
  const appliedHighlightCfisRef = useRef<Set<string>>(new Set());
  const highlightNotesRef = useRef<ParsedBookHighlightNote[]>(highlightNotes);
  const annotationFrameIdsRef = useRef<number[]>([]);
  const [ready, setReady] = useState(false);

  const cancelScheduledAnnotationRefresh = useCallback(() => {
    for (const frameId of annotationFrameIdsRef.current) {
      window.cancelAnimationFrame(frameId);
    }
    annotationFrameIdsRef.current = [];
  }, []);

  const scheduleAnnotationRefresh = useCallback((rendition: Rendition | null | undefined = renditionRef.current) => {
    if (!rendition) {
      return;
    }
    cancelScheduledAnnotationRefresh();
    const refresh = (remainingFrames: number) => {
      const frameId = window.requestAnimationFrame(() => {
        annotationFrameIdsRef.current = annotationFrameIdsRef.current.filter((id) => id !== frameId);
        if (renditionRef.current !== rendition) {
          return;
        }
        renderAnnotationPanes(rendition);
        if (remainingFrames > 0) {
          refresh(remainingFrames - 1);
        }
      });
      annotationFrameIdsRef.current.push(frameId);
    };
    refresh(2);
  }, [cancelScheduledAnnotationRefresh]);

  useEffect(() => {
    readerSettingsRef.current = readerSettings;
  }, [readerSettings]);

  useEffect(() => {
    highlightNotesRef.current = highlightNotes;
  }, [highlightNotes]);

  const goPrev = useCallback(() => {
    const rendition = renditionRef.current;
    clearRenditionSelections(rendition);
    void rendition?.prev().then(() => scheduleAnnotationRefresh(rendition));
  }, [scheduleAnnotationRefresh]);

  const goNext = useCallback(() => {
    const rendition = renditionRef.current;
    clearRenditionSelections(rendition);
    void rendition?.next().then(() => scheduleAnnotationRefresh(rendition));
  }, [scheduleAnnotationRefresh]);

  const handlePageKeyDown = useCallback((event: KeyboardEvent) => {
    if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || isEditableEventTarget(event.target)) {
      return;
    }

    if (event.key === 'ArrowLeft' || event.key === 'ArrowUp' || event.key === 'PageUp') {
      event.preventDefault();
      goPrev();
    } else if (event.key === 'ArrowRight' || event.key === 'ArrowDown' || event.key === 'PageDown' || event.key === ' ') {
      event.preventDefault();
      goNext();
    }
  }, [goNext, goPrev]);

  const handleWheelPageTurn = useCallback((event: WheelEvent) => {
    if (event.defaultPrevented || event.ctrlKey || event.metaKey || isEditableEventTarget(event.target)) {
      return;
    }

    const deltaY = normalizedWheelDelta(event.deltaY, event.deltaMode);
    const deltaX = normalizedWheelDelta(event.deltaX, event.deltaMode);
    if (!Number.isFinite(deltaY) || Math.abs(deltaY) <= Math.abs(deltaX) || Math.abs(deltaY) < 1) {
      return;
    }

    event.preventDefault();
    const now = performance.now();
    if (now - lastWheelTurnRef.current < WHEEL_PAGE_COOLDOWN_MS) {
      wheelDeltaRef.current = 0;
      return;
    }

    const nextDelta = wheelDeltaRef.current + deltaY;
    if (Math.abs(nextDelta) < WHEEL_PAGE_THRESHOLD) {
      wheelDeltaRef.current = nextDelta;
      return;
    }

    wheelDeltaRef.current = 0;
    lastWheelTurnRef.current = now;
    if (nextDelta > 0) {
      goNext();
    } else {
      goPrev();
    }
  }, [goNext, goPrev]);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) {
      return undefined;
    }

    root.addEventListener('wheel', handleWheelPageTurn, WHEEL_LISTENER_OPTIONS);
    return () => root.removeEventListener('wheel', handleWheelPageTurn, WHEEL_LISTENER_OPTIONS);
  }, [handleWheelPageTurn]);

  useEffect(() => {
    const host = hostRef.current;
    if (!host || !url) {
      return undefined;
    }

    let cancelled = false;
    setReady(false);
    appliedHighlightCfisRef.current.clear();
    onSelectionChange(null);
    onTocChange([]);
    host.replaceChildren();

    const handleRelocated = (location: Location) => {
      onProgressChange({
        locator: locationLabel(location),
        cfi: location?.start?.cfi || '',
        percentage: typeof location?.start?.percentage === 'number' && Number.isFinite(location.start.percentage)
          ? location.start.percentage
          : null,
      });
    };
    let rendition: Rendition | null = null;
    let book: Book | null = null;
    const applyEpubHighlight = (cfi: string) => {
      const trimmed = cfi.trim();
      if (!trimmed || appliedHighlightCfisRef.current.has(trimmed) || !rendition) {
        return;
      }
      rendition.annotations.highlight(
        trimmed,
        {},
        undefined,
        'op-book-epub-highlight',
        { fill: '#f6d75f', 'fill-opacity': '0.45', 'mix-blend-mode': 'multiply' },
      );
      appliedHighlightCfisRef.current.add(trimmed);
      scheduleAnnotationRefresh(rendition);
    };
    const createSelection = (cfiRange: string, contents: Contents): EpubSelection | null => {
      const text = selectedTextFromContents(contents);
      if (!text) {
        return null;
      }
      const current = rendition?.currentLocation() as unknown as Location | null;
      return {
        text,
        cfiRange,
        locator: locationLabel(current) || cfiRange,
        applyHighlight: () => {
          applyEpubHighlight(cfiRange);
          contents.window.getSelection()?.removeAllRanges();
        },
      };
    };
    const handleSelected = (cfiRange: string, contents: Contents) => {
      const nextSelection = createSelection(cfiRange, contents);
      onSelectionChange(nextSelection);
    };
    const findHighlightAtPoint = (contents: Contents, clientX: number, clientY: number): string | null => {
      for (const note of highlightNotesRef.current) {
        const cfi = (note.cfi || '').trim();
        if (!cfi) {
          continue;
        }
        try {
          const range = contents.range(cfi);
          for (const rect of Array.from(range.getClientRects())) {
            if (rect.width > 0 && rect.height > 0 && rectContainsPoint(rect, clientX, clientY)) {
              return cfi;
            }
          }
        } catch {
          // Ignore stale notes that do not map into the currently visible section.
        }
      }
      return null;
    };
    const handleContentContextMenu = (event: MouseEvent, contents: Contents) => {
      const selection = contents.window?.getSelection?.();
      const text = (selection?.toString() || '').replace(/\s+/g, ' ').trim();
      if (selection && selection.rangeCount > 0 && text) {
        const range = selection.getRangeAt(0);
        if (!range.collapsed) {
          const cfiRange = contents.cfiFromRange(range);
          const nextSelection = createSelection(cfiRange, contents);
          if (nextSelection) {
            event.preventDefault();
            onSelectionChange(nextSelection);
            if (highlightNotesRef.current.some((note) => note.cfi === cfiRange)) {
              onHighlightContextAction?.({ type: 'remove', cfi: cfiRange });
            } else {
              onHighlightContextAction?.({ type: 'save', selection: nextSelection });
            }
            return;
          }
        }
      }

      const cfi = findHighlightAtPoint(contents, event.clientX, event.clientY);
      if (!cfi) {
        return;
      }
      event.preventDefault();
      contents.window?.getSelection?.()?.removeAllRanges();
      onSelectionChange(null);
      onHighlightContextAction?.({ type: 'remove', cfi });
    };
    const contentWheelCleanups = new Map<Document, () => void>();
    const attachContentInteractions = (contents: Contents) => {
      const doc = contents.document;
      if (!doc || contentWheelCleanups.has(doc)) {
        return;
      }

      const contextMenuListener = (event: MouseEvent) => handleContentContextMenu(event, contents);
      doc.addEventListener('wheel', handleWheelPageTurn, WHEEL_LISTENER_OPTIONS);
      doc.addEventListener('contextmenu', contextMenuListener);
      contentWheelCleanups.set(doc, () => {
        doc.removeEventListener('wheel', handleWheelPageTurn, WHEEL_LISTENER_OPTIONS);
        doc.removeEventListener('contextmenu', contextMenuListener);
      });
    };

    loadEpubFactory()
      .then((ePub) => {
        if (cancelled) {
          return null;
        }
        book = ePub(url, {
          openAs: openMode,
          replacements: openMode === 'epub' ? 'blobUrl' : 'none',
        });
        rendition = book.renderTo(host, {
          width: '100%',
          height: '100%',
          flow: 'paginated',
          spread: 'none',
        });
        renditionRef.current = rendition;
        applyReaderTheme(rendition, readerSettingsRef.current);
        rendition.on('relocated', handleRelocated);
        rendition.on('selected', handleSelected);
        rendition.on('keydown', handlePageKeyDown);
        rendition.hooks.content.register(attachContentInteractions);
        void book.loaded.navigation
          .then((navigation) => {
            if (!cancelled) {
              onTocChange(navigation.toc || []);
            }
          })
          .catch(() => {
            if (!cancelled) {
              onTocChange([]);
            }
          });
        return rendition.display(initialCfi || undefined);
      })
      .then(() => {
        if (!cancelled) {
          scheduleAnnotationRefresh(rendition);
          setReady(true);
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          onError(error instanceof Error ? error.message : 'Unable to open EPUB');
        }
      });

    return () => {
      cancelled = true;
      onSelectionChange(null);
      onTocChange([]);
      rendition?.off('relocated', handleRelocated);
      rendition?.off('selected', handleSelected);
      rendition?.off('keydown', handlePageKeyDown);
      rendition?.hooks.content.deregister(attachContentInteractions);
      cancelScheduledAnnotationRefresh();
      contentWheelCleanups.forEach((cleanup) => cleanup());
      contentWheelCleanups.clear();
      rendition?.destroy();
      book?.destroy();
      if (renditionRef.current === rendition) {
        renditionRef.current = null;
      }
    };
  }, [
    handlePageKeyDown,
    handleWheelPageTurn,
    initialCfi,
    cancelScheduledAnnotationRefresh,
    onError,
    onProgressChange,
    onHighlightContextAction,
    onSelectionChange,
    onTocChange,
    openMode,
    refreshKey,
    scheduleAnnotationRefresh,
    url,
  ]);

  useEffect(() => {
    const rendition = renditionRef.current;
    if (!rendition) {
      return;
    }
    applyReaderTheme(rendition, readerSettings);
    scheduleAnnotationRefresh(rendition);
  }, [readerSettings, scheduleAnnotationRefresh]);

  useEffect(() => {
    const target = (displayRequest?.target || '').trim();
    const rendition = renditionRef.current;
    if (!target) {
      return;
    }
    clearRenditionSelections(rendition);
    void rendition?.display(target).then(() => scheduleAnnotationRefresh(rendition));
  }, [displayRequest, scheduleAnnotationRefresh]);

  useEffect(() => {
    const cfi = (highlightRequest?.cfi || '').trim();
    const rendition = renditionRef.current;
    if (!cfi || !rendition || !ready) {
      return;
    }
    clearRenditionSelections(rendition);
    void rendition.display(cfi).then(() => {
      if (!appliedHighlightCfisRef.current.has(cfi)) {
        rendition.annotations.highlight(
          cfi,
          {},
          undefined,
          'op-book-epub-highlight',
          { fill: '#f6d75f', 'fill-opacity': '0.45', 'mix-blend-mode': 'multiply' },
        );
        appliedHighlightCfisRef.current.add(cfi);
      }
      scheduleAnnotationRefresh(rendition);
    });
  }, [highlightRequest, ready, scheduleAnnotationRefresh]);

  useEffect(() => {
    const rendition = renditionRef.current;
    if (!rendition || !ready) {
      return;
    }
    const nextCfis = new Set(
      highlightNotes
        .map((note) => (note.cfi || '').trim())
        .filter(Boolean)
    );
    const requestedCfi = (highlightRequest?.cfi || '').trim();
    if (requestedCfi) {
      nextCfis.add(requestedCfi);
    }
    for (const cfi of Array.from(appliedHighlightCfisRef.current)) {
      if (!nextCfis.has(cfi)) {
        rendition.annotations.remove(cfi, 'highlight');
        appliedHighlightCfisRef.current.delete(cfi);
      }
    }
    for (const note of highlightNotes) {
      const cfi = (note.cfi || '').trim();
      if (!cfi || appliedHighlightCfisRef.current.has(cfi)) {
        continue;
      }
      rendition.annotations.highlight(
        cfi,
        {},
        undefined,
        'op-book-epub-highlight',
        { fill: '#f6d75f', 'fill-opacity': '0.45', 'mix-blend-mode': 'multiply' },
      );
      appliedHighlightCfisRef.current.add(cfi);
    }
    scheduleAnnotationRefresh(rendition);
  }, [highlightNotes, highlightRequest, ready, scheduleAnnotationRefresh]);

  useEffect(() => {
    window.addEventListener('keydown', handlePageKeyDown);
    return () => window.removeEventListener('keydown', handlePageKeyDown);
  }, [handlePageKeyDown]);

  const backgroundClass = useMemo(() => {
    switch (readerSettings.theme) {
      case 'night':
        return 'bg-[#111111]';
      case 'sepia':
        return 'bg-[#eadfca]';
      default:
        return 'bg-[#f3f0e8]';
    }
  }, [readerSettings.theme]);

  return (
    <div ref={rootRef} className={`relative h-full min-h-0 ${backgroundClass}`}>
      {!ready ? (
        <div className="absolute inset-0 z-10 flex items-center justify-center text-sm text-secondary-text">
          Loading EPUB...
        </div>
      ) : null}
      <button
        type="button"
        className="absolute left-3 top-1/2 z-20 flex h-10 w-8 -translate-y-1/2 items-center justify-center rounded bg-editor-bg/80 text-secondary-text hover:bg-hover-bg hover:text-primary-text"
        onClick={goPrev}
        title="Previous page"
        aria-label="Previous page"
      >
        <span className="rotate-180 text-lg leading-none">›</span>
      </button>
      <div ref={hostRef} className="h-full min-h-0 px-12 py-8" />
      <button
        type="button"
        className="absolute right-3 top-1/2 z-20 flex h-10 w-8 -translate-y-1/2 items-center justify-center rounded bg-editor-bg/80 text-secondary-text hover:bg-hover-bg hover:text-primary-text"
        onClick={goNext}
        title="Next page"
        aria-label="Next page"
      >
        <span className="text-lg leading-none">›</span>
      </button>
    </div>
  );
};
