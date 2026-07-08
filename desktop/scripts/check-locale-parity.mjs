import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const localesRoot = path.resolve(__dirname, '../locales');
const locales = ['en', 'zh-CN'];

function flattenKeys(value, prefix = '') {
  const keys = [];
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return keys;
  }
  for (const [key, child] of Object.entries(value)) {
    const next = prefix ? `${prefix}.${key}` : key;
    if (child && typeof child === 'object' && !Array.isArray(child)) {
      keys.push(...flattenKeys(child, next));
    } else {
      keys.push(next);
    }
  }
  return keys;
}

function loadNamespace(locale, namespace) {
  const filePath = path.join(localesRoot, locale, `${namespace}.json`);
  const raw = fs.readFileSync(filePath, 'utf8');
  return JSON.parse(raw);
}

const namespaces = fs.readdirSync(path.join(localesRoot, 'en'))
  .filter((name) => name.endsWith('.json'))
  .map((name) => name.replace(/\.json$/, ''));

let failed = false;

for (const namespace of namespaces) {
  const byLocale = Object.fromEntries(
    locales.map((locale) => [locale, new Set(flattenKeys(loadNamespace(locale, namespace)))]),
  );
  const [leftLocale, rightLocale] = locales;
  const leftOnly = [...byLocale[leftLocale]].filter((key) => !byLocale[rightLocale].has(key)).sort();
  const rightOnly = [...byLocale[rightLocale]].filter((key) => !byLocale[leftLocale].has(key)).sort();
  if (leftOnly.length || rightOnly.length) {
    failed = true;
    console.error(`[${namespace}] key mismatch`);
    if (leftOnly.length) {
      console.error(`  only in ${leftLocale}:`, leftOnly.join(', '));
    }
    if (rightOnly.length) {
      console.error(`  only in ${rightLocale}:`, rightOnly.join(', '));
    }
  }
}

if (failed) {
  process.exit(1);
}

console.log(`Locale parity OK (${namespaces.length} namespaces, ${locales.join(' + ')})`);
