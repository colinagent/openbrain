import assert from 'node:assert/strict';
import test from 'node:test';

import {
  appendPlanReference,
  buildPlanReferenceBlock,
  buildRelativePlanLink,
} from './planReference';
import {
  buildPlanSkillContext,
  verifyBoundPlanFileSnapshot,
} from './planTurn';
import {
  PLAN_SKILL_SLUG,
  findSkillOptionBySlug,
  resolvePlanSkillShortcutAction,
} from './chatPlanSkillHotkey';

test('buildPlanSkillContext only carries planFilePath', () => {
  assert.deepEqual(
    buildPlanSkillContext({ planFilePath: '/workspace/demo/.agent/context/example.plan.md' }),
    { planFilePath: '/workspace/demo/.agent/context/example.plan.md' }
  );
});

test('buildPlanSkillContext can carry planDir with title for model-selected plan files', () => {
  assert.deepEqual(
    buildPlanSkillContext({
      planDir: '/workspace/demo/.agent/context',
      title: 'Markdown table marker fix',
    }),
    {
      planDir: '/workspace/demo/.agent/context',
      title: 'Markdown table marker fix',
    }
  );
});

test('verifyBoundPlanFileSnapshot accepts an existing markdown plan with a heading and checklist', () => {
  const result = verifyBoundPlanFileSnapshot({
    snapshot: {
      path: '/workspace/demo/.agent/context/example.plan.md',
      exists: true,
      isDir: false,
      content: [
        '# 表格 marker 修复',
        '',
        '修复 markdown 表格 marker 的渲染链路。',
        '',
        '## Tasks',
        '- [ ] 抽共享 inline renderer',
        '',
        '## 验收标准',
        '- [x] 这条不应进入 todo',
      ].join('\n'),
    },
  });

  assert.deepEqual(result, {
    ok: true,
    plan: {
      path: '/workspace/demo/.agent/context/example.plan.md',
      title: '表格 marker 修复',
    },
  });
});

test('verifyBoundPlanFileSnapshot reports precise failures for invalid bound files', () => {
  assert.deepEqual(
    verifyBoundPlanFileSnapshot({
      snapshot: {
        path: '/workspace/demo/.agent/context/missing.plan.md',
        exists: false,
        isDir: false,
        content: null,
      },
    }),
    {
      ok: false,
      error: '绑定的 plan 文件未生成：/workspace/demo/.agent/context/missing.plan.md',
    }
  );

  assert.deepEqual(
    verifyBoundPlanFileSnapshot({
      snapshot: {
        path: '/workspace/demo/.agent/context',
        exists: true,
        isDir: true,
        content: null,
      },
    }),
    {
      ok: false,
      error: '绑定的 plan 路径不是文件：/workspace/demo/.agent/context',
    }
  );

  assert.deepEqual(
    verifyBoundPlanFileSnapshot({
      snapshot: {
        path: '/workspace/demo/.agent/context/empty.plan.md',
        exists: true,
        isDir: false,
        content: '   \n',
      },
    }),
    {
      ok: false,
      error: '绑定的 plan 文件为空：/workspace/demo/.agent/context/empty.plan.md',
    }
  );

  assert.deepEqual(
    verifyBoundPlanFileSnapshot({
      snapshot: {
        path: '/workspace/demo/.agent/context/no-task-section.plan.md',
        exists: true,
        isDir: false,
        content: [
          '# Wrong shape',
          '',
          '- [ ] Only prose and global checkboxes.',
        ].join('\n'),
      },
    }),
    {
      ok: false,
      error: 'plan 文件缺少 `## Tasks` 或 `## 任务` 任务区：/workspace/demo/.agent/context/no-task-section.plan.md',
    }
  );

  assert.deepEqual(
    verifyBoundPlanFileSnapshot({
      snapshot: {
        path: '/workspace/demo/.agent/context/duplicate-task-sections.plan.md',
        exists: true,
        isDir: false,
        content: [
          '# Wrong shape',
          '',
          '## Tasks',
          '- [ ] First',
          '',
          '## 任务',
          '- [ ] Second',
        ].join('\n'),
      },
    }),
    {
      ok: false,
      error: 'plan 文件包含多个 `## Tasks` / `## 任务` 任务区：/workspace/demo/.agent/context/duplicate-task-sections.plan.md',
    }
  );

  assert.deepEqual(
    verifyBoundPlanFileSnapshot({
      snapshot: {
        path: '/workspace/demo/.agent/context/empty-task-section.plan.md',
        exists: true,
        isDir: false,
        content: [
          '# Wrong shape',
          '',
          '## Tasks',
          'No checklist items here.',
        ].join('\n'),
      },
    }),
    {
      ok: false,
      error: 'plan 文件的 `## Tasks` / `## 任务` 任务区缺少 checklist：/workspace/demo/.agent/context/empty-task-section.plan.md',
    }
  );

  assert.deepEqual(
    verifyBoundPlanFileSnapshot({
      snapshot: {
        path: '/workspace/demo/.agent/context/seed.plan.md',
        exists: true,
        isDir: false,
        content: [
          '<!-- openbrain-plan-seed -->',
          '',
          '- [ ] Draft the execution plan',
        ].join('\n'),
      },
    }),
    {
      ok: false,
      error: 'plan 文件仍是旧的占位内容：/workspace/demo/.agent/context/seed.plan.md',
    }
  );
});

test('successful plan turn can append a created plan reference to the chat', () => {
  const verified = verifyBoundPlanFileSnapshot({
    snapshot: {
      path: '/workspace/demo/.agent/context/markdown-table-marker-fix.plan.md',
      exists: true,
      isDir: false,
      content: [
        '# Markdown 表格 marker 修复',
        '',
        '## Tasks',
        '- [ ] 修复 plan 链路',
      ].join('\n'),
    },
  });
  assert.equal(verified.ok, true);
  if (!verified.ok) {
    throw new Error('expected verified plan');
  }

  const chatPath = '/workspace/demo/.agent/chat/marker.md';
  const relativePath = buildRelativePlanLink(chatPath, verified.plan.path);
  const block = `Plan 已创建。\n\n${buildPlanReferenceBlock(verified.plan.title, relativePath)}`;
  const nextChat = appendPlanReference('助手回复', block);

  assert.match(nextChat, /Plan 已创建。/);
  assert.match(nextChat, /\.\.\/plan\/markdown-table-marker-fix\.plan\.md/);
});
