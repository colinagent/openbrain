import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const sourcePath = new URL('./markdownLinkSource.ts', import.meta.url);
const sourceText = await readFile(sourcePath, 'utf8');
const transpiled = ts.transpileModule(sourceText, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const sourceModule = await import(
  `data:text/javascript;base64,${Buffer.from(transpiled, 'utf8').toString('base64')}`
);
const {
  isInlineMarkdownLinkSource,
  parseInlineMarkdownLinkSource,
} = sourceModule;

test('parses inline markdown links', () => {
  assert.deepEqual(
    parseInlineMarkdownLinkSource('[foo](https://example.com)'),
    {
      label: 'foo',
      target: 'https://example.com',
    }
  );
});

test('treats bare bracket text as non-link source', () => {
  assert.equal(isInlineMarkdownLinkSource('[x]'), false);
  assert.equal(isInlineMarkdownLinkSource('[foo]'), false);
  assert.equal(isInlineMarkdownLinkSource('[foo][bar]'), false);
});
