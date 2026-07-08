import { ThemeTokens, MarkdownThemeConfig, CodeThemeConfig } from './tokens';
import {
  DEFAULT_DARK_CORE,
  DEFAULT_LIGHT_CORE,
  OPENBRAIN_DARK_CORE,
  OPENBRAIN_LIGHT_CORE,
  expandCoreToPalette,
  codeSurfaceBgForScheme,
} from './brandCore';

export type ThemeDefinition = {
  id: string;
  label: string;
  scheme: 'dark' | 'light';
  tokens: ThemeTokens;
};

export type BasePalette = {
  highlight: string;
  background: string;
  /** Sidebar rail, sidebar toolbar, and file tree surface; defaults to secondaryBg/background. */
  sidebarBg?: string;
  /** Chat/conversation surface; defaults to sidebarBg/background. */
  chatBg?: string;
  /** Top title bar; defaults to background. */
  titlebarBg?: string;
  secondaryBg?: string;
  overlayBg?: string;
  tertiaryBg?: string;
  buttonBg: string;
  buttonBgHover: string;
  buttonText: string;
  primeText: string;
  secondaryText: string;
  tertiaryText?: string;
  linkText: string;
  linkTextHover: string;
  hoverBg?: string;
  secondaryHoverBg?: string;
  activeBorder?: string;
  logoLight: string;
  logoDark: string;
  trigramYao: string;
  healthText: string;
  selection?: string;
  /** 选中相同字符高亮背景，不填则用 highlight */
  selectionMatch?: string;
  /** 搜索命中项背景，不填则用 scheme 默认 */
  searchMatchBg?: string;
  /** 搜索命中项前景，不填则用 primeText */
  searchMatchText?: string;
};

const defaultLightPalette = expandCoreToPalette(DEFAULT_LIGHT_CORE, 'light');
const defaultDarkPalette = expandCoreToPalette(DEFAULT_DARK_CORE, 'dark');
const openbrainLightPalette = expandCoreToPalette(OPENBRAIN_LIGHT_CORE, 'light');
const openbrainDarkPalette = expandCoreToPalette(OPENBRAIN_DARK_CORE, 'dark');

const defaultMarkdownEditorFontFamily = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace';
const defaultMarkdownCodeBlockFontFamily = '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace';

function resolveFontFamily(value: string | undefined, fallback: string): string {
  if (typeof value !== 'string') {
    return fallback;
  }
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : fallback;
}

export function mapTokens(
  palette: BasePalette,
  scheme: 'dark' | 'light',
  mdTheme?: MarkdownThemeConfig,
  codeTheme?: CodeThemeConfig
): ThemeTokens {
  const ed = mdTheme?.editor;
  const syn = mdTheme?.syntax;
  const pv = mdTheme?.preview;
  const code = codeTheme?.tokens;
  const hoverBg = palette.hoverBg ?? (scheme === 'dark' ? palette.buttonBgHover : palette.logoLight);
  const previewHighlightBg = pv?.highlightBg ?? palette.selectionMatch ?? palette.selection ?? palette.highlight;
  const previewFrontmatterLink = pv?.frontmatterLink ?? palette.linkText;
  const darkSurfaceBg = palette.secondaryBg ?? palette.background;
  const darkActiveLineBg = palette.secondaryHoverBg ?? palette.logoLight;

  return {
    background: palette.background,
    secondaryBg: palette.secondaryBg ?? palette.background,
    overlayBg: palette.overlayBg ?? palette.secondaryBg ?? palette.background,
    tertiaryBg: palette.tertiaryBg ?? palette.secondaryBg ?? palette.background,
    buttonBg: palette.buttonBg,
    buttonBgHover: palette.buttonBgHover,
    buttonText: palette.buttonText,
    primeText: palette.primeText,
    secondaryText: palette.secondaryText,
    tertiaryText: palette.tertiaryText ?? palette.secondaryText,
    linkText: palette.linkText,
    linkTextHover: palette.linkTextHover,
    logoLight: palette.logoLight,
    logoDark: palette.logoDark,
    trigramYao: palette.trigramYao,
    healthText: palette.healthText,
    editorBg: ed?.background ?? palette.background,
    editorFg: ed?.foreground ?? palette.primeText,
    sidebarBg: palette.sidebarBg ?? palette.secondaryBg ?? palette.background,
    sidebarFg: scheme === 'dark' ? palette.secondaryText : palette.primeText,
    titlebarBg: palette.titlebarBg ?? palette.background,
    border: palette.logoLight,
    activeBorder: palette.activeBorder ?? palette.logoLight,
    accent: palette.highlight,
    accentHover: palette.highlight,
    hoverBg,
    secondaryHoverBg: palette.secondaryHoverBg
      ?? hoverBg,
    selection: palette.selection ?? palette.highlight,
    selectionMatch: palette.selectionMatch ?? palette.highlight,
    searchMatchBg: palette.searchMatchBg ?? palette.selectionMatch ?? palette.highlight,
    searchMatchText: palette.searchMatchText ?? palette.primeText,
    highlight: palette.highlight,
    scrollbarThumb: palette.logoLight,
    scrollbarThumbHover: palette.logoLight,
    editorCaret: ed?.caret ?? palette.primeText,
    editorActiveLine: ed?.activeLine ?? (scheme === 'dark' ? darkActiveLineBg : palette.logoLight),
    editorGutterBg: ed?.gutterBg ?? palette.background,
    editorGutterBorder: ed?.gutterBorder ?? palette.logoLight,
    editorGutterFg: ed?.gutterFg ?? palette.logoLight,
    editorFontFamily: resolveFontFamily(ed?.fontFamily, defaultMarkdownEditorFontFamily),
    codeKeyword: code?.keyword ?? palette.highlight,
    codeString: code?.string ?? palette.linkText,
    codeNumber: code?.number ?? palette.linkTextHover,
    codeType: code?.type ?? palette.linkTextHover,
    codeFunction: code?.function ?? palette.linkTextHover,
    codeProperty: code?.property ?? palette.primeText,
    codeVariable: code?.variable ?? palette.primeText,
    codeComment: code?.comment ?? palette.secondaryText,
    codeMeta: code?.meta ?? palette.secondaryText,
    codeOperator: code?.operator ?? palette.secondaryText,
    codePunctuation: code?.punctuation ?? palette.secondaryText,
    codeInvalid: code?.invalid ?? palette.highlight,
    syntaxHeading1: syn?.heading1 ?? palette.primeText,
    syntaxHeading2: syn?.heading2 ?? palette.primeText,
    syntaxHeading3: syn?.heading3 ?? palette.primeText,
    syntaxHeading4: syn?.heading4 ?? palette.primeText,
    syntaxEmphasis: syn?.emphasis ?? palette.primeText,
    syntaxStrong: syn?.strong ?? palette.primeText,
    syntaxLink: syn?.link ?? palette.linkText,
    syntaxUrl: syn?.url ?? palette.linkText,
    syntaxQuote: syn?.quote ?? palette.secondaryText,
    syntaxList: syn?.list ?? palette.primeText,
    syntaxHr: syn?.hr ?? palette.logoLight,
    syntaxMeta: syn?.meta ?? palette.secondaryText,
    syntaxComment: syn?.comment ?? palette.secondaryText,
    previewHeading1: pv?.heading1 ?? palette.primeText,
    previewHeading2: pv?.heading2 ?? palette.primeText,
    previewHeading3: pv?.heading3 ?? palette.primeText,
    previewHeading4: pv?.heading4 ?? palette.primeText,
    previewEmphasis: pv?.emphasis ?? palette.primeText,
    previewStrong: pv?.strong ?? palette.primeText,
    previewHighlightBg,
    previewCodeInlineBg: pv?.codeInlineBg ?? (scheme === 'dark' ? darkSurfaceBg : codeSurfaceBgForScheme(palette, scheme)),
    previewCodeInlineText: pv?.codeInlineText ?? palette.secondaryText,
    previewCodeBlockFontFamily: resolveFontFamily(pv?.codeBlockFontFamily, defaultMarkdownCodeBlockFontFamily),
    previewLink: pv?.link ?? palette.linkText,
    previewBlockquoteBorder: pv?.blockquoteBorder ?? palette.logoLight,
    previewBlockquoteText: pv?.blockquoteText ?? palette.secondaryText,
    previewListMarker: pv?.listMarker ?? palette.primeText,
    previewSyntaxVisible: pv?.syntaxVisible ?? palette.secondaryText,
    previewCodeBlockBg: pv?.codeBlockBg ?? (scheme === 'dark' ? darkSurfaceBg : codeSurfaceBgForScheme(palette, scheme)),
    previewCodeBlockText: pv?.codeBlockText ?? palette.secondaryText,
    previewCalloutBg: pv?.calloutBg ?? (scheme === 'dark' ? darkSurfaceBg : palette.logoLight),
    previewCalloutBorder: pv?.calloutBorder ?? palette.logoLight,
    previewCalloutNote: pv?.calloutNote ?? palette.highlight,
    previewCalloutWarning: pv?.calloutWarning ?? palette.highlight,
    previewCalloutInfo: pv?.calloutInfo ?? palette.highlight,
    previewCalloutTip: pv?.calloutTip ?? palette.highlight,
    previewCalloutSuccess: pv?.calloutSuccess ?? palette.highlight,
    previewCalloutDanger: pv?.calloutDanger ?? palette.highlight,
    previewFrontmatterText: pv?.frontmatterText ?? palette.secondaryText,
    previewFrontmatterBg: pv?.frontmatterBg ?? (scheme === 'dark' ? darkSurfaceBg : palette.logoLight),
    previewFrontmatterLink,
    previewTableBorder: pv?.tableBorder ?? palette.logoLight,
    previewTableBg: pv?.tableBg ?? (scheme === 'dark' ? darkSurfaceBg : palette.logoLight),
    previewWikilink: pv?.wikilink ?? palette.linkText,
    previewTaskBorder: pv?.taskBorder ?? palette.logoLight,
    previewTaskBg: pv?.taskBg ?? (scheme === 'dark' ? (palette.tertiaryBg ?? darkSurfaceBg) : palette.logoLight),
    previewTaskCheckedBg: pv?.taskCheckedBg ?? palette.highlight,
    previewTaskCheckedText: pv?.taskCheckedText ?? palette.buttonText,
    previewTaskLineText: pv?.taskLineText ?? palette.primeText,
    previewMathInline: pv?.mathInline ?? palette.highlight,
    previewMathBlockBg: pv?.mathBlockBg ?? (scheme === 'dark' ? darkSurfaceBg : palette.logoLight),
    previewMermaidBg: pv?.mermaidBg ?? (scheme === 'dark' ? darkSurfaceBg : palette.logoLight),
    previewMermaidError: pv?.mermaidError ?? palette.highlight,
    terminalBg: palette.background,
    terminalFg: palette.primeText,
    terminalSelection: palette.selection ?? palette.highlight,
    terminalCursor: palette.primeText,
  };
}

const defaultLightTokens = mapTokens(defaultLightPalette, 'light');
const defaultDarkTokens = mapTokens(defaultDarkPalette, 'dark');
const openbrainLightTokens = mapTokens(openbrainLightPalette, 'light');
const openbrainDarkTokens = mapTokens(openbrainDarkPalette, 'dark');

export const themes: ThemeDefinition[] = [
  {
    id: 'default-light',
    label: 'Default Light',
    scheme: 'light',
    tokens: defaultLightTokens,
  },
  {
    id: 'default-dark',
    label: 'Default Dark',
    scheme: 'dark',
    tokens: defaultDarkTokens,
  },
  {
    id: 'openbrain-light',
    label: 'OpenBrain Light',
    scheme: 'light',
    tokens: openbrainLightTokens,
  },
  {
    id: 'openbrain-dark',
    label: 'OpenBrain Dark',
    scheme: 'dark',
    tokens: openbrainDarkTokens,
  },
];

export const defaultThemeId = 'default-light';

export function getThemeById(themeId: string) {
  return themes.find((theme) => theme.id === themeId);
}
