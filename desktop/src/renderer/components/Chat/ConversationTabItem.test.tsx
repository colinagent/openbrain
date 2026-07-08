import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import { ConversationStatusLight, getConversationStatusLightMeta } from './ConversationStatusLight';
import { ConversationTabItem } from './ConversationTabItem';

test('ConversationStatusLight exposes running, awaiting, and idle semantics', () => {
  assert.deepEqual(getConversationStatusLightMeta('running'), {
    title: 'Conversation running',
    ariaLabel: 'Conversation running',
  });
  assert.deepEqual(getConversationStatusLightMeta('complete'), {
    title: 'AI response complete',
    ariaLabel: 'AI response complete',
  });
  assert.deepEqual(
    getConversationStatusLightMeta('awaiting_user', {
      requestID: 'req-1',
      questions: [{
        header: 'Q1',
        question: 'Please provide the missing detail before I continue.',
        options: [],
        custom: true,
      }],
      currentIndex: 0,
      answers: [[]],
      customModeByIndex: [false],
      requestedAt: 1,
    }),
    {
      title: 'Conversation waiting for your input: Please provide the missing detail before I continue.',
      ariaLabel: 'Conversation waiting for your input: Please provide the missing detail before I continue.',
    },
  );
  assert.equal(getConversationStatusLightMeta('idle'), null);
});

test('ConversationStatusLight renders a stable slot and status data attribute', () => {
  const html = renderToStaticMarkup(
    <ConversationStatusLight
      status="awaiting_user"
      awaitingUser={{
        requestID: 'req-2',
        questions: [{
          header: 'Q1',
          question: 'Need your approval',
          options: [{ label: 'Accept' }],
          custom: false,
        }],
        currentIndex: 0,
        answers: [[]],
        customModeByIndex: [false],
        requestedAt: 1,
      }}
    />,
  );

  assert.match(html, /conversation-status-light-slot/);
  assert.match(html, /conversation-status-light/);
  assert.match(html, /data-status="awaiting_user"/);
  assert.match(html, /title="Conversation waiting for your input: Need your approval"/);
  assert.match(html, /aria-label="Conversation waiting for your input: Need your approval"/);
});

test('ConversationStatusLight renders spinner for running and green status data for complete', () => {
  const runningHtml = renderToStaticMarkup(<ConversationStatusLight status="running" />);
  const completeHtml = renderToStaticMarkup(<ConversationStatusLight status="complete" />);
  const styles = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');

  assert.match(runningHtml, /conversation-status-light-slot/);
  assert.match(runningHtml, /conversation-status-spinner/);
  assert.doesNotMatch(runningHtml, /data-status="running"/);
  assert.match(completeHtml, /conversation-status-light-slot/);
  assert.match(completeHtml, /data-status="complete"/);
  assert.match(
    styles,
    /\.conversation-status-light\[data-status='complete'\]\s*\{[^}]*background-color:\s*var\(--color-status-done\);/m,
  );
  assert.match(styles, /--color-status-done:\s*#2e7d4f;/);
});

test('ConversationStatusLight keeps the slot but hides idle state', () => {
  const html = renderToStaticMarkup(<ConversationStatusLight status="idle" />);

  assert.match(html, /conversation-status-light-slot/);
  assert.match(html, /aria-hidden="true"/);
  assert.doesNotMatch(html, /conversation-status-light"/);
  assert.doesNotMatch(html, /data-status=/);
});

test('ConversationTabItem keeps the status slot for chat and pending tabs and preserves absolute close button', () => {
  const chatTabHtml = renderToStaticMarkup(
    <ConversationTabItem
      title="hello"
      buttonTitle="/tmp/workspace/.agent/chat/hello.md"
      closeLabel="Close hello"
      isSelected={true}
      isPinned={true}
      status="running"
      onSelect={() => {}}
      onClose={() => {}}
    />,
  );
  const openChatTabHtml = renderToStaticMarkup(
    <ConversationTabItem
      title="hello"
      buttonTitle="/tmp/workspace/.agent/chat/hello.md"
      closeLabel="Close hello"
      isSelected={true}
      isOpenInEditor={true}
      isPinned={true}
      status="running"
      onSelect={() => {}}
      onClose={() => {}}
    />,
  );
  const pendingTabHtml = renderToStaticMarkup(
    <ConversationTabItem
      title="New Chat"
      buttonTitle="New Chat"
      closeLabel="Close New Chat"
      isSelected={false}
      status="idle"
      onSelect={() => {}}
      onClose={() => {}}
    />,
  );

  assert.match(chatTabHtml, /conversation-status-light-slot/);
  assert.match(chatTabHtml, /conversation-status-spinner/);
  assert.doesNotMatch(chatTabHtml, /data-status="running"/);
  assert.match(chatTabHtml, /d="M5\.3 2\.8h5\.4L9\.6 6\.4l2\.1 2\.1v1\.4H8\.6L8 14H6\.9l-\.6-4\.1H3\.2V8\.5l2\.1-2\.1-1-3\.6Z"/);
  assert.match(chatTabHtml, /tab-hover-shell/);
  assert.match(chatTabHtml, /conversation-tab-selected/);
  assert.match(chatTabHtml, /conversation-tab-selected-title/);
  assert.match(chatTabHtml, /text-secondary-text/);
  assert.doesNotMatch(chatTabHtml, /text-highlight/);
  assert.match(chatTabHtml, /tab-hover-bg-sync/);
  assert.match(chatTabHtml, /tab-close-btn-delayed/);
  assert.match(chatTabHtml, /tab-close-btn absolute/);
  assert.doesNotMatch(chatTabHtml, /after:bg-border/);
  assert.match(openChatTabHtml, /tab-hover-shell/);
  assert.match(openChatTabHtml, /text-highlight/);
  assert.doesNotMatch(openChatTabHtml, /conversation-tab-selected/);
  assert.match(pendingTabHtml, /conversation-status-light-slot/);
  assert.match(pendingTabHtml, /aria-hidden="true"/);
  assert.doesNotMatch(pendingTabHtml, /d="M5\.3 2\.8h5\.4L9\.6 6\.4l2\.1 2\.1v1\.4H8\.6L8 14H6\.9l-\.6-4\.1H3\.2V8\.5l2\.1-2\.1-1-3\.6Z"/);
  assert.doesNotMatch(pendingTabHtml, /data-status="idle"/);
  assert.match(pendingTabHtml, /tab-hover-shell/);
  assert.doesNotMatch(pendingTabHtml, /conversation-tab-selected/);
  assert.match(pendingTabHtml, /text-secondary-text/);
  assert.doesNotMatch(pendingTabHtml, /text-highlight/);
  assert.match(pendingTabHtml, /tab-close-btn-delayed/);
  assert.match(pendingTabHtml, /tab-close-btn absolute/);
  assert.doesNotMatch(pendingTabHtml, /after:bg-border/);
});

test('ConversationTabItem selected conversation styling uses prime text and underline for selected backing tabs', () => {
  const styles = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');
  const selectedRule = styles.match(/\.conversation-tab-selected\s*\{([^}]*)\}/m);
  const selectedButtonRule = styles.match(/\.conversation-tab-selected > button\s*\{([^}]*)\}/m);

  assert.ok(selectedRule, 'expected selected conversation tab CSS rule');
  assert.match(selectedRule[1], /color:\s*var\(--color-prime-text\)/);
  assert.doesNotMatch(selectedRule[1], /border-bottom:/);
  assert.doesNotMatch(selectedRule[1], /box-shadow:/);
  assert.doesNotMatch(selectedRule[1], /background-color:\s*var\(--color-hover-bg\)/);
  assert.ok(selectedButtonRule, 'expected selected conversation tab button CSS rule');
  assert.match(selectedButtonRule[1], /color:\s*var\(--color-prime-text\)/);
  assert.match(styles, /\.conversation-tab-selected-title\s*\{[^}]*text-decoration:\s*underline;[^}]*\}/m);
  assert.doesNotMatch(styles, /conversation-tab-selected-label/);
  assert.doesNotMatch(styles, /conversation-tab-shell\s*\{/);
});
