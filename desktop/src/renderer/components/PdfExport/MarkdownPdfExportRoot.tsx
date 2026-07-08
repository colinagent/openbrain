import React, { useEffect, useRef, useState } from 'react';
import { createMarkdownEditor } from '../Editor/codemirror/setup';
import { buildMarkdownPdfExportBootstrapState } from '../../features/markdownPdfExport/bootstrap';
import type { MarkdownPdfExportPayload } from '../../features/markdownPdfExport/types';
import { getWorkspaceStore } from '../../store/appStore';
import { useTabManagerStore } from '../../store/tabManagerStore';
import {
  installPdfExportReadinessTracker,
  PdfExportReadinessTracker,
} from '../../services/pdfExportReadiness';
import {
  MARKDOWN_DOCUMENT_COLUMN_OFFSET,
  setMarkdownEffectiveColumnOffsetCssVar,
} from '../../utils/markdownTextOffset';

function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function waitForAnimationFrame(): Promise<void> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => resolve());
  });
}

async function waitForDocumentFonts(): Promise<void> {
  if (!('fonts' in document) || !document.fonts?.ready) {
    return;
  }
  try {
    await document.fonts.ready;
  } catch {
    // Ignore font readiness failures and continue with the loaded fallback font.
  }
}

async function waitForImages(container: HTMLElement, timeoutMs = 12_000): Promise<void> {
  const images = Array.from(container.querySelectorAll('img')).filter((img) => {
    const src = img.getAttribute('src');
    return Boolean(src) && !img.complete;
  });
  if (images.length === 0) {
    return;
  }

  await Promise.race([
    Promise.all(images.map((img) => new Promise<void>((resolve) => {
      const done = () => {
        img.removeEventListener('load', done);
        img.removeEventListener('error', done);
        resolve();
      };
      img.addEventListener('load', done, { once: true });
      img.addEventListener('error', done, { once: true });
    }))).then(() => {}),
    new Promise<never>((_, reject) => {
      window.setTimeout(() => reject(new Error('Timed out waiting for export images to load')), timeoutMs);
    }),
  ]);
}

async function waitForStableLayout(container: HTMLElement, stableFrames = 2, maxFrames = 120): Promise<void> {
  let previousSignature = '';
  let stableCount = 0;
  let observedFrames = 0;
  while (stableCount < stableFrames && observedFrames < maxFrames) {
    await waitForAnimationFrame();
    const signature = [
      container.clientWidth,
      container.clientHeight,
      container.scrollWidth,
      container.scrollHeight,
    ].join(':');
    if (signature === previousSignature) {
      stableCount += 1;
    } else {
      previousSignature = signature;
      stableCount = 0;
    }
    observedFrames += 1;
  }
}

async function waitForPdfExportReady(container: HTMLElement, tracker: PdfExportReadinessTracker): Promise<void> {
  await waitForAnimationFrame();
  await waitForDocumentFonts();
  await tracker.waitForSettled();
  await waitForAnimationFrame();
  await tracker.waitForSettled();
  await waitForImages(container);
  await waitForStableLayout(container);
  await tracker.waitForSettled();
  await waitForStableLayout(container);
}

function isDocumentMarkdownPath(path: string | undefined): boolean {
  const normalized = (path || '').trim().toLowerCase();
  return normalized.endsWith('.md') || normalized.endsWith('.markdown');
}

export function MarkdownPdfExportRoot() {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [payload, setPayload] = useState<MarkdownPdfExportPayload | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    document.body.classList.add('op-pdf-export-body');

    const init = async () => {
      try {
        const nextPayload = await window.electronAPI?.pdfExport?.getPayload();
        if (!nextPayload) {
          throw new Error('Markdown PDF export payload is unavailable');
        }
        const bootstrap = buildMarkdownPdfExportBootstrapState(nextPayload);
        useTabManagerStore.getState().replaceSession(bootstrap.workspaceSession);
        const store = getWorkspaceStore(bootstrap.workspaceTabId);
        store.setState((state) => ({
          ...state,
          serverUrl: nextPayload.remoteSession?.wsUrl || state.serverUrl,
          ...bootstrap.appStatePatch,
        }));
        if (cancelled) {
          return;
        }
        document.title = nextPayload.title || 'Markdown PDF Export';
        setPayload(nextPayload);
      } catch (nextError) {
        const message = getErrorMessage(nextError);
        if (cancelled) {
          return;
        }
        setError(message);
        window.electronAPI?.pdfExport?.reportError(message);
      }
    };

    void init();

    return () => {
      cancelled = true;
      document.body.classList.remove('op-pdf-export-body');
    };
  }, []);

  useEffect(() => {
    if (!payload || !containerRef.current) {
      return;
    }

    let cancelled = false;
    const tracker = new PdfExportReadinessTracker();
    installPdfExportReadinessTracker(tracker);
    setMarkdownEffectiveColumnOffsetCssVar(
      isDocumentMarkdownPath(payload.sourcePath)
        ? MARKDOWN_DOCUMENT_COLUMN_OFFSET
        : 0
    );

    const editor = createMarkdownEditor(containerRef.current, {
      initialContent: payload.content,
      livePreview: true,
      readOnly: true,
      exportMode: true,
    });

    const finish = async () => {
      try {
        if (!containerRef.current) {
          return;
        }
        await waitForPdfExportReady(containerRef.current, tracker);
        if (cancelled) {
          return;
        }
        window.electronAPI?.pdfExport?.reportReady();
      } catch (nextError) {
        if (cancelled) {
          return;
        }
        const message = getErrorMessage(nextError);
        setError(message);
        window.electronAPI?.pdfExport?.reportError(message);
      }
    };

    void finish();

    return () => {
      cancelled = true;
      installPdfExportReadinessTracker(null);
      editor.destroy();
    };
  }, [payload]);

  return (
    <div className="op-pdf-export-root">
      {error ? (
        <div className="op-pdf-export-error">{error}</div>
      ) : (
        <div ref={containerRef} className="op-pdf-export-editor" />
      )}
    </div>
  );
}
