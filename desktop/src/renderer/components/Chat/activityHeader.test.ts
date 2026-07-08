import assert from 'node:assert/strict';
import test from 'node:test';

import type { LiveStep } from '../../store/chatWorkspaceStore';
import {
  buildActivityHeaderViewModel,
  getLiveStepSummary,
  shouldShowActivityHeaderPreview,
} from './activityHeader';

function createStep(overrides: Partial<LiveStep> = {}): LiveStep {
  return {
    id: overrides.id || 'step-1',
    type: overrides.type || 'reasoning',
    label: overrides.label || 'Thinking',
    status: overrides.status || 'running',
    detail: overrides.detail,
    toolCall: overrides.toolCall,
    toolOutput: overrides.toolOutput,
    ts: overrides.ts ?? 1,
  };
}

function buildHeader(overrides: Partial<Parameters<typeof buildActivityHeaderViewModel>[0]> = {}) {
  return buildActivityHeaderViewModel({
    steps: [],
    streamingText: '',
    inProgress: false,
    errorMessage: null,
    errorCode: null,
    autoRetryActive: false,
    autoRetryAttempt: 0,
    autoRetryLimit: 0,
    autoRetryErrorMessage: null,
    reconnectAttempt: 0,
    reconnectLimit: 0,
    reconnectingMessage: null,
    awaitingUser: null,
    planErrorMessage: null,
    tokenUsage: { inputTokens: 0, outputTokens: 0, totalTokens: 0 },
    reviewCount: 0,
    showPlanCard: false,
    showTodoList: false,
    planSnapshot: null,
    ...overrides,
  });
}

test('uses Cooking when work is in progress before any visible step starts', () => {
  const header = buildHeader({ inProgress: true });

  assert.equal(header.status, 'cooking');
  assert.equal(header.label, 'Cooking');
  assert.equal(header.tone, 'running');
  assert.equal(header.preview, '');
  assert.equal(header.metric, 'live');
});

test('uses retained answer text as idle preview without marking the run as live', () => {
  const header = buildHeader({
    retainedAnswerText: 'Previous answer\n\nFinal recommendation',
  });

  assert.equal(header.status, 'done');
  assert.equal(header.label, 'Done');
  assert.equal(header.preview, 'Final recommendation');
  assert.equal(header.metric, 'answer');
});

test('shows continuation as a header hint without replacing the output preview', () => {
  const header = buildHeader({
    retainedAnswerText: 'Previous answer\n\nFinal recommendation',
    threadTailStatus: 'needs_continuation',
  });

  assert.equal(header.status, 'done');
  assert.equal(header.preview, 'Final recommendation');
  assert.equal(header.hint, 'Ready to continue');
  assert.equal(header.metric, 'answer');
});

test('shows queued work as a header hint for idle threads', () => {
  const header = buildHeader({
    queuedMessageCount: 2,
    tokenUsage: { inputTokens: 1200, outputTokens: 34, totalTokens: 1234, contextTokens: 1234, contextWindow: 1_000_000 },
  });

  assert.equal(header.status, 'done');
  assert.equal(header.hint, '2 queued messages');
  assert.equal(header.metric, '1.2k tokens · 0.1%/1M');
});

test('does not show continuation hints while the thread is running', () => {
  const header = buildHeader({
    inProgress: true,
    threadTailStatus: 'needs_continuation',
    queuedMessageCount: 1,
  });

  assert.equal(header.status, 'cooking');
  assert.equal(header.hint, '');
});

test('binds Thinking preview to the latest running reasoning detail', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'reason-1',
        type: 'reasoning',
        status: 'running',
        label: 'Thinking',
        detail: 'Inspecting ActivityPanel header state',
      }),
    ],
  });

  assert.equal(header.status, 'thinking');
  assert.equal(header.label, 'Thinking');
  assert.equal(header.preview, 'Inspecting ActivityPanel header state');
  assert.equal(header.metric, '1 active');
});

test('does not repeat Thinking in preview before reasoning detail arrives', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'reason-1',
        type: 'reasoning',
        status: 'running',
        label: 'Thinking',
        detail: undefined,
      }),
    ],
  });

  assert.equal(header.status, 'thinking');
  assert.equal(header.label, 'Thinking');
  assert.equal(header.preview, '');
  assert.equal(header.metric, '1 active');
});

test('summarizes reasoning steps from their visible detail', () => {
  const summary = getLiveStepSummary(createStep({
    id: 'reason-1',
    type: 'reasoning',
    status: 'done',
    detail: 'Checking the activity timeline\n\nFound the display issue',
  }));

  assert.equal(summary, 'Found the display issue');
});

test('shows context metric while Thinking when usage and context are available', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'reason-1',
        type: 'reasoning',
        status: 'running',
        label: 'Thinking',
        detail: 'Inspecting ActivityPanel header state',
      }),
    ],
    tokenUsage: {
      inputTokens: 1200,
      outputTokens: 34,
      totalTokens: 1234,
      contextTokens: 250_000,
      contextWindow: 1_000_000,
      contextKnown: true,
    },
  });

  assert.equal(header.status, 'thinking');
  assert.equal(header.metric, 'context 25%/1M');
});

test('falls back to token metric while Thinking when context is unavailable', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'reason-1',
        type: 'reasoning',
        status: 'running',
        label: 'Thinking',
        detail: 'Inspecting ActivityPanel header state',
      }),
    ],
    tokenUsage: { inputTokens: 1200, outputTokens: 34, totalTokens: 1234, contextWindow: 1_000_000 },
  });

  assert.equal(header.status, 'thinking');
  assert.equal(header.metric, 'tokens 1.2k');
});

test('shows Tool use preview from the markdown-style tool summary line', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'tool-1',
        type: 'toolcall',
        status: 'running',
        label: 'bash',
        detail: '{"command":"rg -n \\"pgx|postgresql\\" ."}',
        toolOutput: 'Loaded /tmp/demo.md',
      }),
    ],
  });

  assert.equal(header.status, 'tool');
  assert.equal(header.label, 'Tool use');
  assert.equal(header.preview, 'bash: rg -n "pgx|postgresql" .');
});

test('shows token metric while Tool use when usage and context are available', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'tool-1',
        type: 'toolcall',
        status: 'running',
        label: 'bash',
        detail: '{"command":"rg -n \\"pgx|postgresql\\" ."}',
        toolOutput: 'Loaded /tmp/demo.md',
      }),
    ],
    tokenUsage: {
      inputTokens: 250_000,
      outputTokens: 100,
      totalTokens: 250_100,
      contextTokens: 250_000,
      contextWindow: 1_000_000,
      contextKnown: true,
    },
  });

  assert.equal(header.status, 'tool');
  assert.equal(header.metric, 'tokens 250k');
});

test('falls back to context metric while Tool use when token usage is unavailable', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'tool-1',
        type: 'toolcall',
        status: 'running',
        label: 'bash',
        detail: '{"command":"rg -n \\"pgx|postgresql\\" ."}',
        toolOutput: 'Loaded /tmp/demo.md',
      }),
    ],
    tokenUsage: {
      inputTokens: 0,
      outputTokens: 0,
      totalTokens: 0,
      contextTokens: 250_000,
      contextWindow: 1_000_000,
      contextKnown: true,
    },
  });

  assert.equal(header.status, 'tool');
  assert.equal(header.metric, 'context 25%/1M');
});

test('keeps Tool use preview bound to the latest finished tool summary instead of the tool output', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'tool-1',
        type: 'toolcall',
        status: 'done',
        label: 'read_file',
        detail: '{"path":"/tmp/docs/部署service.md"}',
        toolOutput: 'Loaded /tmp/demo.md',
      }),
    ],
  });

  assert.equal(header.status, 'tool');
  assert.equal(header.label, 'Tool use');
  assert.equal(header.preview, 'read: 部署service.md');
});

test('summarizes tool rows from call arguments before tool output', () => {
  const summary = getLiveStepSummary(createStep({
    id: 'tool-1',
    type: 'toolcall',
    status: 'done',
    label: 'bash',
    detail: '{"command":"rg -n \\"activity\\" desktop/src/renderer"}',
    toolOutput: '102 matches',
  }));

  assert.equal(summary, 'bash: rg -n "activity" desktop/src/renderer');
});

test('summarizes tool rows from structured tool call arguments', () => {
  const summary = getLiveStepSummary(createStep({
    id: 'tool-1',
    type: 'toolcall',
    status: 'running',
    label: 'tool',
    toolCall: {
      id: 'call-1',
      name: 'read',
      rawArguments: '{"path":"/tmp/docs/thread.md"}',
      arguments: { path: '/tmp/docs/thread.md' },
    },
  }));

  assert.equal(summary, 'read: thread.md');
});

test('summarizes result-only tool rows from tool output', () => {
  const summary = getLiveStepSummary(createStep({
    id: 'tool-1',
    type: 'toolcall',
    status: 'done',
    label: 'read',
    toolOutput: 'Loaded /tmp/demo.md',
  }));

  assert.equal(summary, 'Loaded /tmp/demo.md');
});

test('falls back to the normalized tool name when raw arguments cannot be parsed', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'tool-1',
        type: 'toolcall',
        status: 'running',
        label: 'read_file',
        detail: '{invalid json',
        toolOutput: 'Loaded /tmp/demo.md',
      }),
    ],
  });

  assert.equal(header.status, 'tool');
  assert.equal(header.label, 'Tool use');
  assert.equal(header.preview, 'read');
});

test('keeps markdown-style fallback when the detail is already a human-readable summary', () => {
  const header = buildHeader({
    inProgress: true,
    steps: [
      createStep({
        id: 'tool-1',
        type: 'toolcall',
        status: 'done',
        label: 'read_file',
        detail: '\nread: 部署service.md\n\n- scanned deployment notes',
        toolOutput: 'Loaded /tmp/demo.md',
      }),
    ],
  });

  assert.equal(header.status, 'tool');
  assert.equal(header.label, 'Tool use');
  assert.equal(header.preview, 'read: 部署service.md');
});

test('shows Replying preview from assistant stream text', () => {
  const header = buildHeader({
    inProgress: true,
    streamingText: 'Drafting the final reply',
  });

  assert.equal(header.status, 'replying');
  assert.equal(header.label, 'Replying');
  assert.equal(header.preview, 'Drafting the final reply');
});

test('prefers the final assistant text over the last finished tool output when done', () => {
  const header = buildHeader({
    inProgress: false,
    streamingText: 'My recommendation would be pick one explicit semantic and enforce it in gateway.',
    steps: [
      createStep({
        id: 'tool-1',
        type: 'toolcall',
        status: 'done',
        label: 'bash',
        detail: '{"command":"nl -ba internal/core/model.go | sed -n \\"47,65p\\""}',
        toolOutput: '65 return nil, err',
      }),
    ],
  });

  assert.equal(header.status, 'done');
  assert.equal(header.label, 'Done');
  assert.equal(
    header.preview,
    'My recommendation would be pick one explicit semantic and enforce it in gateway.'
  );
});

test('shows token metric only after official usage is available', () => {
  const header = buildHeader({
    tokenUsage: { inputTokens: 1200, outputTokens: 34, totalTokens: 1234 },
  });

  assert.equal(header.metric, '1.2k tokens');
});

test('shows context progress when context window is available', () => {
  const header = buildHeader({
    tokenUsage: {
      inputTokens: 250_000,
      outputTokens: 100,
      totalTokens: 250_100,
      contextTokens: 250_000,
      contextWindow: 1_000_000,
      contextKnown: true,
    },
  });

  assert.equal(header.metric, '250k tokens · 25%/1M');
});

test('does not derive context progress from current loop tokens', () => {
  const header = buildHeader({
    tokenUsage: { inputTokens: 250_000, outputTokens: 100, totalTokens: 250_100, contextWindow: 1_000_000 },
  });

  assert.equal(header.metric, '250k tokens');
});

test('shows unknown context progress after compaction until usage is available', () => {
  const header = buildHeader({
    tokenUsage: { inputTokens: 0, outputTokens: 0, totalTokens: 0, contextWindow: 1_000_000, contextKnown: false },
  });

  assert.equal(header.metric, '?/1M');
});

test('surfaces reconnecting state without downgrading it to an error', () => {
  const header = buildHeader({
    inProgress: true,
    reconnectAttempt: 2,
    reconnectLimit: 5,
    reconnectingMessage: 'Reconnecting... 2/5',
  });

  assert.equal(header.status, 'reconnecting');
  assert.equal(header.label, 'Reconnecting');
  assert.equal(header.tone, 'running');
  assert.equal(header.preview, 'Reconnecting... 2/5');
  assert.equal(header.metric, '2/5');
});

test('surfaces runtime auto retry state separately from transport reconnects', () => {
  const header = buildHeader({
    inProgress: true,
    autoRetryActive: true,
    autoRetryAttempt: 1,
    autoRetryLimit: 5,
    autoRetryErrorMessage: 'upstream server_error',
  });

  assert.equal(header.status, 'retrying');
  assert.equal(header.label, 'Retrying');
  assert.equal(header.tone, 'running');
  assert.equal(header.preview, 'upstream server_error');
  assert.equal(header.metric, '1/5');
});

test('prioritizes waiting state over running progress when user request is pending', () => {
  const header = buildHeader({
    inProgress: true,
    awaitingUser: {
      requestID: 'req-1',
      questions: [{
        header: 'Q1',
        question: 'Please confirm the migration target.',
        options: [{ label: 'Accept' }],
        custom: false,
      }],
      currentIndex: 0,
      answers: [[]],
      customModeByIndex: [false],
      requestedAt: 1,
    },
  });

  assert.equal(header.status, 'waiting');
  assert.equal(header.label, 'Waiting');
  assert.equal(header.tone, 'running');
  assert.equal(header.preview, 'Please confirm the migration target.');
  assert.equal(header.metric, '1/1');
});

test('shows Aborted for user-initiated cancellation', () => {
  const header = buildHeader({
    inProgress: false,
    errorMessage: 'Aborted',
    steps: [
      createStep({
        id: 'tool-1',
        type: 'toolcall',
        status: 'done',
        label: 'read',
      }),
    ],
  });

  assert.equal(header.status, 'aborted');
  assert.equal(header.label, 'Aborted');
  assert.equal(header.tone, 'aborted');
  assert.equal(header.preview, '');
});

test('prioritizes Error over other running header states', () => {
  const header = buildHeader({
    inProgress: true,
    errorMessage: 'stream failed',
    streamingText: 'partial reply',
    steps: [
      createStep({
        id: 'reason-1',
        type: 'reasoning',
        status: 'running',
        detail: 'Still thinking',
      }),
    ],
  });

  assert.equal(header.status, 'error');
  assert.equal(header.label, 'Error');
  assert.equal(header.tone, 'error');
  assert.equal(header.preview, '');
});

test('shows quota exhausted summary in the header preview when billing quota is exhausted', () => {
  const header = buildHeader({
    inProgress: false,
    errorMessage: 'quota_exhausted',
    errorCode: 'quota_exhausted',
  });

  assert.equal(header.status, 'error');
  assert.equal(header.tone, 'error');
  assert.equal(header.preview, 'AI quota exhausted. Open Billing to continue.');
});

test('keeps the plan card header branch intact when only a plan is available', () => {
  const header = buildHeader({
    showPlanCard: true,
    planSnapshot: { totalCount: 3, completedCount: 1 },
  });

  assert.equal(header.status, 'plan');
  assert.equal(header.label, 'Plan');
  assert.equal(header.metric, '3 items');
  assert.equal(header.preview, 'Ready to execute');
});

test('keeps the todo completion header branch intact when all plan items are done', () => {
  const header = buildHeader({
    showTodoList: true,
    planSnapshot: { totalCount: 3, completedCount: 3 },
  });

  assert.equal(header.status, 'done');
  assert.equal(header.label, 'Done');
  assert.equal(header.metric, '3/3');
  assert.equal(header.preview, 'Plan complete');
});

test('shows plan structure errors instead of fake todo progress', () => {
  const header = buildHeader({
    showTodoList: true,
    planErrorMessage: 'Plan 缺少专用任务区：只支持 `## Tasks` 或 `## 任务`。',
  });

  assert.equal(header.status, 'error');
  assert.equal(header.label, 'Error');
  assert.equal(header.tone, 'error');
  assert.equal(header.metric, 'invalid');
  assert.equal(header.preview, 'Plan 缺少专用任务区：只支持 `## Tasks` 或 `## 任务`。');
});

test('shows compacted status when a notice step is the latest settled activity', () => {
  const header = buildHeader({
    steps: [
      createStep({
        id: 'notice-1',
        type: 'notice',
        status: 'done',
        label: 'Compacted',
        detail: 'Compacted older context into a checkpoint summary.',
      }),
    ],
  });

  assert.equal(header.status, 'compacted');
  assert.equal(header.label, 'Compacted');
  assert.equal(header.preview, 'Compacted older context into a checkpoint summary.');
});


test('keeps compacted metric on context only instead of forcing done dual metric', () => {
  const header = buildHeader({
    steps: [
      createStep({
        id: 'notice-1',
        type: 'notice',
        status: 'done',
        label: 'Compacted',
        detail: 'Compacted older context into a checkpoint summary.',
      }),
    ],
    tokenUsage: {
      inputTokens: 250_000,
      outputTokens: 100,
      totalTokens: 250_100,
      contextTokens: 250_000,
      contextWindow: 1_000_000,
      contextKnown: true,
    },
  });

  assert.equal(header.status, 'compacted');
  assert.equal(header.metric, '25%/1M');
});

test('hides expanded preview unless there is non-error header detail', () => {
  assert.equal(shouldShowActivityHeaderPreview('Thinking live text', false, 'running'), true);
  assert.equal(shouldShowActivityHeaderPreview('Thinking live text', true, 'running'), false);
  assert.equal(shouldShowActivityHeaderPreview('', true, 'error'), false);
});
