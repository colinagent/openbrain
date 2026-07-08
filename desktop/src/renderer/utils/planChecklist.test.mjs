import assert from 'node:assert/strict';
import test from 'node:test';

import * as planChecklist from './planChecklist.ts';

test('parses markdown task items from a plan file', () => {
  const content = [
    '# Release Plan',
    '',
    '## Tasks',
    '- [ ] Audit current flow',
    '### Renderer',
    '  - [x] Verify existing session meta',
    '- [ ] Implement UI',
    '',
    '## Findings',
    '- [x] This checkbox must be ignored',
  ].join('\n');

  const snapshot = planChecklist.parsePlanChecklist(content);
  assert.equal(snapshot.ok, true);
  if (!snapshot.ok) {
    throw new Error('expected valid plan checklist');
  }
  assert.equal(snapshot.title, 'Release Plan');
  assert.equal(snapshot.sectionHeading, 'Tasks');
  assert.equal(snapshot.totalCount, 3);
  assert.equal(snapshot.completedCount, 1);
  assert.deepEqual(snapshot.items.map((item) => ({ text: item.text, checked: item.checked })), [
    { text: 'Audit current flow', checked: false },
    { text: 'Verify existing session meta', checked: true },
    { text: 'Implement UI', checked: false },
  ]);
});

test('supports the Chinese task section heading', () => {
  const snapshot = planChecklist.parsePlanChecklist([
    '# 发布计划',
    '',
    '## 任务',
    '- [ ] 修复 Todo parser',
  ].join('\n'));

  assert.equal(snapshot.ok, true);
  if (!snapshot.ok) {
    throw new Error('expected valid Chinese task section');
  }
  assert.equal(snapshot.sectionHeading, '任务');
  assert.equal(snapshot.totalCount, 1);
});

test('reports a missing dedicated task section', () => {
  const snapshot = planChecklist.parsePlanChecklist('- [ ] Ship it', {
    fallbackTitle: 'release-plan.plan',
  });

  assert.deepEqual(snapshot, {
    ok: false,
    title: 'release-plan.plan',
    code: 'missing-task-section',
    error: 'Plan 缺少专用任务区：只支持 `## Tasks` 或 `## 任务`。',
  });
});

test('reports duplicate dedicated task sections', () => {
  const snapshot = planChecklist.parsePlanChecklist([
    '# Release Plan',
    '',
    '## Tasks',
    '- [ ] Ship parser',
    '',
    '## 任务',
    '- [ ] Ship UI',
  ].join('\n'));

  assert.deepEqual(snapshot, {
    ok: false,
    title: 'Release Plan',
    code: 'duplicate-task-section',
    error: 'Plan 只能包含一个专用任务区：`## Tasks` 或 `## 任务`。',
  });
});

test('reports an empty dedicated task section', () => {
  const snapshot = planChecklist.parsePlanChecklist([
    '# Release Plan',
    '',
    '## Tasks',
    '### Notes',
    'No checklist items here.',
  ].join('\n'));

  assert.deepEqual(snapshot, {
    ok: false,
    title: 'Release Plan',
    code: 'empty-task-section',
    error: 'Plan 的专用任务区缺少 checklist 条目。',
  });
});
