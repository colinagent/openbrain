import assert from 'node:assert/strict';
import test from 'node:test';

import {
  DEFAULT_DARK_CORE,
  DEFAULT_LIGHT_CORE,
  OPENBRAIN_DARK_CORE,
  OPENBRAIN_LIGHT_CORE,
  expandCoreToCodeTokens,
  expandCoreToMarkdownTheme,
  expandCoreToPalette,
  mergeTokenOverrides,
} from './brandCore.ts';

const V5_PALETTES = {
  'default-light': {
    highlight: '#be7e4a',
    background: '#fcfaf4',
    titlebarBg: '#fcfaf4',
    sidebarBg: '#f2f0ea',
    secondaryBg: '#f2f0ea',
    tertiaryBg: '#faf5f1',
    buttonBg: '#8f5a2e',
    buttonBgHover: '#be7e4a',
    buttonText: '#ffffff',
    primeText: '#000000',
    secondaryText: '#4c4b49',
    tertiaryText: '#4d4d4d',
    linkText: '#000000',
    linkTextHover: '#be7e4a',
    hoverBg: '#edebe5',
    secondaryHoverBg: '#d6d5cf',
    activeBorder: '#be7e4a',
    logoLight: '#e3e1dc',
    logoDark: '#8f5a2e',
    trigramYao: '#000000',
    healthText: '#16a34a',
    selection: '#e8e6e0',
    selectionMatch: '#dedcd7',
    overlayBg: '#ffffff',
    chatBg: '#f2f0ea',
  },
  'default-dark': {
    highlight: '#be7e4a',
    background: '#292929',
    titlebarBg: '#292929',
    sidebarBg: '#242424',
    secondaryBg: '#242424',
    tertiaryBg: '#38322c',
    buttonBg: '#ffffff',
    buttonBgHover: '#be7e4a',
    buttonText: '#000000',
    primeText: '#ffffff',
    secondaryText: '#bfbfbf',
    tertiaryText: '#4d4d4d',
    linkText: '#ffffff',
    linkTextHover: '#be7e4a',
    hoverBg: '#363636',
    secondaryHoverBg: '#3a3a3a',
    activeBorder: '#be7e4a',
    logoLight: '#434343',
    logoDark: '#ffffff',
    trigramYao: '#ffffff',
    healthText: '#4ade80',
    selection: '#474747',
    selectionMatch: '#505050',
    overlayBg: '#262626',
    chatBg: '#242424',
  },
  'openbrain-light': {
    highlight: '#2f8f6b',
    background: '#f4f9f7',
    titlebarBg: '#f4f9f7',
    sidebarBg: '#eaefed',
    secondaryBg: '#eaefed',
    tertiaryBg: '#eef6f3',
    buttonBg: '#17604d',
    buttonBgHover: '#2f8f6b',
    buttonText: '#ffffff',
    primeText: '#000000',
    secondaryText: '#494b4a',
    tertiaryText: '#4d4d4d',
    linkText: '#000000',
    linkTextHover: '#2f8f6b',
    hoverBg: '#e5eae8',
    secondaryHoverBg: '#cfd4d2',
    activeBorder: '#2f8f6b',
    logoLight: '#dce0de',
    logoDark: '#17604d',
    trigramYao: '#000000',
    healthText: '#16a34a',
    selection: '#e0e5e3',
    selectionMatch: '#d7dbd9',
    overlayBg: '#ffffff',
    chatBg: '#eaefed',
  },
  'openbrain-dark': {
    highlight: '#2f8f6b',
    background: '#101816',
    titlebarBg: '#101816',
    sidebarBg: '#0e1513',
    secondaryBg: '#0e1513',
    tertiaryBg: '#13241f',
    buttonBg: '#ffffff',
    buttonBgHover: '#2f8f6b',
    buttonText: '#000000',
    primeText: '#ffffff',
    secondaryText: '#b7bab9',
    tertiaryText: '#4d4d4d',
    linkText: '#ffffff',
    linkTextHover: '#2f8f6b',
    hoverBg: '#1e2624',
    secondaryHoverBg: '#232a29',
    activeBorder: '#2f8f6b',
    logoLight: '#2d3432',
    logoDark: '#ffffff',
    trigramYao: '#ffffff',
    healthText: '#4ade80',
    selection: '#313837',
    selectionMatch: '#3b4240',
    overlayBg: '#0f1614',
    chatBg: '#0e1513',
  },
};

const CORE_FIXTURES = [
  ['default-light', DEFAULT_LIGHT_CORE, 'light'],
  ['default-dark', DEFAULT_DARK_CORE, 'dark'],
  ['openbrain-light', OPENBRAIN_LIGHT_CORE, 'light'],
  ['openbrain-dark', OPENBRAIN_DARK_CORE, 'dark'],
];

for (const [name, core, scheme] of CORE_FIXTURES) {
  test(`expandCoreToPalette matches v5 ${name}`, () => {
    const palette = expandCoreToPalette(core, scheme);
    const expected = V5_PALETTES[name];
    for (const [key, value] of Object.entries(expected)) {
      assert.equal(palette[key], value, `${name}.${key}`);
    }
  });
}

test('mergeTokenOverrides prefers non-empty user values', () => {
  const base = expandCoreToMarkdownTheme(DEFAULT_LIGHT_CORE, 'light');
  const merged = mergeTokenOverrides(base.syntax, { link: '#ff0000', comment: '' });
  assert.equal(merged.link, '#ff0000');
  assert.equal(merged.comment, base.syntax.comment);
  assert.notEqual(merged.heading1, '#ff0000');
});

test('empty editor overrides do not throw', () => {
  const base = expandCoreToMarkdownTheme(DEFAULT_LIGHT_CORE, 'light');
  const merged = mergeTokenOverrides(base.editor, {});
  assert.equal(merged.background, base.editor.background);
});

test('expandCoreToMarkdownTheme uses prime text for default headings', () => {
  const theme = expandCoreToMarkdownTheme(DEFAULT_LIGHT_CORE, 'light');
  assert.equal(theme.syntax?.heading1, '#000000');
  assert.equal(theme.preview?.heading1, '#000000');
  assert.equal(theme.syntax?.heading2, theme.syntax?.heading1);
});

test('expandCoreToCodeTokens produces 12 keys', () => {
  const tokens = expandCoreToCodeTokens(DEFAULT_LIGHT_CORE, 'light');
  assert.equal(Object.keys(tokens).length, 12);
  assert.equal(tokens.keyword, '#8f5a2e');
  assert.equal(tokens.string, '#be7e4a');
});

test('openbrain code tokens use forest green brand', () => {
  const tokens = expandCoreToCodeTokens(OPENBRAIN_LIGHT_CORE, 'light');
  assert.equal(tokens.keyword, '#17604d');
  assert.equal(tokens.string, '#2f8f6b');
});

test('expandCoreToMarkdownTheme uses neutral active line in dark mode', () => {
  const theme = expandCoreToMarkdownTheme(DEFAULT_DARK_CORE, 'dark');
  assert.equal(theme.editor?.activeLine, '#3a3a3a');
  assert.equal(theme.preview?.frontmatterBg, '#242424');
});

test('expandCoreToMarkdownTheme uses subtle neutral code surfaces on light themes', () => {
  const theme = expandCoreToMarkdownTheme(DEFAULT_LIGHT_CORE, 'light');
  assert.equal(theme.preview?.codeInlineBg, '#f7f5ef');
  assert.equal(theme.preview?.codeBlockBg, '#f7f5ef');
  assert.notEqual(theme.preview?.codeBlockBg, '#ffffff');
});

test('expandCoreToMarkdownTheme keeps explicit code surface overrides', () => {
  const base = expandCoreToMarkdownTheme(DEFAULT_LIGHT_CORE, 'light');
  const merged = mergeTokenOverrides(base.preview, { codeBlockBg: '#ffffff', codeInlineBg: '' });
  assert.equal(merged.codeBlockBg, '#ffffff');
  assert.equal(merged.codeInlineBg, base.preview.codeInlineBg);
});
