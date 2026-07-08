import { useTabManagerStore } from '../store/tabManagerStore';
import { useAppStore, type RemoteSessionInfo } from '../store/appStore';
import { dirnamePosix, normalizePosixPath } from '../utils/markdownMedia';
import {
  canonicalFileURI,
  fileUrlToAbsolutePath,
  isAbsolutePosixPath,
  isDataUrl,
  isFileUrl,
  isHttpUrl,
  parseCanonicalFileURI,
  type RenderHandle,
  type ResourceMeta,
  type ResourceTarget,
  resourceTargetKey,
} from '../core/resource/uri';

export type ResourceImportPurpose = 'markdown-image' | 'attachment';

type InspectIntent = 'render' | 'download' | 'model';

type ResourceMetaWire = Omit<ResourceMeta, 'target'> & { target: ResourceTarget };

type ResourceRequestPayload = {
  target: ResourceTarget;
  intent: InspectIntent | 'render' | 'download';
  roots: string[];
};
type RenderHandleWire = RenderHandle;
type ResourceImportResult = {
  documentRef: string;
  target: ResourceTarget;
  renderHandle?: RenderHandle;
};

type ResourceImportSession = {
  sessionId: string;
  uploadUrl: string;
  expectedDocumentRef: string;
  provisionalTarget: ResourceTarget;
};

type ResourceGrant = {
  grantToken: string;
  expiresAt: string;
};

const resourceHandleCache = new Map<string, Promise<RenderHandle>>();
const resourceMetaCache = new Map<string, Promise<ResourceMeta>>();
const resourceGrantCache = new Map<string, Promise<ResourceGrant>>();

function getActiveWorkspaceTabId(): string {
  return useTabManagerStore.getState().activeTabId;
}

function resolveBaseUrl(workspaceTabId: string): string {
  const ws = useAppStore.getStoreByTabId(workspaceTabId).getState();
  const port = ws.remoteSession?.localPort;
  return port ? `http://127.0.0.1:${port}` : 'http://127.0.0.1:19530';
}

export function resolveWorkspaceHttpBaseUrl(workspaceTabId = getActiveWorkspaceTabId()): string {
  return resolveBaseUrl(workspaceTabId);
}

function normalizeRemoteAuthoritySeed(session: RemoteSessionInfo | null | undefined): string | null {
  if (!session) return null;
  const hostLabel = (session.hostLabel || '').trim();
  const remoteHome = normalizePosixPath(session.remoteHome || '');
  const installDir = normalizePosixPath(session.installDir || '');
  if (!hostLabel || !remoteHome || !installDir) return null;
  return [hostLabel, remoteHome, installDir].join('|');
}

function buildGrantedRoots(workspaceTabId: string, extraPaths: string[] = []): string[] {
  const ws = useAppStore.getStoreByTabId(workspaceTabId).getState();
  const roots = new Set<string>();
  const push = (value: string | null | undefined) => {
    const normalized = normalizePosixPath((value || '').trim());
    if (normalized && normalized.startsWith('/')) {
      roots.add(normalized);
    }
  };
  push(ws.currentDir);
  push(ws.workspaceRootDir);
  push(ws.agentsRootDir);
  push(ws.baseDir ? `${normalizePosixPath(ws.baseDir)}/resources` : '');
  for (const path of extraPaths) {
    push(path);
    push(dirnamePosix(path));
  }
  return Array.from(roots);
}

export function buildResourceGrantRoots(
  extraPaths: string[] = [],
  workspaceTabId = getActiveWorkspaceTabId(),
): string[] {
  return buildGrantedRoots(workspaceTabId, extraPaths);
}

export function authorityIdForWorkspace(workspaceTabId = getActiveWorkspaceTabId()): string {
  const ws = useAppStore.getStoreByTabId(workspaceTabId).getState();
  const instanceID = (ws.instanceID || '').trim();
  if (instanceID) {
    return instanceID;
  }
  const remoteSeed = normalizeRemoteAuthoritySeed(ws.remoteSession);
  if (remoteSeed) {
    return `remote:${remoteSeed}`;
  }
  return 'local:default';
}

export function canonicalFileURIFromPath(path: string, workspaceTabId = getActiveWorkspaceTabId()): string {
  return canonicalFileURI(authorityIdForWorkspace(workspaceTabId), normalizePosixPath(path));
}

function parseCanonicalFileURIFromClientTarget(target: Extract<ResourceTarget, { kind: 'file' }>): string {
  const prefix = 'opfs://';
  if (!target.uri.startsWith(prefix)) {
    return '';
  }
  const slash = target.uri.indexOf('/', prefix.length);
  return slash >= 0 ? normalizePosixPath(target.uri.slice(slash)) : '';
}

export function resolveResourceTargetFromRef(
  documentPath: string | null,
  rawRef: string,
  workspaceTabId = getActiveWorkspaceTabId(),
): ResourceTarget {
  const trimmed = (rawRef || '').trim();
  if (!trimmed) {
    throw new Error('Resource reference is empty');
  }
  if (isDataUrl(trimmed)) {
    return { kind: 'data', url: trimmed };
  }
  if (isHttpUrl(trimmed)) {
    return { kind: 'external', url: trimmed };
  }
  if (isFileUrl(trimmed)) {
    return { kind: 'file', uri: canonicalFileURIFromPath(fileUrlToAbsolutePath(trimmed), workspaceTabId) };
  }
  const absolutePath = isAbsolutePosixPath(trimmed)
    ? normalizePosixPath(trimmed)
    : normalizePosixPath(`${dirnamePosix(documentPath || '')}/${trimmed}`);
  return { kind: 'file', uri: canonicalFileURIFromPath(absolutePath, workspaceTabId) };
}

async function postJSON<T>(url: string, payload: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const text = (await res.text().catch(() => '')).trim();
    throw new Error(text || `Request failed: ${res.status}`);
  }
  return res.json() as Promise<T>;
}

function handleCacheKey(baseUrl: string, target: ResourceTarget, intent: 'render' | 'download') {
  return `${baseUrl}|${intent}|${resourceTargetKey(target)}`;
}

function normalizeRootsForCache(roots: string[]): string[] {
  return Array.from(
    new Set(
      roots
        .map((root) => normalizePosixPath(root))
        .filter((root) => root.startsWith('/'))
    )
  ).sort();
}

async function ensureGrantToken(
  workspaceTabId: string,
  authority: string,
  roots: string[],
): Promise<string> {
  const baseUrl = resolveBaseUrl(workspaceTabId);
  const normalizedRoots = normalizeRootsForCache(roots);
  const cacheKey = `${baseUrl}|${authority}|${normalizedRoots.join('|')}`;
  const pending = resourceGrantCache.get(cacheKey);
  if (pending) {
    const grant = await pending;
    if (new Date(grant.expiresAt).getTime() - Date.now() > 30_000) {
      return grant.grantToken;
    }
    resourceGrantCache.delete(cacheKey);
  }
  const request = postJSON<ResourceGrant>(`${baseUrl}/v1/resources/grants`, {
    authority,
    roots: normalizedRoots,
  }).finally(() => {
    const current = resourceGrantCache.get(cacheKey);
    if (current === request) {
      resourceGrantCache.delete(cacheKey);
    }
  });
  resourceGrantCache.set(cacheKey, request);
  const grant = await request;
  resourceGrantCache.set(cacheKey, Promise.resolve(grant));
  return grant.grantToken;
}

export async function createResourceGrantToken(
  extraPaths: string[] = [],
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<string> {
  const authority = authorityIdForWorkspace(workspaceTabId);
  const roots = buildGrantedRoots(workspaceTabId, extraPaths);
  return ensureGrantToken(workspaceTabId, authority, roots);
}

async function getRenderUrlForTarget(
  target: ResourceTarget,
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<string> {
  if (target.kind === 'external' || target.kind === 'data') {
    return target.url;
  }
  if (target.kind !== 'file') {
    throw new Error(`Unsupported render target kind: ${target.kind}`);
  }

  const meta = await inspectResource(target, workspaceTabId, 'render');
  if (!meta.exists) {
    throw new Error('Resource does not exist');
  }
  if (!meta.renderable) {
    throw new Error('Resource is not renderable');
  }

  const handle = await getRenderHandle(target, workspaceTabId, 'render');
  const url = new URL(handle.url);
  const normalizedVersion = (meta.lastModified || '').trim();
  if (normalizedVersion) {
    url.searchParams.set('v', normalizedVersion);
  }
  return url.toString();
}


export async function inspectResource(
  target: ResourceTarget,
  workspaceTabId = getActiveWorkspaceTabId(),
  intent: InspectIntent = 'render',
): Promise<ResourceMeta> {
  if (target.kind === 'external') {
    return {
      target,
      name: target.url.split('/').pop() || target.url,
      mimeType: '',
      exists: true,
      renderable: true,
      downloadable: true,
    };
  }
  if (target.kind === 'data') {
    return {
      target,
      name: 'inline',
      mimeType: target.url.slice(5, target.url.indexOf(';')) || '',
      exists: true,
      renderable: true,
      downloadable: false,
    };
  }
  const baseUrl = resolveBaseUrl(workspaceTabId);
  const key = `${baseUrl}|inspect|${intent}|${resourceTargetKey(target)}`;
  const pending = resourceMetaCache.get(key);
  if (pending) return pending;
  const roots = buildGrantedRoots(workspaceTabId, target.kind === 'file' ? [parseCanonicalFileURIFromClientTarget(target)] : []);
  const authority = target.kind === 'file'
    ? parseCanonicalFileURI(target.uri).authorityId
    : authorityIdForWorkspace(workspaceTabId);
  const grantToken = await ensureGrantToken(workspaceTabId, authority, roots);
  const request = postJSON<ResourceMetaWire>(`${baseUrl}/v1/resources/inspect`, { target, intent, grantToken })
    .then((payload) => payload)
    .finally(() => resourceMetaCache.delete(key));
  resourceMetaCache.set(key, request);
  return request;
}

export async function getRenderHandle(
  target: ResourceTarget,
  workspaceTabId = getActiveWorkspaceTabId(),
  intent: 'render' | 'download' = 'render',
): Promise<RenderHandle> {
  if (target.kind === 'external' || target.kind === 'data') {
    return {
      handleId: resourceTargetKey(target),
      url: target.kind === 'external' ? target.url : target.url,
      expiresAt: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
      intent,
      mimeType: target.kind === 'data' ? (target.url.slice(5, target.url.indexOf(';')) || '') : '',
    };
  }
  const baseUrl = resolveBaseUrl(workspaceTabId);
  const key = handleCacheKey(baseUrl, target, intent);
  const pending = resourceHandleCache.get(key);
  if (pending) {
    return pending;
  }
  const roots = buildGrantedRoots(workspaceTabId, target.kind === 'file' ? [parseCanonicalFileURIFromClientTarget(target)] : []);
  const authority = target.kind === 'file'
    ? parseCanonicalFileURI(target.uri).authorityId
    : authorityIdForWorkspace(workspaceTabId);
  const grantToken = await ensureGrantToken(workspaceTabId, authority, roots);
  const request = postJSON<RenderHandleWire>(`${baseUrl}/v1/resources/handle`, { target, intent, grantToken })
    .then((payload) => ({ ...payload, url: payload.url.startsWith('http') ? payload.url : `${baseUrl}${payload.url}` }))
    .finally(() => {
      const current = resourceHandleCache.get(key);
      if (current === request) {
        resourceHandleCache.delete(key);
      }
    });
  resourceHandleCache.set(key, request);
  return request;
}

async function digestSHA256(blob: Blob): Promise<string> {
  const buffer = await blob.arrayBuffer();
  const digest = await crypto.subtle.digest('SHA-256', buffer);
  return Array.from(new Uint8Array(digest)).map((item) => item.toString(16).padStart(2, '0')).join('');
}

export async function importFile(
  input: {
    purpose: ResourceImportPurpose;
    targetDocumentPath: string;
    file: File;
  },
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<ResourceImportResult> {
  const baseUrl = resolveBaseUrl(workspaceTabId);
  const targetDocumentURI = canonicalFileURIFromPath(input.targetDocumentPath, workspaceTabId);
  const roots = buildGrantedRoots(workspaceTabId, [input.targetDocumentPath]);
  const grantToken = await ensureGrantToken(workspaceTabId, authorityIdForWorkspace(workspaceTabId), roots);
  const session = await postJSON<ResourceImportSession>(`${baseUrl}/v1/resources/import-sessions`, {
    purpose: input.purpose,
    targetDocumentURI,
    fileName: input.file.name || 'file',
    mimeType: input.file.type || 'application/octet-stream',
    size: input.file.size,
    sha256: await digestSHA256(input.file),
    grantToken,
  });
  const uploadUrl = session.uploadUrl.startsWith('http') ? session.uploadUrl : `${baseUrl}${session.uploadUrl}`;
  const uploadRes = await fetch(uploadUrl, {
    method: 'PUT',
    headers: { 'Content-Type': input.file.type || 'application/octet-stream' },
    body: input.file,
  });
  if (!uploadRes.ok) {
    const text = (await uploadRes.text().catch(() => '')).trim();
    throw new Error(text || `Import upload failed: ${uploadRes.status}`);
  }
  const result = await uploadRes.json() as ResourceImportResult;
  if (result.renderHandle && !result.renderHandle.url.startsWith('http')) {
    result.renderHandle = { ...result.renderHandle, url: `${baseUrl}${result.renderHandle.url}` };
  }
  return result;
}

export async function getRenderUrlForPhysicalPath(
  physicalPath: string,
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<string> {
  return getRenderUrlForTarget(
    { kind: 'file', uri: canonicalFileURIFromPath(physicalPath, workspaceTabId) },
    workspaceTabId,
  );
}

export async function getRenderHandleForPhysicalPath(
  physicalPath: string,
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<{ url: string; meta: ResourceMeta; handle: RenderHandle }> {
  const target = { kind: 'file', uri: canonicalFileURIFromPath(physicalPath, workspaceTabId) } satisfies ResourceTarget;
  const meta = await inspectResource(target, workspaceTabId, 'render');
  if (!meta.exists) {
    throw new Error('Resource does not exist');
  }
  if (!meta.renderable) {
    throw new Error('Resource is not renderable');
  }
  const handle = await getRenderHandle(target, workspaceTabId, 'render');
  const url = new URL(handle.url);
  const normalizedVersion = (meta.lastModified || '').trim();
  if (normalizedVersion) {
    url.searchParams.set('v', normalizedVersion);
  }
  return { url: url.toString(), meta, handle };
}

export async function getRenderUrlForReference(
  documentPath: string | null,
  rawRef: string,
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<string> {
  const target = resolveResourceTargetFromRef(documentPath, rawRef, workspaceTabId);
  return getRenderUrlForTarget(target, workspaceTabId);
}

export async function resolveLooseResourceUrl(
  value: string | null | undefined,
  workspaceTabId = getActiveWorkspaceTabId(),
): Promise<string> {
  const trimmed = (value || '').trim();
  if (!trimmed) return '';
  if (isHttpUrl(trimmed) || isDataUrl(trimmed)) return trimmed;
  if (isFileUrl(trimmed)) {
    return getRenderUrlForPhysicalPath(fileUrlToAbsolutePath(trimmed), workspaceTabId);
  }
  if (isAbsolutePosixPath(trimmed)) {
    return getRenderUrlForPhysicalPath(trimmed, workspaceTabId);
  }
  return trimmed;
}

export function base64ToFile(base64: string, fileName: string, mimeType: string): File {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return new File([bytes], fileName, { type: mimeType });
}
