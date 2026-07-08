import { rendererI18n } from '../../../main/i18n/renderer';

export type SidebarBuiltinView = 'workspace' | 'openbrain' | 'messenger' | 'agents' | 'skills';
export type SidebarView = SidebarBuiltinView | 'search' | 'tools' | 'cron';
export type SidebarRailItemKey = SidebarBuiltinView | 'cron';

export type SidebarRailItemDescriptor = {
  key: SidebarRailItemKey;
  label: string;
};

export function getMainSidebarRailItems(): SidebarRailItemDescriptor[] {
  return [
    { key: 'workspace', label: rendererI18n.t('sidebar:rail.folder') },
    { key: 'openbrain', label: rendererI18n.t('sidebar:rail.openbrain') },
    { key: 'messenger', label: 'Messenger' },
    { key: 'agents', label: rendererI18n.t('sidebar:rail.agents') },
    { key: 'skills', label: rendererI18n.t('sidebar:rail.skills') },
    { key: 'cron', label: 'Cron' },
  ];
}

export function isMainSidebarRailItemActive(view: SidebarView, itemKey: SidebarRailItemKey): boolean {
  return view === itemKey;
}

export function isSidebarMoreRailActive(view: SidebarView, moreMenuOpen: boolean): boolean {
  return moreMenuOpen || view === 'tools';
}
