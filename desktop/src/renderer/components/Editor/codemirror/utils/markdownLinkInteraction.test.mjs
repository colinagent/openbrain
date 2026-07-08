import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const sourcePath = new URL('./markdownLinkInteraction.ts', import.meta.url);
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
const { shouldInterceptRenderedMarkdownLinkMouseDown } = sourceModule;

function createMouseEventSnapshot(overrides = {}) {
  return {
    button: 0,
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
    ...overrides,
  };
}

function createClosestTarget(selectors) {
  return {
    closest(selector) {
      return selectors[selector] ?? null;
    },
  };
}

test('rendered inline markdown links intercept unmodified primary mousedown', () => {
  const target = createClosestTarget({
    '.cm-md-link[data-md-link]': { dataset: { mdLink: '../plan/fix-collapse-click-area.md' } },
  });

  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot(), target),
    true
  );
});

test('rendered agent mentions use the same link mousedown interception', () => {
  const target = createClosestTarget({
    '.cm-md-link[data-md-link]': {
      className: 'cm-md-link cm-md-agent-mention',
      dataset: { mdLink: 'agent:agent-coder' },
    },
  });

  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot(), target),
    true
  );
});

test('rendered inline markdown links do not intercept modified clicks', () => {
  const target = createClosestTarget({
    '.cm-md-link[data-md-link]': { dataset: { mdLink: '#heading' } },
  });

  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot({ metaKey: true }), target),
    false
  );
  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot({ ctrlKey: true }), target),
    false
  );
  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot({ altKey: true }), target),
    false
  );
  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot({ shiftKey: true }), target),
    false
  );
});

test('rendered inline markdown links do not intercept non-primary buttons', () => {
  const target = createClosestTarget({
    '.cm-md-link[data-md-link]': { dataset: { mdLink: 'https://example.com' } },
  });

  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot({ button: 1 }), target),
    false
  );
});

test('source-state links and frontmatter links do not use rendered-link interception', () => {
  const sourceStateTarget = createClosestTarget({
    '.cm-md-link[data-md-link]': null,
    '[data-md-link]': { dataset: { mdLink: '../plan/fix-collapse-click-area.md' } },
  });
  const frontmatterTarget = createClosestTarget({
    '.cm-md-link[data-md-link]': null,
    '[data-md-link]': { dataset: { mdLink: 'thread:thread-demo' } },
  });

  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot(), sourceStateTarget),
    false
  );
  assert.equal(
    shouldInterceptRenderedMarkdownLinkMouseDown(createMouseEventSnapshot(), frontmatterTarget),
    false
  );
});
