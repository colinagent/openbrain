export type OpenBrainProviderMode = 'cloud' | 'local';

export type OpenBrainSourceDisplayInput = {
  sourceID: string;
  name: string;
  path?: string;
  bindingStatus?: 'connected' | 'needs_binding';
  bindingReason?: 'unbound' | 'moved' | 'mismatch';
  runtimeReachable?: boolean;
  disabledQueries?: boolean;
  publicAccess?: boolean;
  bindingMode?: 'own' | 'granted';
  effectivePermission?: 'read' | 'write' | 'admin';
  canMutateSource?: boolean;
  publicOwnerUID?: string;
};

export type OpenBrainSourceDisplayMenu = {
  canOpen: boolean;
  canBind: boolean;
  canRemoveFromDevice: boolean;
  canManageCloud: boolean;
};

export type OpenBrainSourceDisplayState = {
  arcLinked: boolean;
  statusText: string;
  detail?: string;
  openable: boolean;
  menu: OpenBrainSourceDisplayMenu;
};

export type OpenBrainSourceDisplayOptions = {
  provider: OpenBrainProviderMode;
  uiLinked: boolean;
};

function canManageOpenBrainSource(
  source: Pick<OpenBrainSourceDisplayInput, 'bindingMode' | 'canMutateSource' | 'effectivePermission' | 'publicOwnerUID'> | null | undefined,
): boolean {
  if (!source) {
    return false;
  }
  if (source.bindingMode === 'granted' || source.publicOwnerUID) {
    return false;
  }
  if (source.effectivePermission === 'read') {
    return false;
  }
  if (typeof source.canMutateSource === 'boolean') {
    return source.canMutateSource;
  }
  return true;
}

function resolveStatusText(source: OpenBrainSourceDisplayInput): string {
  if (source.runtimeReachable === false) {
    return 'Runtime offline';
  }
  if (source.bindingStatus === 'needs_binding') {
    if (source.bindingReason === 'moved') {
      return 'Folder moved';
    }
    if (source.bindingReason === 'mismatch') {
      return 'Repo mismatch';
    }
    return 'Needs binding';
  }
  if (source.disabledQueries === true) {
    return 'Disabled query';
  }
  if (source.publicAccess === true) {
    return 'Public';
  }
  return 'Connected';
}

function resolveDetail(source: OpenBrainSourceDisplayInput): string | undefined {
  if (source.bindingStatus === 'connected' && source.path) {
    return source.path;
  }
  return undefined;
}

function resolveArcLinked(
  source: OpenBrainSourceDisplayInput,
  provider: OpenBrainProviderMode,
  uiLinked: boolean,
): boolean {
  if (provider === 'cloud') {
    return source.disabledQueries !== true && uiLinked;
  }
  return uiLinked;
}

function resolveOpenable(source: OpenBrainSourceDisplayInput): boolean {
  return source.runtimeReachable !== false
    && source.bindingStatus === 'connected'
    && Boolean(source.path);
}

function resolveMenu(
  source: OpenBrainSourceDisplayInput,
  provider: OpenBrainProviderMode,
  openable: boolean,
): OpenBrainSourceDisplayMenu {
  const runtimeReachable = source.runtimeReachable !== false;
  return {
    canOpen: openable,
    canBind: runtimeReachable && source.bindingStatus === 'needs_binding',
    canRemoveFromDevice: runtimeReachable,
    canManageCloud: provider === 'cloud' && canManageOpenBrainSource(source),
  };
}

export function resolveOpenBrainSourceDisplayState(
  source: OpenBrainSourceDisplayInput,
  options: OpenBrainSourceDisplayOptions,
): OpenBrainSourceDisplayState {
  const openable = resolveOpenable(source);
  return {
    arcLinked: resolveArcLinked(source, options.provider, options.uiLinked),
    statusText: resolveStatusText(source),
    detail: resolveDetail(source),
    openable,
    menu: resolveMenu(source, options.provider, openable),
  };
}
