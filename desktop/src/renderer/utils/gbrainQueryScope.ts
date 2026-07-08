import type { GBrainQueryScope } from '../store/chatWorkspaceStore';

function clean(value: string | null | undefined): string {
  return (value || '').trim();
}

export function gbrainQueryScopeLabel(scope: GBrainQueryScope): string {
  if (!scope) {
    return '';
  }
  if (scope.kind === 'source') {
    return clean(scope.label) || clean(scope.sourceID) || 'source';
  }
  const username = clean(scope.username);
  return clean(scope.label) || (username ? `@${username}` : clean(scope.ownerUID)) || 'public brain';
}

export function buildGBrainQueryScopePrompt(scope: GBrainQueryScope): string {
  if (!scope) {
    return '';
  }
  if (scope.kind === 'source') {
    const label = gbrainQueryScopeLabel(scope);
    const sourceID = clean(scope.sourceID);
    return [
      '## OpenBrain GBrain Query Scope',
      '',
      `This turn was started from OpenBrain graph scope "${label}".`,
      `Only use GBrain Cloud results from source_id "${sourceID}".`,
      `When calling the gbrain-cloud query tool, include source_id "${sourceID}".`,
      'Do not use search or unscoped query for scoped retrieval unless the user explicitly asks to broaden the scope.',
    ].join('\n');
  }

  const label = gbrainQueryScopeLabel(scope);
  const sources = (scope.sources || [])
    .map((source) => ({
      sourceID: clean(source.sourceID),
      name: clean(source.name),
    }))
    .filter((source) => source.sourceID);
  const sourceLines = sources.length > 0
    ? sources.map((source) => `- ${source.sourceID}${source.name ? ` (${source.name})` : ''}`)
    : ['- No public source IDs were provided. Ask the user to refresh the OpenBrain graph before doing scoped retrieval.'];
  return [
    '## OpenBrain GBrain Query Scope',
    '',
    `This turn was started from OpenBrain graph scope "${label}".`,
    'Limit GBrain Cloud retrieval to these public source IDs:',
    ...sourceLines,
    'When calling the gbrain-cloud query tool, use one allowed source_id at a time and synthesize the results.',
    'Do not use search or unscoped query for scoped retrieval unless the user explicitly asks to broaden the scope.',
  ].join('\n');
}
