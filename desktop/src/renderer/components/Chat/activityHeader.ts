import type {
  LiveStep,
  AwaitingUserState,
  ThreadTailStatus,
  TokenUsage,
} from '../../store/chatWorkspaceStore';
import { getToolActivityDetailPreview } from '../../utils/toolActivitySummary';
import { resolveActivityErrorInfo } from './activityErrorState';
import {
  formatToolCallSummary,
  tryFormatToolCallSummary,
} from '../../utils/toolCallSummary';

export type ActivityHeaderStatus = 'error' | 'aborted' | 'retrying' | 'reconnecting' | 'waiting' | 'tool' | 'thinking' | 'replying' | 'cooking' | 'compacted' | 'done' | 'plan';
export type ActivityHeaderTone = 'running' | 'done' | 'error' | 'aborted';

export type ActivityHeaderPlanSnapshot = {
  totalCount: number;
  completedCount: number;
};

export type ActivityHeaderViewModel = {
  status: ActivityHeaderStatus;
  label: string;
  tone: ActivityHeaderTone;
  preview: string;
  hint: string;
  metric: string;
};

type BuildActivityHeaderViewModelParams = {
  steps: LiveStep[];
  streamingText: string;
  retainedAnswerText?: string;
  inProgress: boolean;
  errorMessage: string | null;
  errorCode: string | null;
  autoRetryActive: boolean;
  autoRetryAttempt: number;
  autoRetryLimit: number;
  autoRetryErrorMessage: string | null;
  reconnectAttempt: number;
  reconnectLimit: number;
  reconnectingMessage: string | null;
  awaitingUser: AwaitingUserState | null;
  planErrorMessage: string | null;
  tokenUsage: TokenUsage;
  threadTailStatus?: ThreadTailStatus | null;
  queuedMessageCount?: number;
  reviewCount: number;
  showPlanCard: boolean;
  showTodoList: boolean;
  planSnapshot: ActivityHeaderPlanSnapshot | null;
};

const HEADER_LABELS: Record<ActivityHeaderStatus, string> = {
  error: 'Error',
  aborted: 'Aborted',
  retrying: 'Retrying',
  reconnecting: 'Reconnecting',
  waiting: 'Waiting',
  tool: 'Tool use',
  thinking: 'Thinking',
  replying: 'Replying',
  cooking: 'Cooking',
  compacted: 'Compacted',
  done: 'Done',
  plan: 'Plan',
};

function formatTokenCount(n: number): string {
  if (n >= 1_000_000) {
    const millions = n / 1_000_000;
    return Number.isInteger(millions) ? `${millions}M` : `${millions.toFixed(1)}M`;
  }
  if (n >= 10_000) return `${Math.round(n / 1000)}k`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

export function getLiveStepContent(step: LiveStep | null | undefined): string | undefined {
  if (!step) {
    return undefined;
  }
  return step.type === 'toolcall' ? step.toolOutput : step.detail;
}

function toSingleLineTail(text: string, maxChars = 140): string {
  const normalized = text.replace(/\r\n/g, '\n');
  const lines = normalized
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '{"details":null}')
    .filter(Boolean);
  const source = (lines.length > 0 ? lines[lines.length - 1] : normalized).replace(/\s+/g, ' ').trim();
  if (!source) {
    return '';
  }
  if (source.length <= maxChars) {
    return source;
  }
  return `...${source.slice(source.length - maxChars)}`;
}

function findLatestStep(
  steps: LiveStep[],
  predicate: (step: LiveStep) => boolean
): LiveStep | null {
  for (let index = steps.length - 1; index >= 0; index -= 1) {
    const step = steps[index];
    if (predicate(step)) {
      return step;
    }
  }
  return null;
}

function getAwaitingQuestionPreview(awaitingUser: AwaitingUserState | null): string {
  if (!awaitingUser) {
    return '';
  }
  const current = awaitingUser.questions[awaitingUser.currentIndex] || awaitingUser.questions[0];
  return current ? current.question : '';
}

function getToolInputPreview(step: LiveStep | null): string {
  if (!step) {
    return '';
  }
  const detail = step.toolCall?.rawArguments || step.detail || '';
  const trimmedDetail = detail.trim();
  const parsedArguments = step.toolCall?.arguments || parseToolArgumentsObject(detail);
  const summaryFromArguments = tryFormatToolCallSummary(step.toolCall?.name || step.label, parsedArguments);
  if (summaryFromArguments) {
    return summaryFromArguments;
  }
  if (!trimmedDetail) {
    return '';
  }
  const isJsonLikeDetail = trimmedDetail.startsWith('{') || trimmedDetail.startsWith('[');
  if (!parsedArguments && !isJsonLikeDetail) {
    const markdownStyleSummary = getToolActivityDetailPreview(detail);
    if (markdownStyleSummary) {
      return markdownStyleSummary;
    }
  }
  return formatToolCallSummary(step.toolCall?.name || step.label, parsedArguments);
}

function getToolPreview(step: LiveStep | null): string {
  if (!step) {
    return '';
  }
  return getToolInputPreview(step) || formatToolCallSummary(step.toolCall?.name || step.label, null);
}

export function getLiveStepSummary(step: LiveStep | null | undefined): string {
  if (!step) {
    return '';
  }
  if (step.type === 'toolcall') {
    const toolInputPreview = getToolInputPreview(step);
    if (toolInputPreview) {
      return toolInputPreview;
    }
    const toolOutputPreview = toSingleLineTail(step.toolOutput || '');
    if (toolOutputPreview) {
      return toolOutputPreview;
    }
    return formatToolCallSummary(step.toolCall?.name || step.label, null);
  }
  return toSingleLineTail(getLiveStepContent(step) || '');
}

function getStepDetailPreview(step: LiveStep | null): string {
  if (!step) {
    return '';
  }
  return getLiveStepSummary(step);
}

function parseToolArgumentsObject(detail: string): Record<string, unknown> | null {
  const trimmed = detail.trim();
  if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) {
    return null;
  }
  try {
    const parsed = JSON.parse(trimmed);
    return parsed && typeof parsed === 'object' ? parsed as Record<string, unknown> : null;
  } catch {
    return null;
  }
}

function isAbortActivityText(text: string | null | undefined): boolean {
  return /^(aborted|interrupted)$/i.test((text || '').trim()) || /context canceled/i.test((text || '').trim());
}

function resolveBaseStatus({
  steps,
  streamingText,
  inProgress,
  errorMessage,
  autoRetryActive,
  autoRetryErrorMessage,
  reconnectingMessage,
  awaitingUser,
}: Pick<BuildActivityHeaderViewModelParams, 'steps' | 'streamingText' | 'inProgress' | 'errorMessage' | 'autoRetryActive' | 'autoRetryErrorMessage' | 'reconnectingMessage' | 'awaitingUser'>): {
  status: ActivityHeaderStatus;
  tone: ActivityHeaderTone;
} {
  if (isAbortActivityText(errorMessage)) {
    return { status: 'aborted', tone: 'aborted' };
  }

  if ((errorMessage || '').trim()) {
    return { status: 'error', tone: 'error' };
  }

  if (autoRetryActive) {
    return { status: 'retrying', tone: 'running' };
  }

  if ((reconnectingMessage || '').trim()) {
    return { status: 'reconnecting', tone: 'running' };
  }

  if (awaitingUser) {
    return { status: 'waiting', tone: 'running' };
  }

  const latestError = findLatestStep(
    steps,
    (step) => step.status === 'error' || step.type === 'error'
  );
  if (latestError) {
    const latestErrorText = getLiveStepContent(latestError) || latestError.label || '';
    if (isAbortActivityText(latestErrorText)) {
      return { status: 'aborted', tone: 'aborted' };
    }
    return { status: 'error', tone: 'error' };
  }

  const latestNotice = findLatestStep(
    steps,
    (step) => step.type === 'notice'
  );
  if (!inProgress) {
    if (latestNotice) {
      return { status: 'compacted', tone: 'done' };
    }
    return { status: 'done', tone: 'done' };
  }

  const latestRunning = findLatestStep(steps, (step) => step.status === 'running');
  const latestFinishedTool = findLatestStep(
    steps,
    (step) => step.type === 'toolcall' && step.status === 'done'
  );
  if (latestRunning?.type === 'toolcall') {
    return { status: 'tool', tone: 'running' };
  }
  if (latestRunning?.type === 'reasoning') {
    return { status: 'thinking', tone: 'running' };
  }
  if (streamingText.trim()) {
    return { status: 'replying', tone: 'running' };
  }
  if (latestFinishedTool) {
    return { status: 'tool', tone: 'running' };
  }
  return { status: 'cooking', tone: 'running' };
}

function buildPreview(
  status: ActivityHeaderStatus,
  params: Pick<BuildActivityHeaderViewModelParams, 'steps' | 'streamingText' | 'retainedAnswerText' | 'inProgress' | 'errorMessage' | 'errorCode' | 'autoRetryErrorMessage' | 'reconnectingMessage' | 'awaitingUser'>
): string {
  const autoRetryErrorMessage = (params.autoRetryErrorMessage || '').trim();
  const reconnectingMessage = (params.reconnectingMessage || '').trim();
  const latestRunning = findLatestStep(params.steps, (step) => step.status === 'running');
  const latestTool = findLatestStep(params.steps, (step) => step.type === 'toolcall');
  const latestNotice = findLatestStep(params.steps, (step) => step.type === 'notice');
  const latestFinished = findLatestStep(params.steps, () => true);
  const quotaErrorInfo = resolveActivityErrorInfo(params.errorMessage, params.errorCode);

  switch (status) {
    case 'error':
      return quotaErrorInfo?.summary || '';
    case 'aborted':
      return '';
    case 'retrying':
      return toSingleLineTail(autoRetryErrorMessage);
    case 'reconnecting':
      return toSingleLineTail(reconnectingMessage);
    case 'waiting':
      return toSingleLineTail(getAwaitingQuestionPreview(params.awaitingUser));
    case 'thinking':
      return getStepDetailPreview(latestRunning);
    case 'tool':
      return getToolPreview(latestTool);
    case 'replying':
      return toSingleLineTail(params.streamingText);
    case 'compacted':
      return toSingleLineTail(getLiveStepContent(latestNotice) || latestNotice?.label || '');
    case 'done':
      return toSingleLineTail(
        params.streamingText
        || params.retainedAnswerText
        || getLiveStepContent(latestFinished)
        || latestFinished?.label
        || ''
      );
    default:
      return '';
  }
}

function formatTokenMetric(totalTokens: number): string {
  return `${formatTokenCount(totalTokens)} tokens`;
}

function formatRunningTokenMetric(totalTokens: number): string {
  return `tokens ${formatTokenCount(totalTokens)}`;
}

function formatContextMetric(tokenUsage: TokenUsage): string | null {
  const contextWindow = tokenUsage.contextWindow || 0;
  if (contextWindow <= 0) {
    return null;
  }
  if (tokenUsage.contextKnown === false) {
    return `?/${formatTokenCount(contextWindow)}`;
  }
  const contextTokens = typeof tokenUsage.contextTokens === 'number' && Number.isFinite(tokenUsage.contextTokens)
    ? tokenUsage.contextTokens
    : 0;
  if (contextTokens <= 0) {
    return null;
  }
  const percent = typeof tokenUsage.contextPercent === 'number' && Number.isFinite(tokenUsage.contextPercent)
    ? tokenUsage.contextPercent
    : (contextTokens / contextWindow) * 100;
  const percentText = percent >= 10 ? `${Math.round(percent)}%` : `${percent.toFixed(1)}%`;
  return `${percentText}/${formatTokenCount(contextWindow)}`;
}

function formatRunningContextMetric(tokenUsage: TokenUsage): string {
  const contextMetric = formatContextMetric(tokenUsage);
  return contextMetric ? `context ${contextMetric}` : '';
}

function buildMetric(
  status: ActivityHeaderStatus,
  {
    tokenUsage,
    steps,
    inProgress,
    autoRetryActive,
    autoRetryAttempt,
    autoRetryLimit,
    reconnectAttempt,
    reconnectLimit,
    reconnectingMessage,
    awaitingUser,
  }: Pick<BuildActivityHeaderViewModelParams, 'tokenUsage' | 'steps' | 'inProgress' | 'autoRetryActive' | 'autoRetryAttempt' | 'autoRetryLimit' | 'reconnectAttempt' | 'reconnectLimit' | 'reconnectingMessage' | 'awaitingUser'>,
): string {
  if (autoRetryActive) {
    return autoRetryLimit > 0 ? `${autoRetryAttempt}/${autoRetryLimit}` : 'retrying';
  }

  if ((reconnectingMessage || '').trim()) {
    return reconnectLimit > 0 ? `${reconnectAttempt}/${reconnectLimit}` : 'retrying';
  }

  if (awaitingUser) {
    const total = awaitingUser.questions.length;
    const currentIndex = Math.min(Math.max(awaitingUser.currentIndex + 1, 1), total || 1);
    return `${currentIndex}/${total || 1}`;
  }

  const totalTokens = tokenUsage.totalTokens || tokenUsage.inputTokens + tokenUsage.outputTokens;
  const tokenMetric = totalTokens > 0 ? formatTokenMetric(totalTokens) : '';
  const contextMetric = formatContextMetric(tokenUsage);

  if (status === 'thinking') {
    const runningContextMetric = formatRunningContextMetric(tokenUsage);
    const runningTokenMetric = totalTokens > 0 ? formatRunningTokenMetric(totalTokens) : '';
    if (runningContextMetric || runningTokenMetric) {
      return runningContextMetric || runningTokenMetric;
    }
  }

  if (status === 'tool') {
    const runningTokenMetric = totalTokens > 0 ? formatRunningTokenMetric(totalTokens) : '';
    const runningContextMetric = formatRunningContextMetric(tokenUsage);
    if (runningTokenMetric || runningContextMetric) {
      return runningTokenMetric || runningContextMetric;
    }
  }

  if (status === 'done') {
    if (tokenMetric && contextMetric) {
      return `${tokenMetric} · ${contextMetric}`;
    }
    if (tokenMetric) {
      return tokenMetric;
    }
    if (contextMetric) {
      return contextMetric;
    }
  }

  if (contextMetric) {
    return contextMetric;
  }
  if (tokenMetric) {
    return tokenMetric;
  }

  if (inProgress) {
    const runningCount = steps.filter((step) => step.status === 'running').length;
    return runningCount > 0 ? `${runningCount} active` : 'live';
  }

  return `${steps.length} step${steps.length === 1 ? '' : 's'}`;
}

function buildThreadStatusHint({
  inProgress,
  awaitingUser,
  tone,
  queuedMessageCount,
  threadTailStatus,
}: Pick<BuildActivityHeaderViewModelParams, 'inProgress' | 'awaitingUser' | 'queuedMessageCount' | 'threadTailStatus'> & {
  tone: ActivityHeaderTone;
}): string {
  if (inProgress || awaitingUser || tone === 'error' || tone === 'aborted') {
    return '';
  }
  const normalizedQueuedCount = Math.max(0, Math.floor(queuedMessageCount || 0));
  if (normalizedQueuedCount > 0) {
    return normalizedQueuedCount === 1 ? '1 queued message' : `${normalizedQueuedCount} queued messages`;
  }
  if (threadTailStatus === 'needs_continuation') {
    return 'Ready to continue';
  }
  return '';
}

function formatReviewMetric(reviewCount: number): string {
  return `${reviewCount} review${reviewCount === 1 ? '' : 's'}`;
}

export function buildActivityHeaderViewModel(
  params: BuildActivityHeaderViewModelParams
): ActivityHeaderViewModel {
  const base = resolveBaseStatus(params);
  const hasContent = Boolean(
    params.streamingText
    || params.retainedAnswerText
    || params.errorMessage
    || params.steps.length > 0
  );
  const normalizedPlanError = (params.planErrorMessage || '').trim();

  let status = base.status;
  let tone = base.tone;
  let preview = buildPreview(status, params);
  let metric = buildMetric(status, params);
  let hint = buildThreadStatusHint({
    inProgress: params.inProgress,
    awaitingUser: params.awaitingUser,
    tone,
    queuedMessageCount: params.queuedMessageCount,
    threadTailStatus: params.threadTailStatus,
  });

  if (!params.inProgress && params.retainedAnswerText && !params.streamingText && params.steps.length === 0) {
    metric = 'answer';
  }

  if (!hasContent && !params.inProgress && params.reviewCount > 0) {
    metric = formatReviewMetric(params.reviewCount);
    if (!preview) {
      preview = 'Review history';
    }
  }

  if (
    !hasContent
    && !params.inProgress
    && params.reviewCount === 0
    && (params.showPlanCard || params.showTodoList)
    && normalizedPlanError
  ) {
    status = 'error';
    tone = 'error';
    metric = 'invalid';
    hint = '';
    preview = normalizedPlanError;
  } else if (!hasContent && !params.inProgress && params.reviewCount === 0 && params.showPlanCard) {
    status = 'plan';
    tone = 'done';
    metric = params.planSnapshot
      ? `${params.planSnapshot.totalCount} item${params.planSnapshot.totalCount === 1 ? '' : 's'}`
      : 'ready';
    preview = preview || 'Ready to execute';
  } else if (!hasContent && !params.inProgress && params.reviewCount === 0 && params.showTodoList) {
    const allComplete = Boolean(
      params.planSnapshot
      && params.planSnapshot.totalCount > 0
      && params.planSnapshot.completedCount === params.planSnapshot.totalCount
    );
    status = allComplete ? 'done' : 'plan';
    tone = 'done';
    metric = params.planSnapshot
      ? `${params.planSnapshot.completedCount}/${params.planSnapshot.totalCount}`
      : 'todo';
    preview = preview || (allComplete ? 'Plan complete' : 'Todo list');
  }

  return {
    status,
    label: HEADER_LABELS[status],
    tone,
    preview,
    hint,
    metric,
  };
}

export function shouldShowActivityHeaderPreview(
  preview: string,
  expanded: boolean,
  tone: ActivityHeaderTone
): boolean {
  return Boolean(preview) && (!expanded || tone === 'error');
}
