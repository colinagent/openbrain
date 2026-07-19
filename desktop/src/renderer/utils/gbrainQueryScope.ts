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
  return '';
}

export function buildGBrainQueryScopePrompt(scope: GBrainQueryScope): string {
  if (!scope) {
    return '';
  }
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
