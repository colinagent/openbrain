import assert from 'node:assert/strict';
import test from 'node:test';
import {
  DEFAULT_DISPLAY_LOCALE,
  LOCALE_LABELS,
  normalizeDisplayLocale,
  isDisplayLocale,
} from './locales';

test('normalizeDisplayLocale accepts configured locales', () => {
  assert.equal(normalizeDisplayLocale('en'), 'en');
  assert.equal(normalizeDisplayLocale('zh-CN'), 'zh-CN');
});

test('normalizeDisplayLocale maps system zh variants', () => {
  assert.equal(normalizeDisplayLocale(undefined, 'zh-CN'), 'zh-CN');
  assert.equal(normalizeDisplayLocale(undefined, 'zh-Hans'), 'zh-CN');
});

test('normalizeDisplayLocale falls back to english', () => {
  assert.equal(normalizeDisplayLocale(undefined, 'fr-FR'), DEFAULT_DISPLAY_LOCALE);
  assert.equal(normalizeDisplayLocale('invalid', 'fr-FR'), DEFAULT_DISPLAY_LOCALE);
});

test('locale labels stay native', () => {
  assert.equal(LOCALE_LABELS.en, 'English');
  assert.equal(LOCALE_LABELS['zh-CN'], '简体中文');
  assert.equal(isDisplayLocale('zh-CN'), true);
  assert.equal(isDisplayLocale('zh-TW'), false);
});
