import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { PDFDocumentLoadingTask, PDFDocumentProxy, PDFPageProxy } from 'pdfjs-dist';
import pdfWorkerUrl from 'pdfjs-dist/build/pdf.worker.mjs?url';
import type { ParsedBookHighlightNote } from './bookNotes';

type PdfHighlightRect = {
  id: string;
  top: number;
  left: number;
  width: number;
  height: number;
};

export type PdfHighlightTargetRect = {
  top: number;
  left: number;
  width: number;
  height: number;
};

export type PdfSelection = {
  text: string;
  locator: string;
  page: number;
  rects: PdfHighlightTargetRect[];
  applyHighlight: () => void;
};

export type PdfHighlightContextAction =
  | { type: 'save'; selection: PdfSelection }
  | { type: 'remove'; page: number; rects: PdfHighlightTargetRect[] };

export type PdfHighlightRequest = {
  page: number;
  rects: PdfHighlightTargetRect[];
  seq: number;
};

type PdfPageSize = {
  page: number;
  width: number;
  height: number;
};

type PdfBookViewProps = {
  url: string;
  refreshKey: number;
  page: number;
  zoom: number;
  highlightRequest?: PdfHighlightRequest | null;
  highlightNotes?: ParsedBookHighlightNote[];
  onPageChange: (page: number) => void;
  onPageCountChange: (pageCount: number) => void;
  onSelectionChange: (selection: PdfSelection | null) => void;
  onHighlightContextAction?: (action: PdfHighlightContextAction) => void;
  onError: (message: string) => void;
};

type PdfJsModule = typeof import('pdfjs-dist');

let pdfJsPromise: Promise<PdfJsModule> | null = null;

async function loadPdfJs(): Promise<PdfJsModule> {
  if (!pdfJsPromise) {
    pdfJsPromise = import('pdfjs-dist').then((module) => {
      module.GlobalWorkerOptions.workerSrc = pdfWorkerUrl;
      return module;
    });
  }
  return pdfJsPromise;
}

function clampPage(page: number, pageCount: number): number {
  return Math.min(Math.max(1, page), Math.max(1, pageCount));
}

function rectsFromSelection(selection: Selection, layer: HTMLElement): PdfHighlightRect[] {
  const layerBounds = layer.getBoundingClientRect();
  const rects: PdfHighlightRect[] = [];
  let index = 0;
  for (let i = 0; i < selection.rangeCount; i += 1) {
    const range = selection.getRangeAt(i);
    for (const rect of Array.from(range.getClientRects())) {
      if (rect.width < 1 || rect.height < 1) {
        continue;
      }
      rects.push({
        id: `${Date.now()}-${index}`,
        top: rect.top - layerBounds.top,
        left: rect.left - layerBounds.left,
        width: rect.width,
        height: rect.height,
      });
      index += 1;
    }
  }
  return rects;
}

function normalizeRects(rects: PdfHighlightRect[], size: PdfPageSize | null): PdfHighlightTargetRect[] {
  if (!size || size.width <= 0 || size.height <= 0) {
    return [];
  }
  return rects.map((rect) => ({
    top: rect.top / size.height,
    left: rect.left / size.width,
    width: rect.width / size.width,
    height: rect.height / size.height,
  }));
}

function denormalizeRects(rects: PdfHighlightTargetRect[], size: PdfPageSize | null, keyPrefix: string): PdfHighlightRect[] {
  if (!size || size.width <= 0 || size.height <= 0) {
    return [];
  }
  return rects
    .map((rect, index) => ({
      id: `${keyPrefix}-${index}`,
      top: rect.top * size.height,
      left: rect.left * size.width,
      width: rect.width * size.width,
      height: rect.height * size.height,
    }))
    .filter((rect) => rect.width > 0 && rect.height > 0);
}

function rectContainsPoint(rect: PdfHighlightRect, left: number, top: number): boolean {
  const tolerance = 2;
  return left >= rect.left - tolerance
    && left <= rect.left + rect.width + tolerance
    && top >= rect.top - tolerance
    && top <= rect.top + rect.height + tolerance;
}

export const PdfBookView: React.FC<PdfBookViewProps> = ({
  url,
  refreshKey,
  page,
  zoom,
  highlightRequest = null,
  highlightNotes = [],
  onPageChange,
  onPageCountChange,
  onSelectionChange,
  onHighlightContextAction,
  onError,
}) => {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const textLayerRef = useRef<HTMLDivElement | null>(null);
  const pdfRef = useRef<PDFDocumentProxy | null>(null);
  const pdfJsRef = useRef<PdfJsModule | null>(null);
  const [pageCount, setPageCount] = useState(0);
  const [pageSize, setPageSize] = useState<PdfPageSize | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let loadingTask: PDFDocumentLoadingTask | null = null;
    setLoading(true);
    setPageSize(null);
    onSelectionChange(null);

    loadPdfJs()
      .then((pdfJs) => {
        if (cancelled) {
          return null;
        }
        pdfJsRef.current = pdfJs;
        loadingTask = pdfJs.getDocument({ url });
        return loadingTask.promise;
      })
      .then((pdf) => {
        if (!pdf) {
          return;
        }
        if (cancelled) {
          loadingTask?.destroy();
          return;
        }
        pdfRef.current = pdf;
        setPageCount(pdf.numPages);
        onPageCountChange(pdf.numPages);
        onPageChange(1);
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          onError(error instanceof Error ? error.message : 'Unable to open PDF');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
      onSelectionChange(null);
      pdfRef.current = null;
      loadingTask?.destroy();
    };
  }, [onError, onPageChange, onPageCountChange, onSelectionChange, refreshKey, url]);

  useEffect(() => {
    const pdf = pdfRef.current;
    const canvas = canvasRef.current;
    const textLayerNode = textLayerRef.current;
    if (!pdf || !canvas || !textLayerNode || pageCount <= 0) {
      return undefined;
    }

    let cancelled = false;
    let pageProxy: PDFPageProxy | null = null;
    let textLayer: { cancel: () => void; render: () => Promise<unknown> } | null = null;
    const targetPage = clampPage(page, pageCount);
    if (targetPage !== page) {
      onPageChange(targetPage);
      return undefined;
    }

    setLoading(true);
    onSelectionChange(null);
    textLayerNode.replaceChildren();

    pdf.getPage(targetPage)
      .then(async (nextPage) => {
        if (cancelled) {
          nextPage.cleanup();
          return;
        }
        pageProxy = nextPage;
        const viewport = nextPage.getViewport({ scale: zoom });
        const outputScale = window.devicePixelRatio || 1;
        const context = canvas.getContext('2d');
        if (!context) {
          throw new Error('Canvas is unavailable');
        }

        canvas.width = Math.floor(viewport.width * outputScale);
        canvas.height = Math.floor(viewport.height * outputScale);
        canvas.style.width = `${viewport.width}px`;
        canvas.style.height = `${viewport.height}px`;
        setPageSize({ page: targetPage, width: viewport.width, height: viewport.height });

        await nextPage.render({
          canvas,
          canvasContext: context,
          viewport,
          transform: outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : undefined,
        }).promise;

        if (cancelled) {
          return;
        }
        const pdfJs = pdfJsRef.current || await loadPdfJs();
        textLayer = new pdfJs.TextLayer({
          textContentSource: await nextPage.getTextContent(),
          container: textLayerNode,
          viewport,
        });
        await textLayer.render();
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          onError(error instanceof Error ? error.message : 'Unable to render PDF page');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
      textLayer?.cancel();
      pageProxy?.cleanup();
    };
  }, [onError, onPageChange, onSelectionChange, page, pageCount, zoom]);

  useEffect(() => {
    if (!highlightRequest || pageCount <= 0) {
      return;
    }
    const targetPage = clampPage(highlightRequest.page, pageCount);
    if (targetPage !== page) {
      onPageChange(targetPage);
      return;
    }
    if (pageSize?.page !== targetPage) {
      return;
    }
  }, [highlightRequest, onPageChange, page, pageCount, pageSize]);

  const highlights = useMemo(() => {
    if (!pageSize || pageSize.page !== page) {
      return [];
    }
    const rects: PdfHighlightRect[] = [];
    const seen = new Set<string>();
    const appendRects = (inputRects: PdfHighlightTargetRect[], keyPrefix: string) => {
      const key = JSON.stringify(inputRects);
      if (seen.has(key)) {
        return;
      }
      seen.add(key);
      rects.push(...denormalizeRects(inputRects, pageSize, keyPrefix));
    };
    highlightNotes.forEach((note, index) => {
      if (note.format === 'pdf' && note.page === page && note.rects.length > 0) {
        appendRects(note.rects, `note-${index}`);
      }
    });
    if (highlightRequest?.page === page && highlightRequest.rects.length > 0) {
      appendRects(highlightRequest.rects, `target-${highlightRequest.seq}`);
    }
    return rects;
  }, [highlightNotes, highlightRequest, page, pageSize]);

  const getSelectionSnapshot = useCallback((): PdfSelection | null => {
    const selection = window.getSelection();
    const textLayerNode = textLayerRef.current;
    const text = (selection?.toString() || '').replace(/\s+/g, ' ').trim();
    if (!selection || !textLayerNode || !text) {
      return null;
    }
    const anchorNode = selection.anchorNode;
    const focusNode = selection.focusNode;
    if (
      (anchorNode && !textLayerNode.contains(anchorNode)) ||
      (focusNode && !textLayerNode.contains(focusNode))
    ) {
      return null;
    }
    const rects = rectsFromSelection(selection, textLayerNode);
    if (rects.length === 0) {
      return null;
    }
    return {
      text,
      locator: `Page ${page}`,
      page,
      rects: pageSize?.page === page ? normalizeRects(rects, pageSize) : [],
      applyHighlight: () => {
        selection.removeAllRanges();
      },
    };
  }, [page, pageSize]);

  const handleMouseUp = useCallback(() => {
    onSelectionChange(getSelectionSnapshot());
  }, [getSelectionSnapshot, onSelectionChange]);

  const findHighlightAtPoint = useCallback((clientX: number, clientY: number): ParsedBookHighlightNote | null => {
    const pageNode = textLayerRef.current?.parentElement;
    if (!pageNode || !pageSize || pageSize.page !== page) {
      return null;
    }
    const bounds = pageNode.getBoundingClientRect();
    const left = clientX - bounds.left;
    const top = clientY - bounds.top;
    for (const note of highlightNotes) {
      if (note.format !== 'pdf' || note.page !== page || note.rects.length === 0) {
        continue;
      }
      const rects = denormalizeRects(note.rects, pageSize, 'hit');
      if (rects.some((rect) => rectContainsPoint(rect, left, top))) {
        return note;
      }
    }
    return null;
  }, [highlightNotes, page, pageSize]);

  const handleContextMenu = useCallback((event: React.MouseEvent<HTMLDivElement>) => {
    const selectionSnapshot = getSelectionSnapshot();
    if (selectionSnapshot && selectionSnapshot.rects.length > 0) {
      event.preventDefault();
      onSelectionChange(selectionSnapshot);
      onHighlightContextAction?.({ type: 'save', selection: selectionSnapshot });
      return;
    }

    const note = findHighlightAtPoint(event.clientX, event.clientY);
    if (!note || note.page === null || note.rects.length === 0) {
      return;
    }
    event.preventDefault();
    window.getSelection()?.removeAllRanges();
    onSelectionChange(null);
    onHighlightContextAction?.({ type: 'remove', page: note.page, rects: note.rects });
  }, [findHighlightAtPoint, getSelectionSnapshot, onHighlightContextAction, onSelectionChange]);

  const goPrev = useCallback(() => {
    onPageChange(clampPage(page - 1, pageCount));
  }, [onPageChange, page, pageCount]);

  const goNext = useCallback(() => {
    onPageChange(clampPage(page + 1, pageCount));
  }, [onPageChange, page, pageCount]);

  const pageStyle = pageSize
    ? { width: `${pageSize.width}px`, height: `${pageSize.height}px` }
    : undefined;

  return (
    <div className="flex h-full min-h-0 flex-col bg-[#f5f1e8]">
      <div className="min-h-0 flex-1 overflow-auto px-6 py-6">
        <div className="mx-auto w-fit">
          <div
            className="relative bg-white shadow-[0_10px_30px_rgba(0,0,0,0.18)]"
            style={pageStyle}
            onMouseUp={handleMouseUp}
            onContextMenu={handleContextMenu}
          >
            {loading ? (
              <div className="absolute inset-0 z-30 flex items-center justify-center bg-white/80 text-sm text-secondary-text">
                Loading PDF...
              </div>
            ) : null}
            <canvas ref={canvasRef} className="block" />
            <div ref={textLayerRef} className="textLayer absolute inset-0" />
            <div className="pointer-events-none absolute inset-0">
              {highlights.map((rect) => (
                <span
                  key={rect.id}
                  className="absolute rounded-[2px] bg-[#f6d75f]/50 mix-blend-multiply"
                  style={{
                    top: rect.top,
                    left: rect.left,
                    width: rect.width,
                    height: rect.height,
                  }}
                />
              ))}
            </div>
          </div>
        </div>
      </div>
      <div className="flex h-8 shrink-0 items-center justify-center gap-3 border-t border-border bg-editor-bg text-xs text-secondary-text">
        <button type="button" className="rounded px-2 py-1 hover:bg-hover-bg hover:text-primary-text" onClick={goPrev}>
          Previous
        </button>
        <span className="tabular-nums">{pageCount > 0 ? `${page} / ${pageCount}` : 'PDF'}</span>
        <button type="button" className="rounded px-2 py-1 hover:bg-hover-bg hover:text-primary-text" onClick={goNext}>
          Next
        </button>
      </div>
    </div>
  );
};
