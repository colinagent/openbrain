import i18n from 'i18next';
import {
  DEFAULT_DISPLAY_LOCALE,
  displayLocaleToHtmlLang,
  normalizeDisplayLocale,
  type DisplayLocale,
} from './locales';
import { I18N_NAMESPACES, I18N_RESOURCES } from './resources';

let mainInitialized = false;

export function initMainI18n(locale?: DisplayLocale, systemLocale?: string): DisplayLocale {
  const resolved = normalizeDisplayLocale(locale, systemLocale);
  if (!mainInitialized) {
    void i18n.init({
      lng: resolved,
      fallbackLng: DEFAULT_DISPLAY_LOCALE,
      resources: I18N_RESOURCES,
      ns: [...I18N_NAMESPACES],
      defaultNS: 'common',
      interpolation: { escapeValue: false },
      returnNull: false,
    });
    mainInitialized = true;
    return resolved;
  }
  if (i18n.language !== resolved) {
    void i18n.changeLanguage(resolved);
  }
  return resolved;
}

export function setMainI18nLocale(locale: DisplayLocale): void {
  initMainI18n(locale);
  if (i18n.language !== locale) {
    void i18n.changeLanguage(locale);
  }
}

export function getMainI18nLocale(): DisplayLocale {
  return normalizeDisplayLocale(i18n.language);
}

export function mainT(key: string, options?: Record<string, unknown>): string {
  return i18n.t(key, options);
}

export function applyMainDocumentLang(locale: DisplayLocale): void {
  // Main process has no document; reserved for symmetry if needed later.
  void displayLocaleToHtmlLang(locale);
}

export { i18n as mainI18n };
