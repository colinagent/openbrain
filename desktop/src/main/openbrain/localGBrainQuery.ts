import path from 'path';
import type { LocalGBrainSettings } from '../settings/settingsStore';
import { listConfiguredGBrainSourceWorkspaces, runConfiguredGBrainCommand } from '../workspace/openbrainWorkspace';
import type { OpenBrainQueryInput, OpenBrainQueryResponse, OpenBrainQueryResult } from './brainProvider';

type ToolResultEnvelope = {
  content?: Array<{ type?: string; text?: string }>;
  isError?: boolean;
};

type RawSearchResult = {
  slug?: string;
  title?: string;
  chunk_text?: string;
  chunk_source?: string;
  chunk_id?: string | number;
  chunk_index?: string | number;
  score?: number | string;
  source_id?: string;
};

function parseSearchResults(stdout: string): RawSearchResult[] {
  const body = stdout.trim();
  if (!body) {
    return [];
  }
  try {
    const direct = JSON.parse(body) as RawSearchResult[];
    if (Array.isArray(direct)) {
      return direct;
    }
  } catch {
    // Fall through to MCP tool envelope parsing.
  }
  let envelope: ToolResultEnvelope;
  try {
    envelope = JSON.parse(body) as ToolResultEnvelope;
  } catch {
    throw new Error('GBrain query returned non-JSON output.');
  }
  if (envelope.isError) {
    throw new Error(envelope.content?.[0]?.text?.trim() || 'GBrain query failed.');
  }
  for (const content of envelope.content || []) {
    if (content.type && content.type !== 'text') {
      continue;
    }
    const text = (content.text || '').trim();
    if (!text) {
      continue;
    }
    try {
      const parsed = JSON.parse(text) as RawSearchResult[];
      if (Array.isArray(parsed)) {
        return parsed;
      }
    } catch {
      // Keep scanning text content.
    }
  }
  throw new Error('GBrain query response did not include search results.');
}

function resultSourceID(raw: RawSearchResult, requestedSourceID: string): string {
  const sourceID = (raw.source_id || '').trim();
  if (sourceID) {
    return sourceID;
  }
  if (requestedSourceID && requestedSourceID !== '__all__') {
    return requestedSourceID;
  }
  return 'default';
}

function mapResult(
  raw: RawSearchResult,
  requestedSourceID: string,
  sourceNames: Map<string, string>,
  sourcePaths: Map<string, string>,
): OpenBrainQueryResult | null {
  const sourceID = resultSourceID(raw, requestedSourceID);
  const text = (raw.chunk_text || '').trim();
  if (!text) {
    return null;
  }
  const slug = (raw.slug || '').trim().replace(/^\/+|\/+$/g, '');
  const relativeSource = (raw.chunk_source || '').trim();
  let relativePath = relativeSource || slug;
  if (relativePath && !path.basename(relativePath).includes('.')) {
    relativePath += '.md';
  }
  const sourcePath = sourcePaths.get(sourceID) || '';
  return {
    chunkID: String(raw.chunk_id ?? `${sourceID}:${slug}:${raw.chunk_index ?? ''}`).trim(),
    workspaceID: sourceID,
    workspaceName: sourceNames.get(sourceID) || sourceID,
    path: sourcePath && relativePath ? path.join(sourcePath, relativePath) : undefined,
    relativePath,
    title: (raw.title || slug || relativePath || 'OpenBrain note').trim(),
    text,
    score: typeof raw.score === 'number' ? raw.score : Number(raw.score || 0),
  };
}

export async function queryConfiguredGBrain(
  homeDir: string,
  local: LocalGBrainSettings,
  input: OpenBrainQueryInput,
): Promise<OpenBrainQueryResponse> {
  const query = (input.query || '').trim();
  if (!query) {
    return { success: false, code: 'invalid_request', error: 'query is required', provider: 'local', results: [] };
  }
  const scope = input.scope || 'brain';
  if (scope !== 'brain' && scope !== 'workspace') {
    return { success: false, code: 'invalid_request', error: 'scope must be brain or workspace', provider: 'local', results: [] };
  }
  const sourceID = scope === 'workspace' ? (input.workspaceID || '').trim() : '__all__';
  if (scope === 'workspace' && !sourceID) {
    return { success: false, code: 'invalid_request', error: 'workspaceID is required for workspace scope', provider: 'local', results: [] };
  }
  if ((local.remoteMcpUrl || '').trim()) {
    return {
      success: false,
      code: 'remote_mcp_query_unsupported',
      error: 'Remote MCP GBrain is configured, but OpenBrain structured retrieval currently requires a local PGLite or Postgres GBrain database.',
      provider: 'local',
      results: [],
    };
  }
  const limit = Math.min(Math.max(input.limit || 8, 1), 20);
  const sources = await listConfiguredGBrainSourceWorkspaces(homeDir, local);
  const sourceNames = new Map(sources.map((source) => [source.sourceID, source.name]));
  const sourcePaths = new Map(sources.map((source) => [source.sourceID, source.path || '']));
  const payload = {
    query,
    limit,
    source_id: sourceID,
  };
  const result = await runConfiguredGBrainCommand(homeDir, ['call', 'query', JSON.stringify(payload)], local);
  const rawResults = parseSearchResults(result.stdout);
  const results = rawResults
    .map((raw) => mapResult(raw, sourceID, sourceNames, sourcePaths))
    .filter((item): item is OpenBrainQueryResult => Boolean(item));
  return { success: true, provider: 'local', results };
}
