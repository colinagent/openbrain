import {
  getRenderUrlForReference,
  resolveResourceTargetFromRef,
} from '../../../../services/resourceService';
import { trackPdfExportTask } from '../../../../services/pdfExportReadiness';
import { resourceTargetKey } from '../../../../core/resource/uri';

const resolvedRenderUrlCache = new Map<string, string>();
const pendingRenderUrlCache = new Map<string, Promise<string>>();

function renderCacheKey(documentPath: string | null | undefined, rawRef: string): string {
  const trimmedRef = (rawRef || '').trim();
  try {
    return resourceTargetKey(resolveResourceTargetFromRef(documentPath || null, trimmedRef));
  } catch {
    return JSON.stringify(['reference', documentPath || '', trimmedRef]);
  }
}

export function peekRenderUrlForReference(documentPath: string | null | undefined, rawRef: string): string | null {
  const cached = resolvedRenderUrlCache.get(renderCacheKey(documentPath, rawRef));
  return cached ?? null;
}

export function loadRenderUrlForReference(documentPath: string | null | undefined, rawRef: string): Promise<string> {
  const key = renderCacheKey(documentPath, rawRef);
  const pending = pendingRenderUrlCache.get(key);
  if (pending) {
    return pending;
  }

  const request = trackPdfExportTask(getRenderUrlForReference(documentPath || null, rawRef))
    .then((url) => {
      resolvedRenderUrlCache.set(key, url);
      return url;
    })
    .finally(() => {
      pendingRenderUrlCache.delete(key);
    });

  pendingRenderUrlCache.set(key, request);
  return request;
}
