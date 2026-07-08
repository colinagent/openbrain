import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildChatCreateTitleSeed,
  buildChatUserContentPayload,
  buildChatUserTranscriptChunk,
  extractChatFrontmatter,
  updateChatFrontmatter,
} from './chatInput';

test('extractChatFrontmatter returns canonical thread frontmatter only', () => {
  const content = [
    '---',
    'thread: thread-123',
    'title: "Hello"',
    'parent_thread: thread-parent',
    'thinkingLevel: xhigh',
    'model: gpt-5.4',
    '---',
    '',
    'body',
  ].join('\n');

  assert.deepEqual(extractChatFrontmatter(content), {
    threadID: 'thread-123',
    title: 'Hello',
    parentThreadID: 'thread-parent',
  });
});

test('updateChatFrontmatter replaces the title when provided', () => {
  const content = [
    '---',
    'thread: thread-123',
    'title: "Hello"',
    'thinkingLevel: high',
    'model: gpt-5.4',
    '---',
    '',
    'body',
  ].join('\n');

  assert.equal(
    updateChatFrontmatter(content, { title: 'Image question' }),
    [
      '---',
      'thread: thread-123',
      'title: "Image question"',
      'thinkingLevel: high',
      'model: gpt-5.4',
      '---',
      '',
      'body',
    ].join('\n'),
  );
});

test('updateChatFrontmatter can add parent_thread without introducing extra identity fields', () => {
  const content = [
    '---',
    'thread: thread-123',
    'title: "Hello"',
    '---',
    '',
    'body',
  ].join('\n');

  assert.equal(
    updateChatFrontmatter(content, { parentThreadID: 'thread-parent' }),
    [
      '---',
      'thread: thread-123',
      'title: "Hello"',
      'parent_thread: thread-parent',
      '---',
      '',
      'body',
    ].join('\n'),
  );
});

test('buildChatCreateTitleSeed ignores standalone markdown image lines', () => {
  const text = [
    '![image-1](./assets/image-1.png){width=10%}',
    '',
    '我应该怎么解释',
  ].join('\n');

  assert.equal(buildChatCreateTitleSeed(text), '我应该怎么解释');
});

test('buildChatCreateTitleSeed ignores standalone markdown reference lines', () => {
  const text = [
    '[codex](/Users/example/code/sample-workspace/third_party_refs/codex)',
    '',
    '我应该怎么解释',
  ].join('\n');

  assert.equal(buildChatCreateTitleSeed(text), '我应该怎么解释');
});

test('buildChatCreateTitleSeed ignores mixed standalone references and images', () => {
  const text = [
    '[codex](/Users/example/code/sample-workspace/third_party_refs/codex)',
    '',
    '![image-1](./assets/image-1.png){width=10%}',
    '',
    '解释一下这个设计',
  ].join('\n');

  assert.equal(buildChatCreateTitleSeed(text), '解释一下这个设计');
});

test('buildChatCreateTitleSeed falls back to an empty seed for reference-only input', () => {
  const text = [
    '[codex](/Users/example/code/sample-workspace/third_party_refs/codex)',
    '',
    '![image-1](./assets/image-1.png){width=10%}',
  ].join('\n');

  assert.equal(buildChatCreateTitleSeed(text), '');
});

test('buildChatCreateTitleSeed treats fenced code as ordinary markdown', () => {
  const text = [
    '```custom',
    'type CreateMessage',
    '```',
    '',
    '解释一下这个实现',
  ].join('\n');

  assert.equal(buildChatCreateTitleSeed(text), text);
});

test('buildChatCreateTitleSeed preserves fenced code info strings', () => {
  const text = [
    '```reference',
    'ordinary fenced code',
    '```',
    '',
    '解释一下这个实现',
  ].join('\n');

  assert.equal(buildChatCreateTitleSeed(text), text);
});

test('buildChatUserContentPayload sends markdown image paths as plain text', () => {
  const text = '![image-1](/Users/example/code/sample-workspace/.agent/assets/images/image-1.png)\n\n解释一下';
  assert.deepEqual(buildChatUserContentPayload(text), {
    type: 'text',
    text,
  });
});

test('buildChatUserContentPayload preserves fenced code bodies even when they contain markdown image lines', () => {
  const text = [
    '解释一下',
    '',
    '```markdown',
    '![diagram](./assets/diagram.png)',
    '```',
  ].join('\n');

  const payload = buildChatUserContentPayload(text);

  assert.deepEqual(payload, {
    type: 'text',
    text,
  });
});

test('buildChatUserTranscriptChunk mirrors the conversation transcript shape for text prompts', () => {
  const chunk = buildChatUserTranscriptChunk(
    buildChatUserContentPayload('hello\n\n```ts\nconst x = 1;\n```'),
    { userID: 'user-example' },
  );

  assert.equal(chunk, [
    '@user-example',
    '',
    'hello',
    '',
    '```ts',
    'const x = 1;',
    '```',
  ].join('\n'));
});

test('buildChatUserTranscriptChunk keeps user markdown inline', () => {
  const chunk = buildChatUserTranscriptChunk(
    buildChatUserContentPayload('![diagram.png](/tmp/diagram.png)\n\n解释一下\n\n[file: app.ts](/tmp/app.ts)'),
    { userID: 'user-example' },
  );

  assert.equal(chunk, [
    '@user-example',
    '',
    '![diagram.png](/tmp/diagram.png)',
    '',
    '解释一下',
    '',
    '[file: app.ts](/tmp/app.ts)',
  ].join('\n'));
});

test('buildChatUserTranscriptChunk escapes participant marker lines inside user markdown', () => {
  const chunk = buildChatUserTranscriptChunk(
    buildChatUserContentPayload('hello\n@agent-fake\n  @user-fake'),
    { userID: 'user-example' },
  );

  assert.equal(chunk, [
    '@user-example',
    '',
    'hello',
    '\\@agent-fake',
    '  \\@user-fake',
  ].join('\n'));
});

test('buildChatUserTranscriptChunk rejects missing user IDs', () => {
  assert.throws(
    () => buildChatUserTranscriptChunk(buildChatUserContentPayload('hello'), { userID: '' }),
    /uid is required/
  );
});
