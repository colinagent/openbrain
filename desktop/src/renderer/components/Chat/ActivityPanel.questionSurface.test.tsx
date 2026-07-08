import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import type { AwaitingUserState } from '../../store/chatWorkspaceStore';
import {
  EmbeddedQuestionsCard,
  advanceAwaitingUserQuestion,
  buildAwaitingQuestionOptionItems,
  canContinueAwaitingUser,
  getCurrentCustomAnswer,
  navigateAwaitingUserQuestion,
  resolveAwaitingUserKeyboardAction,
  selectAwaitingUserOption,
  skipAwaitingUserQuestion,
  updateAwaitingUserCustomAnswer,
} from './ActivityPanel';

function createAwaitingUser(overrides: Partial<AwaitingUserState> = {}): AwaitingUserState {
  return {
    requestID: overrides.requestID || 'req-1',
    questions: overrides.questions || [{
      header: 'Logo',
      question: '你希望完成的「logo 修改」具体是哪种？',
      options: [
        { label: '标题栏：当前 4 格 Logo 改用主题的 logo 色', description: '与 PixelLogo 配色一致' },
        { label: '标题栏：把 4 格 Logo 换成 PixelLogo', description: '与欢迎页/新窗口一致' },
        { label: '应用图标：完善 build/icon 生成链路或图标资源', description: '见 docs/desktop发布.md' },
      ],
      custom: true,
    }],
    currentIndex: overrides.currentIndex ?? 0,
    answers: overrides.answers || [[]],
    customModeByIndex: overrides.customModeByIndex || [false],
    requestedAt: overrides.requestedAt ?? 1,
  };
}

test('buildAwaitingQuestionOptionItems renders provided options plus custom row', () => {
  const items = buildAwaitingQuestionOptionItems(createAwaitingUser());

  assert.deepEqual(
    items.map((item) => [item.badge, item.kind, item.label]),
    [
      ['A', 'option', '标题栏：当前 4 格 Logo 改用主题的 logo 色'],
      ['B', 'option', '标题栏：把 4 格 Logo 换成 PixelLogo'],
      ['C', 'option', '应用图标：完善 build/icon 生成链路或图标资源'],
      ['D', 'custom', 'Other...'],
    ],
  );
});

test('freeform-only questions render an input without an Other option and can continue empty', () => {
  const awaitingUser = createAwaitingUser({
    questions: [{
      header: 'Path',
      question: 'Please provide the Markdown path.',
      options: [],
      custom: true,
    }],
    answers: [[]],
    customModeByIndex: [false],
  });

  assert.deepEqual(buildAwaitingQuestionOptionItems(awaitingUser), []);
  assert.equal(canContinueAwaitingUser(awaitingUser), true);
  assert.deepEqual(
    resolveAwaitingUserKeyboardAction({
      awaitingUser,
      highlightedIndex: 0,
      key: 'Enter',
    }),
    { kind: 'continue' },
  );

  const answered = updateAwaitingUserCustomAnswer(awaitingUser, 'wiki/demo-format.md');
  assert.deepEqual(answered.answers, [['wiki/demo-format.md']]);
  assert.equal(canContinueAwaitingUser(answered), true);

  const html = renderToStaticMarkup(
    <EmbeddedQuestionsCard
      awaitingUser={awaitingUser}
      busy={false}
      error={null}
      highlightedIndex={0}
      onPrevious={() => {}}
      onNext={() => {}}
      onSelectOption={() => {}}
      onCustomAnswerChange={() => {}}
      onSkip={() => {}}
      onCancel={() => {}}
      onContinue={() => {}}
    />,
  );
  assert.match(html, /Answer \(optional\)/);
  assert.doesNotMatch(html, /Other\.\.\./);
});

test('EmbeddedQuestionsCard renders embedded question header, count, options, and footer actions', () => {
  const awaitingUser = createAwaitingUser();
  const html = renderToStaticMarkup(
    <EmbeddedQuestionsCard
      awaitingUser={awaitingUser}
      busy={false}
      error={null}
      highlightedIndex={1}
      onPrevious={() => {}}
      onNext={() => {}}
      onSelectOption={() => {}}
      onCustomAnswerChange={() => {}}
      onSkip={() => {}}
      onCancel={() => {}}
      onContinue={() => {}}
    />,
  );

  assert.match(html, /Questions/);
  assert.match(html, /1 of 1/);
  assert.match(html, /你希望完成的「logo 修改」具体是哪种？/);
  assert.match(html, /Other\.\.\./);
  assert.match(html, /Skip/);
  assert.match(html, /Continue/);
  assert.match(html, /Previous question/);
  assert.match(html, /Next question/);
  assert.match(html, /Cancel questions/);
});

test('selectAwaitingUserOption updates single-select answers', () => {
  const next = selectAwaitingUserOption(createAwaitingUser(), 1);

  assert.deepEqual(next.answers, [['标题栏：把 4 格 Logo 换成 PixelLogo']]);
  assert.equal(next.customModeByIndex[0], false);
  assert.equal(canContinueAwaitingUser(next), true);
});

test('selectAwaitingUserOption enables custom mode and custom answer is required before continue', () => {
  const selectedCustom = selectAwaitingUserOption(createAwaitingUser(), 3);
  assert.equal(selectedCustom.customModeByIndex[0], true);
  assert.equal(canContinueAwaitingUser(selectedCustom), false);

  const answered = updateAwaitingUserCustomAnswer(selectedCustom, '其他：我会在聊天里补充具体需求');
  assert.equal(getCurrentCustomAnswer(answered), '其他：我会在聊天里补充具体需求');
  assert.deepEqual(answered.answers, [['其他：我会在聊天里补充具体需求']]);
  assert.equal(canContinueAwaitingUser(answered), true);
});

test('skipAwaitingUserQuestion clears current answer but preserves question count', () => {
  const skipped = skipAwaitingUserQuestion(createAwaitingUser({
    answers: [['标题栏：把 4 格 Logo 换成 PixelLogo']],
  }));

  assert.deepEqual(skipped.answers, [[]]);
  assert.equal(skipped.currentIndex, 0);
});

test('advanceAwaitingUserQuestion paginates through multiple questions', () => {
  const next = advanceAwaitingUserQuestion(createAwaitingUser({
    questions: [
      {
        header: 'Q1',
        question: 'First question',
        options: [{ label: 'A' }],
        custom: true,
      },
      {
        header: 'Q2',
        question: 'Second question',
        options: [{ label: 'B' }],
        custom: true,
      },
    ],
    answers: [['A'], []],
    customModeByIndex: [false, false],
  }));

  assert.equal(next.currentIndex, 1);
});

test('navigateAwaitingUserQuestion moves between questions without clearing answers', () => {
  const awaitingUser = createAwaitingUser({
    questions: [
      {
        header: 'Q1',
        question: 'First question',
        options: [{ label: 'A' }],
        custom: true,
      },
      {
        header: 'Q2',
        question: 'Second question',
        options: [{ label: 'B' }],
        custom: true,
      },
    ],
    answers: [['A'], ['B']],
    customModeByIndex: [false, false],
  });

  const next = navigateAwaitingUserQuestion(awaitingUser, 1);
  assert.equal(next.currentIndex, 1);
  assert.deepEqual(next.answers, [['A'], ['B']]);

  const prev = navigateAwaitingUserQuestion(next, -1);
  assert.equal(prev.currentIndex, 0);
  assert.deepEqual(prev.answers, [['A'], ['B']]);
});

test('resolveAwaitingUserKeyboardAction supports navigation, selection, continue, and cancel', () => {
  const awaitingUser = createAwaitingUser();

  assert.deepEqual(
    resolveAwaitingUserKeyboardAction({
      awaitingUser,
      highlightedIndex: 0,
      key: 'ArrowRight',
    }),
    { kind: 'navigate', direction: 1 },
  );
  assert.deepEqual(
    resolveAwaitingUserKeyboardAction({
      awaitingUser,
      highlightedIndex: 0,
      key: 'ArrowDown',
    }),
    { kind: 'move', direction: 1 },
  );
  assert.deepEqual(
    resolveAwaitingUserKeyboardAction({
      awaitingUser,
      highlightedIndex: 0,
      key: 'Enter',
    }),
    { kind: 'selectHighlighted' },
  );
  assert.deepEqual(
    resolveAwaitingUserKeyboardAction({
      awaitingUser: createAwaitingUser({
        answers: [['标题栏：当前 4 格 Logo 改用主题的 logo 色']],
      }),
      highlightedIndex: 0,
      key: 'Enter',
    }),
    { kind: 'continue' },
  );
  assert.deepEqual(
    resolveAwaitingUserKeyboardAction({
      awaitingUser,
      highlightedIndex: 0,
      key: 'Escape',
    }),
    { kind: 'cancel' },
  );
});
