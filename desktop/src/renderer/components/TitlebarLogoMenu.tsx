import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  DISPLAY_LOCALES,
  LOCALE_LABELS,
  normalizeDisplayLocale,
  type DisplayLocale,
} from '../../main/i18n/locales';
import { setRendererI18nLocale } from '../../main/i18n/renderer';
import { showLoginRequiredDialog } from '../store/loginRequiredStore';
import { useAuthStore } from '../store/authStore';
import { useAppStore } from '../store/appStore';
import { useUiStore } from '../store/uiStore';
import { resolveHostLabel, type SshHost } from '../store/tabManagerStore';
import {
  AgentBrainIcon,
  AppWindowIcon,
  ChevronRightIcon,
  LanguagesIcon,
  LogInIcon,
  LogOutIcon,
  PaintBrushIcon,
  PlusIcon,
  SettingsIcon,
  UserIcon,
  WorkspaceIcon,
} from './Icons';
import { useDismissOnOutsideInteraction } from '../hooks/useDismissOnOutsideInteraction';
import { PopupMenu, PopupMenuItem, PopupMenuSeparator } from './PopupMenu';
import { TitlebarUserAvatar } from './TitlebarUserAvatar';

type LogoPanel = 'window' | 'theme' | 'language';

type WindowInfo = {
  id: number;
  sessionId: string;
  label: string;
  mode: 'local' | 'remote';
  presentation: 'default' | 'newWindowLanding';
  authRequired?: boolean;
  workspaceId: string;
  workspacePath?: string;
  remoteHost?: SshHost;
  active: boolean;
};

type TitlebarLogoMenuProps = {
  currentWindowId: number | null;
  windowActive: boolean;
  onOpenSettings: () => void | Promise<void>;
};

function MenuRowWithFlyout({
  panel,
  expectedPanel,
  scrollable = false,
  trigger,
  children,
}: {
  panel: LogoPanel | null;
  expectedPanel: LogoPanel;
  scrollable?: boolean;
  trigger: React.ReactNode;
  children: React.ReactNode;
}) {
  const isOpen = panel === expectedPanel;

  return (
    <div className="relative">
      {trigger}
      {isOpen && (
        <PopupMenu
          plain
          className={[
            'absolute left-full top-0 ml-1 z-50 w-[200px] no-drag text-secondary-text shadow-lg',
            scrollable ? 'max-h-[60vh] overflow-auto' : '',
          ].filter(Boolean).join(' ')}
        >
          {children}
        </PopupMenu>
      )}
    </div>
  );
}

export function TitlebarLogoMenu({
  currentWindowId,
  windowActive,
  onOpenSettings,
}: TitlebarLogoMenuProps) {
  const { t, i18n } = useTranslation(['menu']);
  const displayLocale = normalizeDisplayLocale(i18n.language);
  const [menuOpen, setMenuOpen] = useState(false);
  const [panel, setPanel] = useState<LogoPanel | null>(null);
  const [windowList, setWindowList] = useState<WindowInfo[]>([]);
  const menuRef = useRef<HTMLDivElement | null>(null);

  const authLoggedIn = useAuthStore((state) => state.loggedIn);
  const authUID = useAuthStore((state) => state.uid);
  const authEmail = useAuthStore((state) => state.email);
  const authProfile = useAuthStore((state) => state.profile);
  const authLogout = useAuthStore((state) => state.logout);

  const availableThemes = useUiStore((state) => state.availableThemes);
  const themeId = useUiStore((state) => state.themeId);
  const setThemeId = useUiStore((state) => state.setThemeId);
  const activeThemeIndex = availableThemes.findIndex((theme) => theme.id === themeId);
  const activeTheme = availableThemes[activeThemeIndex] || availableThemes[0];

  const {
    openDashboardTab,
    openModelsTab,
    openDesktopSettingsTab,
  } = useAppStore();

  const closeMenu = useCallback(() => {
    setMenuOpen(false);
    setPanel(null);
  }, []);

  const togglePanel = useCallback((next: LogoPanel) => {
    setPanel((current) => (current === next ? null : next));
  }, []);

  const handleLogin = useCallback(() => {
    showLoginRequiredDialog('chat');
    closeMenu();
  }, [closeMenu]);

  const handleLogout = useCallback(async () => {
    closeMenu();
    await authLogout();
  }, [authLogout, closeMenu]);

  const handleThemeSelect = useCallback((nextThemeId: string) => {
    if (!availableThemes.length) {
      return;
    }
    setThemeId(nextThemeId);
    window.electronAPI?.settings.set({ ui: { themeId: nextThemeId } });
    closeMenu();
  }, [availableThemes.length, closeMenu, setThemeId]);

  const handleDisplayLocaleSelect = useCallback((nextLocale: DisplayLocale) => {
    void setRendererI18nLocale(nextLocale);
    window.electronAPI?.settings.set({ ui: { displayLocale: nextLocale } });
    closeMenu();
  }, [closeMenu]);

  const handleNewWindow = useCallback(async () => {
    await window.electronAPI?.window?.createNew();
  }, []);

  useEffect(() => {
    if (!menuOpen) {
      return;
    }
    const api = window.electronAPI?.window;
    if (!api) {
      return;
    }
    void api.list().then(setWindowList);
    return api.onListChanged(setWindowList);
  }, [menuOpen]);

  useDismissOnOutsideInteraction({
    active: menuOpen,
    onDismiss: closeMenu,
    insideRefs: [menuRef],
  });

  return (
    <div className="titlebar-menu-slot relative z-50 flex no-drag" ref={menuRef}>
      <button
        type="button"
        className="no-drag flex items-center gap-1 px-2 h-full bg-transparent text-secondary-text hover:text-highlight"
        onClick={() => {
          setMenuOpen((open) => !open);
          setPanel(null);
        }}
        title="App menu"
      >
        <TitlebarUserAvatar
          loggedIn={authLoggedIn}
          profile={authProfile}
          email={authEmail}
          uid={authUID}
          size="titlebar"
        />
      </button>
      {menuOpen && (
        <PopupMenu
          plain
          className="absolute left-full top-0 ml-1 z-50 min-w-[180px] overflow-visible text-secondary-text no-drag shadow-lg"
        >
          <MenuRowWithFlyout
            panel={panel}
            expectedPanel="window"
            scrollable
            trigger={(
              <PopupMenuItem
                active={panel === 'window'}
                className="justify-between group"
                onClick={() => togglePanel('window')}
              >
                <div className="flex items-center gap-2">
                  <AppWindowIcon className="w-4 h-4 opacity-70" />
                  <span>{t('menu:window')}</span>
                </div>
                <ChevronRightIcon className="w-3.5 h-3.5 opacity-50 group-hover:opacity-100" />
              </PopupMenuItem>
            )}
          >
            <PopupMenuItem
              onClick={async () => {
                await handleNewWindow();
                closeMenu();
              }}
            >
              <PlusIcon className="w-4 h-4 opacity-70" />
              {t('menu:newWindow')}
            </PopupMenuItem>
            {windowList.length > 0 && (
              <>
                <PopupMenuSeparator />
                {windowList.map((w) => {
                  const isCurrentWindow = currentWindowId != null && w.id === currentWindowId;
                  const isActiveWindow = isCurrentWindow ? windowActive : w.active;
                  return (
                    <PopupMenuItem
                      key={w.id}
                      active={isActiveWindow}
                      className="group"
                      onClick={async () => {
                        await window.electronAPI?.window?.focus(w.id);
                        closeMenu();
                      }}
                      title={w.mode === 'local' ? w.workspacePath : resolveHostLabel(w.remoteHost)}
                    >
                      <AppWindowIcon
                        className={`w-4 h-4 opacity-70 flex-shrink-0 ${isActiveWindow ? 'text-highlight' : ''}`}
                      />
                      <div className="flex flex-col min-w-0 flex-1">
                        <div className="text-sm truncate">{w.label}</div>
                        <div
                          className={`text-xs truncate opacity-70 ${
                            isActiveWindow ? 'text-highlight' : 'text-secondary-text group-hover:text-prime-text'
                          }`}
                        >
                          {w.mode === 'local' ? w.workspacePath ?? '' : resolveHostLabel(w.remoteHost)}
                        </div>
                      </div>
                    </PopupMenuItem>
                  );
                })}
              </>
            )}
          </MenuRowWithFlyout>

          <PopupMenuItem
            onClick={() => {
              openDashboardTab();
              closeMenu();
            }}
          >
            <WorkspaceIcon className="w-4 h-4 opacity-70" />
            {t('menu:dashboard')}
          </PopupMenuItem>
          <PopupMenuItem
            onClick={() => {
              closeMenu();
              void onOpenSettings();
            }}
          >
            <SettingsIcon className="w-4 h-4 opacity-70" />
            {t('menu:settings')}
          </PopupMenuItem>

          <MenuRowWithFlyout
            panel={panel}
            expectedPanel="theme"
            trigger={(
              <PopupMenuItem
                active={panel === 'theme'}
                className="justify-between group"
                onClick={() => togglePanel('theme')}
                title={activeTheme ? t('menu:themeWithName', { name: activeTheme.label }) : t('menu:theme')}
              >
                <div className="flex items-center gap-2">
                  <PaintBrushIcon className="w-4 h-4 opacity-70" />
                  <span>{t('menu:theme')}</span>
                </div>
                <ChevronRightIcon className="w-3.5 h-3.5 opacity-50 group-hover:opacity-100" />
              </PopupMenuItem>
            )}
          >
            {availableThemes.map((theme) => (
              <PopupMenuItem
                key={theme.id}
                active={theme.id === activeTheme?.id}
                className="justify-between"
                onClick={() => handleThemeSelect(theme.id)}
              >
                <span>{theme.label}</span>
                {theme.id === activeTheme?.id && <span className="text-highlight opacity-80">✓</span>}
              </PopupMenuItem>
            ))}
          </MenuRowWithFlyout>

          <MenuRowWithFlyout
            panel={panel}
            expectedPanel="language"
            trigger={(
              <PopupMenuItem
                active={panel === 'language'}
                className="justify-between group"
                onClick={() => togglePanel('language')}
                title={LOCALE_LABELS[displayLocale]}
              >
                <div className="flex items-center gap-2">
                  <LanguagesIcon className="w-4 h-4 opacity-70" />
                  <span>{t('menu:language')}</span>
                </div>
                <ChevronRightIcon className="w-3.5 h-3.5 opacity-50 group-hover:opacity-100" />
              </PopupMenuItem>
            )}
          >
            {DISPLAY_LOCALES.map((locale) => (
              <PopupMenuItem
                key={locale}
                active={locale === displayLocale}
                className="justify-between"
                onClick={() => handleDisplayLocaleSelect(locale)}
              >
                <span>{LOCALE_LABELS[locale]}</span>
                {locale === displayLocale && <span className="text-highlight opacity-80">✓</span>}
              </PopupMenuItem>
            ))}
          </MenuRowWithFlyout>

          <PopupMenuItem
            onClick={() => {
              openModelsTab();
              closeMenu();
            }}
          >
            <AgentBrainIcon className="w-4 h-4 opacity-70" />
            {t('menu:models')}
          </PopupMenuItem>
          <PopupMenuItem
            onClick={() => {
              openDesktopSettingsTab();
              closeMenu();
            }}
          >
            <AppWindowIcon className="w-4 h-4 opacity-70" />
            {t('menu:desktop')}
          </PopupMenuItem>
          <PopupMenuSeparator />
          {authLoggedIn ? (
            <>
              <div
                className="px-2 py-1.5 flex items-center gap-2 mb-1"
                title={authEmail || authUID}
              >
                <TitlebarUserAvatar
                  loggedIn={authLoggedIn}
                  profile={authProfile}
                  email={authEmail}
                  uid={authUID}
                  size="menu"
                />
                <div className="flex flex-col min-w-0">
                  <span className="text-sm truncate font-medium text-prime-text">
                    {authProfile?.name || authProfile?.username || authEmail || authUID || t('menu:loggedIn')}
                  </span>
                  {(authProfile?.username || authEmail) && (
                    <span className="text-xs text-secondary-text truncate opacity-70">
                      {authProfile?.username ? `@${authProfile.username}` : authEmail}
                    </span>
                  )}
                </div>
              </div>
              <PopupMenuItem
                onClick={() => {
                  window.open('https://app.openbrain.chat/settings/profile', '_blank', 'noopener');
                  closeMenu();
                }}
              >
                <UserIcon className="w-4 h-4 opacity-70" />
                {t('menu:profiles')}
              </PopupMenuItem>
              <PopupMenuItem
                onClick={() => void handleLogout()}
              >
                <LogOutIcon className="w-4 h-4 opacity-70" />
                {t('menu:logout')}
              </PopupMenuItem>
            </>
          ) : (
            <PopupMenuItem
              onClick={handleLogin}
            >
              <LogInIcon className="w-4 h-4 opacity-70" />
              {t('menu:login')}
            </PopupMenuItem>
          )}
        </PopupMenu>
      )}
    </div>
  );
}
