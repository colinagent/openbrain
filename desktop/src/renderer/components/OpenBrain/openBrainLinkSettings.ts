const SOURCE_LINK_SETTINGS_KEY = 'openbrain.graph.sourceLinks.v1';

/** Per-source link toggle persisted in renderer localStorage (defaults to linked). */
export type SourceLinkSettings = Record<string, boolean>;

export function readSourceLinkSettings(): SourceLinkSettings {
  try {
    const raw = window.localStorage.getItem(SOURCE_LINK_SETTINGS_KEY);
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== 'object') {
      return {};
    }
    return Object.fromEntries(
      Object.entries(parsed as Record<string, unknown>).filter((entry): entry is [string, boolean] => (
        typeof entry[0] === 'string' && typeof entry[1] === 'boolean'
      )),
    );
  } catch {
    return {};
  }
}

export function writeSourceLinkSettings(settings: SourceLinkSettings): void {
  try {
    window.localStorage.setItem(SOURCE_LINK_SETTINGS_KEY, JSON.stringify(settings));
  } catch {
    // localStorage may be unavailable
  }
}

export function resolveSourceLinked(settings: SourceLinkSettings, sourceID: string): boolean {
  if (!sourceID) {
    return true;
  }
  return settings[sourceID] ?? true;
}
