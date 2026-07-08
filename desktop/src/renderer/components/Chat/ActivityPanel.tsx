import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import unicodeSpinners from 'unicode-animations';
import {
  useChatWorkspaceStore,
  type LiveStep,
  type LiveTextSegment,
  type AwaitingUserState,
  type ThreadSummary,
  type TokenUsage,
} from '../../store/chatWorkspaceStore';
import { useAppStore } from '../../store/appStore';
import { useUiStore } from '../../store/uiStore';
import { useModelsStore } from '../../store/modelsStore';
import { useToastStore } from '../../store/toastStore';
import { useAuthStore } from '../../store/authStore';
import type { ThreadReviewFile, ThreadReviewState } from '../../services/reviewService';
import type { ChatMessageRecord, ThreadEntry, ThreadSnapshot } from '../../services/threadService';
import {
  clearContextExecutePlan,
  executePlanInCurrentThread,
  loadThreadSnapshotWindow,
  type PlanBuildRequest,
} from '../../services/chatService';
import type { ThinkingLevel } from '../../services/chatInput';
import type { ModelEntry } from '../../types/electron';
import { parsePlanChecklist, type PlanChecklistSnapshot } from '../../utils/planChecklist';
import {
  getThinkingPickerLevels,
  normalizeThinkingLevelForModel,
  persistGlobalThinkingLevel,
  UI_CHAT_THINKING_ON_LEVEL,
} from '../../utils/chatThinking';
import { PRIMARY_CHAT_CAPABLE_AGENT_OPCODE } from '../../utils/agentSwitch';
import { buildModelSelectOption } from '../../utils/modelDisplay';
import {
  buildActivityHeaderViewModel,
  getLiveStepContent,
  getLiveStepSummary,
  shouldShowActivityHeaderPreview,
} from './activityHeader';
import { buildActivityThreadMetadataViewModel } from './activityThreadMetadata';
import {
  resolveActivityErrorInfo,
  shouldKeepActivityPanelExpandedAfterRun,
} from './activityErrorState';
import { BillingRestrictionCard } from '../BillingRestrictionCard';
import { formatReviewActionError } from '../../utils/reviewMessages';
import { resolveDefaultChatModelSelection } from '../../utils/chatModelSelection';
import { buildThreadLinkTarget } from '../../utils/threadLink';
import { navigateFrontmatterLink } from '../../utils/frontmatterLinkNavigate';
import { resolveLooseResourceUrl } from '../../services/resourceService';
import { SelectMenu } from '../SelectMenu';
import { OP_SG_CAPSULE, OP_SG_CAPSULE_ON_ACTIVITY_HEADER } from '../staticGlassCapsule';
import { ChatLineIcon, ChevronRightIcon, CloseTinyIcon } from '../Icons';
import { ActivityMarkdownView } from './ActivityMarkdown';
import { buildInitials, initialsBackgroundColor } from '../avatarInitials';
import { resolveUserAvatarSrc } from '../TitlebarUserAvatar';

const DEFAULT_ACTIVITY_PANEL_BODY_MAX_HEIGHT = 400;
const ACTIVITY_PANEL_FOLLOW_BOTTOM_THRESHOLD_PX = 24;
const ACTIVITY_PANEL_LOAD_OLDER_THRESHOLD_PX = 48;
const ACTIVITY_PANEL_MESSAGE_COLLAPSE_LINES = 99;
const ACTIVITY_PANEL_ACTION_KEYCAPS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
const PLAN_FILE_POLL_INTERVAL_MS = 1200;
const COOKING_SPINNER_NAMES = ['braillewave', 'dna', 'rain', 'sparkle', 'waverows', 'helix'] as const;

function activityPanelActionKeycap(index: number): string {
  if (index >= 0 && index < ACTIVITY_PANEL_ACTION_KEYCAPS.length) {
    return ACTIVITY_PANEL_ACTION_KEYCAPS[index];
  }
  return String(index + 1);
}

function CookingAnimation() {
  const spinner = useMemo(() => {
    const spinnerName = COOKING_SPINNER_NAMES[Math.floor(Math.random() * COOKING_SPINNER_NAMES.length)];
    return unicodeSpinners[spinnerName];
  }, []);
  const [frameIndex, setFrameIndex] = useState(() => Math.floor(Math.random() * spinner.frames.length));

  useEffect(() => {
    const timer = window.setInterval(() => {
      setFrameIndex((current) => (current + 1) % spinner.frames.length);
    }, spinner.interval);
    return () => {
      window.clearInterval(timer);
    };
  }, [spinner]);

  return (
    <span className="op-activity-panel-cooking-animation" aria-hidden="true">
      {spinner.frames[frameIndex]}
    </span>
  );
}

type BuildAgentOption = {
  id: string;
  name: string;
  cwd: string;
};

type BuildConfigState = {
  agentID: string;
  modelKey: string | null;
  thinkingLevel: ThinkingLevel;
};

function formatThinkingLabel(level: ThinkingLevel): string {
  if (level === UI_CHAT_THINKING_ON_LEVEL) {
    return 'On';
  }
  return level === 'off' ? 'Off' : level;
}

function buildAgentOptions(
  agentNodes: Array<{ id: string; cwd?: string; meta?: Record<string, unknown>; opCodes?: string[] }>,
): BuildAgentOption[] {
  return agentNodes
    .filter((node) => (Array.isArray(node.opCodes) ? node.opCodes : []).some((code) => (
      (code || '').trim() === PRIMARY_CHAT_CAPABLE_AGENT_OPCODE
    )))
    .map((node) => {
      const cwd = (node.cwd || '').trim();
      const id = (node.id || '').trim();
      if (!cwd || !id) {
        return null;
      }
      const meta = node.meta || {};
      const name = (typeof meta.name === 'string' ? meta.name : '').trim() || id;
      return { id, name, cwd };
    })
    .filter((option): option is BuildAgentOption => Boolean(option))
    .sort((a, b) => a.name.localeCompare(b.name));
}

function formatAgentOptionLabel(option: BuildAgentOption): string {
  const dirName = option.cwd.split('/').filter(Boolean).pop() || option.cwd;
  return `${dirName}/${option.name}`;
}

function getPlanTitleFallback(path: string): string {
  const fileName = (path || '').trim().split('/').pop() || '';
  return fileName.replace(/\.md$/i, '').trim() || 'Plan';
}

function usePlanChecklistSnapshot(
  chatPath: string | null,
  threadMeta: ThreadSummary | null,
  planRevision: number,
) {
  const readTextFile = useAppStore((s) => s.readTextFile);
  const statPath = useAppStore((s) => s.statPath);
  const [snapshot, setSnapshot] = useState<PlanChecklistSnapshot | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const planPath = useMemo(
    () => ((threadMeta?.executionPlanPath || threadMeta?.planPath || '').trim() || ''),
    [threadMeta?.executionPlanPath, threadMeta?.planPath]
  );

  useEffect(() => {
    let cancelled = false;
    let lastModTime = -1;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const load = async (force = false) => {
      if (!planPath || !chatPath) {
        if (!cancelled) {
          setSnapshot(null);
          setError(null);
        }
        return;
      }
      if (!cancelled) {
        setLoading(true);
      }
      try {
        const stat = await statPath(planPath);
        if (stat.error || stat.isDir) {
          throw new Error(stat.error || 'Plan file is unavailable.');
        }
        if (!force && stat.modTime === lastModTime) {
          return;
        }
        lastModTime = stat.modTime;
        const content = await readTextFile(planPath);
        if (content == null) {
          throw new Error('Failed to read plan file.');
        }
        const parsed = parsePlanChecklist(content, {
          fallbackTitle: getPlanTitleFallback(planPath),
        });
        if (!cancelled) {
          if (parsed.ok) {
            setSnapshot(parsed);
            setError(null);
          } else {
            setSnapshot(null);
            setError(parsed.error);
          }
        }
      } catch (nextError) {
        if (!cancelled) {
          setSnapshot(null);
          setError((nextError as Error)?.message || 'Failed to load plan.');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    void load(true);

    const poll = async () => {
      await load(false);
      if (!cancelled) {
        timer = setTimeout(() => {
          void poll();
        }, PLAN_FILE_POLL_INTERVAL_MS);
      }
    };
    timer = setTimeout(() => {
      void poll();
    }, PLAN_FILE_POLL_INTERVAL_MS);

    return () => {
      cancelled = true;
      if (timer) {
        clearTimeout(timer);
      }
    };
  }, [chatPath, planPath, planRevision, readTextFile, statPath]);

  return {
    planPath,
    snapshot,
    loading,
    error,
  };
}

type EntryMessageQuestionOption = {
  id: string;
  label: string;
};

type EntryMessageQuestion = {
  id: string;
  question: string;
  options?: EntryMessageQuestionOption[];
};

type EntryMessageQuestionAnswer = {
  questionID: string;
  optionID?: string;
  label?: string;
  other?: boolean;
  text?: string;
};

type EntryMessage = {
  id: string;
  role: string;
  bubbleRole: 'user' | 'assistant' | 'system' | 'request' | 'status' | 'message';
  body: string;
  title?: string;
  status?: string;
  channelID?: string;
  recordID?: string;
  actions?: Array<{ id: string; label: string; tone?: 'primary' | 'danger' }>;
  questions?: EntryMessageQuestion[];
  replyToMessageID?: string;
  actionID?: string;
  answers?: EntryMessageQuestionAnswer[];
};

type ActivityUserProfileLike = {
  name?: string;
  username?: string;
  email?: string;
  avatar?: string;
};

type ActivityMessageParticipant = {
  kind: EntryMessage['bubbleRole'];
  name: string;
  avatarSrc: string | null;
  fallbackText: string;
};

type ActivityMessageParticipants = {
  user: ActivityMessageParticipant;
  assistant: ActivityMessageParticipant;
};

type ActivityTimelineItem =
  | { kind: 'message'; ts: number; order: number | null; message: EntryMessage }
  | { kind: 'stream'; ts: number; order: number | null; segment: LiveTextSegment }
  | { kind: 'step'; ts: number; order: number | null; step: LiveStep };

type ActivityTimelineRenderItem = {
  item: ActivityTimelineItem;
  showParticipantHeader: boolean;
};

function getActivityTimelineKindRank(kind: ActivityTimelineItem['kind']): number {
  switch (kind) {
    case 'message':
      return 0;
    case 'step':
      return 1;
    case 'stream':
      return 2;
    default:
      return 3;
  }
}

function isAgentActivityTimelineItem(item: ActivityTimelineItem): boolean {
  if (item.kind === 'message') {
    // Agent-published request/status/message records are part of the agent
    // segment so a turn shows the agent name once and does not stamp each
    // request card with its own title as a fake name.
    return item.message.bubbleRole === 'assistant' || item.message.role === 'agent';
  }
  if (item.kind === 'stream') {
    return true;
  }
  return item.step.type === 'toolcall' || item.step.type === 'reasoning';
}

function buildActivityTimelineRenderItems(items: ActivityTimelineItem[]): ActivityTimelineRenderItem[] {
  let agentSegmentOpen = false;
  return items.map((item) => {
    const isAgentItem = isAgentActivityTimelineItem(item);
    if (!isAgentItem) {
      agentSegmentOpen = false;
      return { item, showParticipantHeader: true };
    }
    const showParticipantHeader = !agentSegmentOpen;
    agentSegmentOpen = true;
    return { item, showParticipantHeader };
  });
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function entryTimestampMs(entry: ThreadEntry): number {
  const parsed = Date.parse(asString(entry.timestamp));
  return Number.isFinite(parsed) ? parsed : 0;
}

function contentBlocks(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value)
    ? value.map(asRecord).filter((block): block is Record<string, unknown> => Boolean(block))
    : [];
}

function compactText(parts: string[]): string {
  return parts.map((part) => part.trim()).filter(Boolean).join('\n\n').trim();
}

function stringifyToolArguments(value: unknown): string {
  if (typeof value === 'string') {
    return value.trim();
  }
  if (!value || typeof value !== 'object') {
    return '';
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return '';
  }
}

function textFromMessageContent(message: Record<string, unknown>): string {
  const direct = asString(message.content);
  if (direct) {
    return direct;
  }
  const parts = contentBlocks(message.content);
  return compactText(parts.map((block) => {
    const type = asString(block.type);
    if (type === 'text' || type === 'input_text' || type === 'output_text' || type === 'compaction') {
      return asString(block.text);
    }
    return '';
  }));
}

function textFromQueueMessage(message: unknown): string {
  const record = asRecord(message);
  if (!record) {
    return '';
  }
  const direct = asString(record.content);
  if (direct) {
    return direct;
  }
  const parts = Array.isArray(record.content_parts)
    ? record.content_parts.map(asRecord).filter((part): part is Record<string, unknown> => Boolean(part))
    : [];
  return compactText(parts.map((part) => asString(part.text)));
}

function bubbleRoleForMessage(role: string, kind?: string): EntryMessage['bubbleRole'] {
  const normalizedKind = (kind || '').trim().toLowerCase();
  if (normalizedKind === 'request') {
    return 'request';
  }
  if (normalizedKind === 'status') {
    return 'status';
  }
  const normalizedRole = role.trim().toLowerCase();
  if (normalizedRole === 'user') {
    return 'user';
  }
  if (normalizedRole === 'assistant' || normalizedRole === 'agent') {
    return 'assistant';
  }
  if (normalizedRole === 'system' || normalizedRole === 'developer') {
    return 'system';
  }
  return 'message';
}

function buildEntryMessage(
  entry: ThreadEntry,
  order: number,
  role: string,
  body: string,
  title?: string,
  options?: {
    kind?: string;
    id?: string;
    status?: string;
    channelID?: string;
    recordID?: string;
    actions?: Array<{ id: string; label: string; tone?: 'primary' | 'danger' }>;
    questions?: EntryMessageQuestion[];
    replyToMessageID?: string;
    actionID?: string;
    answers?: EntryMessageQuestionAnswer[];
  },
): ActivityTimelineItem | null {
  const normalizedBody = body.trim();
  const normalizedTitle = (title || '').trim();
  if (!normalizedBody && !normalizedTitle && !options?.questions?.length) {
    return null;
  }
  const normalizedRole = role.trim() || 'message';
  const normalizedKind = (options?.kind || '').trim();
  return {
    kind: 'message',
    ts: entryTimestampMs(entry),
    order,
    message: {
      id: options?.id || entry.id || `entry-message-${order}`,
      role: normalizedRole,
      bubbleRole: bubbleRoleForMessage(normalizedRole, normalizedKind),
      body: normalizedBody,
      ...(normalizedTitle ? { title: normalizedTitle } : {}),
      ...(options?.status ? { status: options.status } : {}),
      ...(options?.channelID ? { channelID: options.channelID } : {}),
      ...(options?.recordID ? { recordID: options.recordID } : {}),
      ...(options?.actions?.length ? { actions: options.actions } : {}),
      ...(options?.questions?.length ? { questions: options.questions } : {}),
      ...(options?.replyToMessageID ? { replyToMessageID: options.replyToMessageID } : {}),
      ...(options?.actionID ? { actionID: options.actionID } : {}),
      ...(options?.answers?.length ? { answers: options.answers } : {}),
    },
  };
}

function messageRecordTimestampMs(record: ChatMessageRecord): number {
  const parsed = Date.parse(asString(record.updatedAt) || asString(record.createdAt));
  return Number.isFinite(parsed) ? parsed : 0;
}

function collectEntryMessageRecordIDs(entries: ThreadEntry[]): Set<string> {
  const ids = new Set<string>();
  for (const entry of entries) {
    const entryType = asString(entry.type);
    if (entryType !== 'message_append' && entryType !== 'message_update') {
      continue;
    }
    const record = asRecord(entry.record);
    const id = asString(record?.id);
    if (id) {
      ids.add(id);
    }
  }
  return ids;
}

function threadMessageRecordTimelineItems(snapshot: ThreadSnapshot | null, startOrder: number): ActivityTimelineItem[] {
  const entries = Array.isArray(snapshot?.entries) ? snapshot.entries : [];
  const existingRecordIDs = collectEntryMessageRecordIDs(entries);
  const records = Array.isArray(snapshot?.messageRecords) ? snapshot.messageRecords : [];
  return records
    .filter((record: ChatMessageRecord) => {
      const id = asString(record.id);
      return id && !existingRecordIDs.has(id) && asString(record.status) !== 'archived';
    })
    .map((record, index) => {
      const normalizedTitle = asString(record.title);
      const role = asString(record.sender) || asString(record.kind) || 'message';
      const kind = asString(record.kind);
      return {
        kind: 'message' as const,
        ts: messageRecordTimestampMs(record),
        order: startOrder + index,
        message: {
          id: `message-record:${record.id}`,
          role,
          bubbleRole: bubbleRoleForMessage(role, kind),
          body: asString(record.body),
          ...(normalizedTitle ? { title: normalizedTitle } : {}),
          status: asString(record.status) || undefined,
          channelID: asString(record.channelID) || undefined,
          recordID: asString(record.id) || undefined,
          actions: Array.isArray(record.actions) ? record.actions : undefined,
          questions: Array.isArray(record.questions) ? record.questions : undefined,
          replyToMessageID: asString(record.replyToMessageID) || undefined,
          actionID: asString(record.actionID) || undefined,
          answers: Array.isArray(record.answers) ? record.answers : undefined,
        },
      };
    })
    .filter((item) => item.message.body || item.message.title || (item.message.questions?.length || 0) > 0);
}

function buildAnsweredRequestMap(items: ActivityTimelineItem[]): Record<string, boolean> {
  const result: Record<string, boolean> = {};
  for (const item of items) {
    if (item.kind !== 'message') {
      continue;
    }
    const message = item.message;
    const replyToMessageID = (message.replyToMessageID || '').trim();
    if (!replyToMessageID || message.bubbleRole !== 'user') {
      continue;
    }
    result[replyToMessageID] = true;
  }
  return result;
}

function buildMessageRecordByID(items: ActivityTimelineItem[]): Record<string, EntryMessage> {
  const result: Record<string, EntryMessage> = {};
  for (const item of items) {
    if (item.kind !== 'message') {
      continue;
    }
    const recordID = (item.message.recordID || '').trim();
    if (recordID) {
      result[recordID] = item.message;
    }
  }
  return result;
}

function buildRequestAnswerDetailsByID(items: ActivityTimelineItem[]): Record<string, EntryMessageQuestionAnswer[]> {
  const result: Record<string, EntryMessageQuestionAnswer[]> = {};
  for (const item of items) {
    if (item.kind !== 'message') {
      continue;
    }
    const message = item.message;
    const replyToMessageID = (message.replyToMessageID || '').trim();
    if (!replyToMessageID || message.bubbleRole !== 'user' || !message.answers?.length) {
      continue;
    }
    result[replyToMessageID] = message.answers;
  }
  return result;
}

function requestQuestionText(request: EntryMessage | undefined, questionID: string): string {
  const normalizedQuestionID = questionID.trim();
  if (!normalizedQuestionID || !request?.questions?.length) {
    return '';
  }
  const match = request.questions.find((question) => (question.id || '').trim() === normalizedQuestionID);
  return (match?.question || '').trim();
}

function formatRequestAnswerLabel(answer: EntryMessageQuestionAnswer): string {
  if (answer.other) {
    const text = (answer.text || '').trim();
    return text ? `Other — ${text}` : 'Other';
  }
  const label = (answer.label || '').trim();
  const optionID = (answer.optionID || '').trim();
  if (label && optionID && label !== optionID) {
    return `${label} (${optionID})`;
  }
  return label || optionID;
}

function summarizeRequestAnswers(answers: EntryMessageQuestionAnswer[] | undefined): string {
  if (!answers?.length) {
    return '';
  }
  return answers
    .map((answer) => formatRequestAnswerLabel(answer))
    .filter(Boolean)
    .join(', ');
}

function buildNoticeStep(entry: ThreadEntry, order: number, label: string, detail?: string): ActivityTimelineItem {
  return {
    kind: 'step',
    ts: entryTimestampMs(entry),
    order,
    step: {
      id: entry.id || `entry-step-${order}`,
      type: 'notice',
      label,
      status: 'done',
      detail: (detail || '').trim() || undefined,
      ts: entryTimestampMs(entry),
      order,
    },
  };
}

type ToolStepIndex = Map<string, Extract<ActivityTimelineItem, { kind: 'step' }>>;

function toolStepKey(toolCallID: string): string {
  return `tool:${toolCallID}`;
}

function pushStepItem(
  items: ActivityTimelineItem[],
  entry: ThreadEntry,
  order: number,
  step: LiveStep,
): Extract<ActivityTimelineItem, { kind: 'step' }> {
  const item: Extract<ActivityTimelineItem, { kind: 'step' }> = {
    kind: 'step',
    ts: entryTimestampMs(entry),
    order,
    step,
  };
  items.push(item);
  return item;
}

function upsertToolStep(
  items: ActivityTimelineItem[],
  toolSteps: ToolStepIndex,
  entry: ThreadEntry,
  order: number,
  step: LiveStep,
): void {
  const toolCallID = (step.toolCall?.id || '').trim();
  if (!toolCallID) {
    pushStepItem(items, entry, order, step);
    return;
  }
  const key = toolStepKey(toolCallID);
  const existing = toolSteps.get(key);
  if (!existing) {
    toolSteps.set(key, pushStepItem(items, entry, order, step));
    return;
  }
  existing.step = {
    ...existing.step,
    label: step.label || existing.step.label,
    status: step.status === 'error' ? 'error' : existing.step.status,
    detail: existing.step.detail || step.detail,
    toolCall: {
      ...(existing.step.toolCall || {}),
      ...(step.toolCall || {}),
      id: toolCallID,
    },
    toolOutput: step.toolOutput || existing.step.toolOutput,
  };
}

function toolResultOutputText(block: Record<string, unknown>, result: Record<string, unknown> | null): string {
  const direct = asString(result?.outputText) || asString(block.text);
  if (direct) {
    return direct;
  }
  const outputContent = contentBlocks(result?.outputContent);
  return compactText(outputContent.map((part) => asString(part.text)));
}

function canonicalMessageItems(
  entry: ThreadEntry,
  entryOrder: number,
  items: ActivityTimelineItem[],
  toolSteps: ToolStepIndex,
): void {
  const message = asRecord(entry.message);
  if (!message) {
    return;
  }
  const role = asString(message.role) || 'message';
  const blocks = contentBlocks(message.content);
  const text = textFromMessageContent(message);
  const isToolResultRole = role === 'tool_result';
  const textItem = isToolResultRole
    ? null
    : buildEntryMessage(entry, entryOrder, role, text);
  if (textItem) {
    items.push(textItem);
  }
  if (isToolResultRole && text) {
    pushStepItem(items, entry, entryOrder, {
      id: entry.id || `entry-tool-result-${entryOrder}`,
      type: 'toolcall',
      label: 'Tool result',
      status: 'done',
      toolOutput: text,
      ts: entryTimestampMs(entry),
      order: entryOrder,
    });
  }
  blocks.forEach((block, index) => {
    const type = asString(block.type);
    const order = entryOrder + index + 1;
    if (type === 'thinking') {
      const detail = asString(block.text) || asString(block.thinkingReplayField);
      if (detail) {
        pushStepItem(items, entry, order, {
          id: `${entry.id || 'entry'}-thinking-${index}`,
          type: 'reasoning',
          label: 'Thinking',
          status: 'done',
          detail,
          ts: entryTimestampMs(entry),
          order,
        });
      }
    } else if (type === 'tool_call') {
      const toolCall = asRecord(block.toolCall);
      const name = asString(toolCall?.name) || 'tool';
      const detail = asString(toolCall?.rawArguments) || stringifyToolArguments(toolCall?.arguments);
      upsertToolStep(items, toolSteps, entry, order, {
        id: asString(toolCall?.id) || `${entry.id || 'entry'}-tool-${index}`,
        type: 'toolcall',
        label: name,
        status: 'done',
        detail: detail || undefined,
        toolCall: {
          id: asString(toolCall?.id) || undefined,
          name,
          rawArguments: detail || undefined,
          arguments: asRecord(toolCall?.arguments) || undefined,
        },
        ts: entryTimestampMs(entry),
        order,
      });
    } else if (type === 'tool_result') {
      const result = asRecord(block.toolResult);
      const name = asString(result?.toolName) || 'tool';
      const toolCallID = asString(result?.toolCallID);
      const output = toolResultOutputText(block, result);
      upsertToolStep(items, toolSteps, entry, order, {
        id: toolCallID || `${entry.id || 'entry'}-tool-result-${index}`,
        type: 'toolcall',
        label: name,
        status: result?.isError === true ? 'error' : 'done',
        toolCall: toolCallID ? { id: toolCallID, name } : undefined,
        toolOutput: output || undefined,
        ts: entryTimestampMs(entry),
        order,
      });
    } else if (type === 'compaction') {
      const detail = asString(block.text);
      if (detail) {
        pushStepItem(items, entry, order, {
          id: `${entry.id || 'entry'}-compaction-${index}`,
          type: 'notice',
          label: 'Compacted',
          status: 'done',
          detail,
          ts: entryTimestampMs(entry),
          order,
        });
      }
    }
  });
}

function threadEntryTimelineItems(entries: ThreadEntry[]): ActivityTimelineItem[] {
  const items: ActivityTimelineItem[] = [];
  const toolSteps: ToolStepIndex = new Map();
  const recordItemIndex = new Map<string, number>();
  entries.forEach((entry, index) => {
    const entryType = asString(entry.type);
    const order = index * 100;
    if (entryType === 'canonical_message') {
      canonicalMessageItems(entry, order, items, toolSteps);
      return;
    }
    if (entryType === 'compaction') {
      items.push(buildNoticeStep(entry, order, 'Compacted', asString(entry.summary)));
      return;
    }
    if (entryType === 'message_append' || entryType === 'message_update') {
      const record = asRecord(entry.record);
      const status = asString(record?.status);
      if (status === 'archived') {
        return;
      }
      const recordID = asString(record?.id);
      const channelID = asString(record?.channelID);
      const actions = Array.isArray(record?.actions)
        ? record.actions as Array<{ id: string; label: string; tone?: 'primary' | 'danger' }>
        : undefined;
      const questions = Array.isArray(record?.questions)
        ? record.questions as EntryMessageQuestion[]
        : undefined;
      const answers = Array.isArray(record?.answers)
        ? record.answers as EntryMessageQuestionAnswer[]
        : undefined;
      const item = buildEntryMessage(
        entry,
        order,
        asString(record?.sender) || asString(record?.kind) || 'message',
        asString(record?.body),
        asString(record?.title),
        {
          kind: asString(record?.kind),
          id: recordID ? `message-record:${recordID}` : undefined,
          status: status || undefined,
          channelID,
          recordID,
          actions,
          questions,
          replyToMessageID: asString(record?.replyToMessageID) || undefined,
          actionID: asString(record?.actionID) || undefined,
          answers,
        },
      );
      if (!item) {
        return;
      }
      const existingIndex = recordID ? recordItemIndex.get(recordID) : undefined;
      if (existingIndex !== undefined) {
        // message_update collapses into the original message_append position so
        // a single record renders as one card with the latest state instead of
        // one card per append/update with a duplicate React key.
        items[existingIndex] = item;
        return;
      }
      if (recordID) {
        recordItemIndex.set(recordID, items.length);
      }
      items.push(item);
      return;
    }
    if (entryType === 'queue_enqueue') {
      const item = asRecord(entry.item);
      const queueKind = asString(entry.queueKind).replace(/_/g, ' ') || 'queued';
      const detail = textFromQueueMessage(item?.message);
      items.push(buildNoticeStep(entry, order, `Queued ${queueKind}`, detail));
      return;
    }
    if (entryType === 'queue_dequeue') {
      items.push(buildNoticeStep(entry, order, 'Delivered queued message'));
      return;
    }
    if (entryType === 'queue_remove') {
      items.push(buildNoticeStep(entry, order, 'Removed queued message'));
      return;
    }
    if (entryType === 'queue_promote') {
      items.push(buildNoticeStep(entry, order, 'Promoted queued message'));
      return;
    }
    if (entryType === 'review') {
      items.push(buildNoticeStep(entry, order, 'Review updated', asString(entry.status)));
      return;
    }
    if (entryType) {
      items.push(buildNoticeStep(entry, order, entryType));
    }
  });
  return items;
}

function isPendingReview(review: ThreadReviewState): boolean {
  return review.status === 'pending' || review.unresolved > 0;
}

function isActivityPanelNearBottom(element: HTMLDivElement): boolean {
  return element.scrollHeight - element.scrollTop - element.clientHeight
    <= ACTIVITY_PANEL_FOLLOW_BOTTOM_THRESHOLD_PX;
}

function ThreadActivityView({
  bodyRef,
  bodyMaxHeight,
  showEmptyState,
  children,
}: {
  bodyRef: React.Ref<HTMLDivElement>;
  bodyMaxHeight: number;
  showEmptyState: boolean;
  children: React.ReactNode;
}) {
  return (
    <div ref={bodyRef} className="op-activity-panel-body" style={{ height: bodyMaxHeight }}>
      <div className="op-activity-panel-column">
        {children}
        {showEmptyState && (
          <div className="op-activity-panel-empty-state">
            <div className="op-activity-panel-empty-title">Loading conversation...</div>
            <div className="op-activity-panel-empty-body">Messages and activity will appear here.</div>
          </div>
        )}
      </div>
    </div>
  );
}

export type AwaitingQuestionOptionItem = {
  badge: string;
  kind: 'option' | 'custom';
  label: string;
  description?: string;
  selected: boolean;
};

export type AwaitingQuestionKeyboardAction =
  | { kind: 'move'; direction: -1 | 1 }
  | { kind: 'navigate'; direction: -1 | 1 }
  | { kind: 'selectHighlighted' }
  | { kind: 'continue' }
  | { kind: 'cancel' };

function getCurrentAwaitingQuestion(awaitingUser: AwaitingUserState | null | undefined) {
  if (!awaitingUser || awaitingUser.questions.length === 0) {
    return null;
  }
  return awaitingUser.questions[awaitingUser.currentIndex] || awaitingUser.questions[0] || null;
}

function getCurrentQuestionOptionLabels(question: NonNullable<ReturnType<typeof getCurrentAwaitingQuestion>>): Set<string> {
  return new Set(question.options.map((option) => option.label));
}

function isFreeformOnlyQuestion(question: NonNullable<ReturnType<typeof getCurrentAwaitingQuestion>>): boolean {
  return question.custom !== false && question.options.length === 0;
}

export function getCurrentCustomAnswer(awaitingUser: AwaitingUserState): string {
  const question = getCurrentAwaitingQuestion(awaitingUser);
  if (!question) {
    return '';
  }
  const currentAnswers = awaitingUser.answers[awaitingUser.currentIndex] || [];
  const optionLabels = getCurrentQuestionOptionLabels(question);
  return currentAnswers.find((answer) => !optionLabels.has(answer)) || '';
}

export function buildAwaitingQuestionOptionItems(awaitingUser: AwaitingUserState): AwaitingQuestionOptionItem[] {
  const question = getCurrentAwaitingQuestion(awaitingUser);
  if (!question) {
    return [];
  }
  if (isFreeformOnlyQuestion(question)) {
    return [];
  }
  const currentAnswers = awaitingUser.answers[awaitingUser.currentIndex] || [];
  const customMode = awaitingUser.customModeByIndex[awaitingUser.currentIndex] === true;
  const items: AwaitingQuestionOptionItem[] = question.options.map((option, index) => ({
    badge: String.fromCharCode(65 + index),
    kind: 'option',
    label: option.label,
    description: option.description,
    selected: currentAnswers.includes(option.label) && !(!question.multiple && customMode),
  }));
  if (question.custom !== false) {
    items.push({
      badge: String.fromCharCode(65 + items.length),
      kind: 'custom',
      label: 'Other...',
      selected: customMode,
    });
  }
  return items;
}

function cloneAwaitingAnswers(awaitingUser: AwaitingUserState): string[][] {
  return awaitingUser.answers.map((answers) => [...answers]);
}

function cloneAwaitingCustomModes(awaitingUser: AwaitingUserState): boolean[] {
  return [...awaitingUser.customModeByIndex];
}

export function selectAwaitingUserOption(awaitingUser: AwaitingUserState, optionIndex: number): AwaitingUserState {
  const question = getCurrentAwaitingQuestion(awaitingUser);
  if (!question) {
    return awaitingUser;
  }
  const items = buildAwaitingQuestionOptionItems(awaitingUser);
  const item = items[optionIndex];
  if (!item) {
    return awaitingUser;
  }

  const currentIndex = awaitingUser.currentIndex;
  const answers = cloneAwaitingAnswers(awaitingUser);
  const customModeByIndex = cloneAwaitingCustomModes(awaitingUser);
  const optionLabels = getCurrentQuestionOptionLabels(question);

  if (item.kind === 'custom') {
    if (question.multiple) {
      const nextCustomMode = !customModeByIndex[currentIndex];
      customModeByIndex[currentIndex] = nextCustomMode;
      if (!nextCustomMode) {
        answers[currentIndex] = answers[currentIndex].filter((answer) => optionLabels.has(answer));
      }
      return {
        ...awaitingUser,
        answers,
        customModeByIndex,
      };
    }
    customModeByIndex[currentIndex] = true;
    answers[currentIndex] = getCurrentCustomAnswer(awaitingUser)
      ? [getCurrentCustomAnswer(awaitingUser)]
      : [];
    return {
      ...awaitingUser,
      answers,
      customModeByIndex,
    };
  }

  if (question.multiple) {
    if (answers[currentIndex].includes(item.label)) {
      answers[currentIndex] = answers[currentIndex].filter((answer) => answer !== item.label);
    } else {
      answers[currentIndex] = [...answers[currentIndex], item.label];
    }
    return {
      ...awaitingUser,
      answers,
      customModeByIndex,
    };
  }

  customModeByIndex[currentIndex] = false;
  answers[currentIndex] = [item.label];
  return {
    ...awaitingUser,
    answers,
    customModeByIndex,
  };
}

export function updateAwaitingUserCustomAnswer(awaitingUser: AwaitingUserState, value: string): AwaitingUserState {
  const question = getCurrentAwaitingQuestion(awaitingUser);
  if (!question || question.custom === false) {
    return awaitingUser;
  }
  const currentIndex = awaitingUser.currentIndex;
  const answers = cloneAwaitingAnswers(awaitingUser);
  const customModeByIndex = cloneAwaitingCustomModes(awaitingUser);
  const optionLabels = getCurrentQuestionOptionLabels(question);
  const trimmedValue = value;

  customModeByIndex[currentIndex] = true;
  if (question.multiple) {
    const nextAnswers = answers[currentIndex].filter((answer) => optionLabels.has(answer));
    if (trimmedValue.trim()) {
      nextAnswers.push(trimmedValue);
    }
    answers[currentIndex] = nextAnswers;
  } else {
    answers[currentIndex] = trimmedValue.trim() ? [trimmedValue] : [];
  }
  return {
    ...awaitingUser,
    answers,
    customModeByIndex,
  };
}

export function canContinueAwaitingUser(awaitingUser: AwaitingUserState): boolean {
  const question = getCurrentAwaitingQuestion(awaitingUser);
  if (!question) {
    return false;
  }
  if (question.multiple) {
    return true;
  }
  if (isFreeformOnlyQuestion(question)) {
    return true;
  }
  if (awaitingUser.customModeByIndex[awaitingUser.currentIndex]) {
    return getCurrentCustomAnswer(awaitingUser).trim().length > 0;
  }
  return (awaitingUser.answers[awaitingUser.currentIndex] || []).length > 0;
}

export function isLastAwaitingUserQuestion(awaitingUser: AwaitingUserState): boolean {
  return awaitingUser.currentIndex >= awaitingUser.questions.length - 1;
}

export function advanceAwaitingUserQuestion(awaitingUser: AwaitingUserState): AwaitingUserState {
  if (isLastAwaitingUserQuestion(awaitingUser)) {
    return awaitingUser;
  }
  return {
    ...awaitingUser,
    currentIndex: awaitingUser.currentIndex + 1,
  };
}

export function skipAwaitingUserQuestion(awaitingUser: AwaitingUserState): AwaitingUserState {
  const currentIndex = awaitingUser.currentIndex;
  const answers = cloneAwaitingAnswers(awaitingUser);
  const customModeByIndex = cloneAwaitingCustomModes(awaitingUser);
  answers[currentIndex] = [];
  customModeByIndex[currentIndex] = false;
  return {
    ...awaitingUser,
    answers,
    customModeByIndex,
  };
}

export function resolveAwaitingUserKeyboardAction(params: {
  awaitingUser: AwaitingUserState;
  highlightedIndex: number;
  key: string;
  metaKey?: boolean;
  ctrlKey?: boolean;
  altKey?: boolean;
}): AwaitingQuestionKeyboardAction | null {
  const key = params.key.toLowerCase();
  if (params.altKey) {
    return null;
  }
  if (key === 'arrowleft') {
    return { kind: 'navigate', direction: -1 };
  }
  if (key === 'arrowright') {
    return { kind: 'navigate', direction: 1 };
  }
  if (key === 'arrowup') {
    return { kind: 'move', direction: -1 };
  }
  if (key === 'arrowdown') {
    return { kind: 'move', direction: 1 };
  }
  if (key === 'escape') {
    return { kind: 'cancel' };
  }
  if (key === 'enter' && (params.metaKey || params.ctrlKey) && getCurrentAwaitingQuestion(params.awaitingUser)?.custom !== false) {
    return { kind: 'continue' };
  }
  if (key === 'enter') {
    const items = buildAwaitingQuestionOptionItems(params.awaitingUser);
    const highlighted = items[params.highlightedIndex];
    if (!highlighted) {
      return canContinueAwaitingUser(params.awaitingUser)
        ? { kind: 'continue' }
        : null;
    }
    return highlighted.selected && canContinueAwaitingUser(params.awaitingUser)
      ? { kind: 'continue' }
      : { kind: 'selectHighlighted' };
  }
  return null;
}

function isEditableQuestionTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  const tagName = target.tagName.toLowerCase();
  return tagName === 'input' || tagName === 'textarea' || target.isContentEditable;
}

export function navigateAwaitingUserQuestion(
  awaitingUser: AwaitingUserState,
  direction: -1 | 1,
): AwaitingUserState {
  const total = awaitingUser.questions.length;
  if (total <= 1) {
    return awaitingUser;
  }
  const nextIndex = Math.min(total - 1, Math.max(0, awaitingUser.currentIndex + direction));
  if (nextIndex === awaitingUser.currentIndex) {
    return awaitingUser;
  }
  return {
    ...awaitingUser,
    currentIndex: nextIndex,
  };
}

function ErrorCard({ message }: { message: string }) {
  return (
    <div className="rounded-md border border-border px-3 py-2">
      <div className="mb-2 text-xs uppercase tracking-wide text-secondary-text">Error</div>
      <pre className="op-activity-panel-text whitespace-pre-wrap">{message}</pre>
    </div>
  );
}

function RetryCard({
  attempt,
  limit,
  delayMs,
  startedAt,
  errorMessage,
  now,
}: {
  attempt: number;
  limit: number;
  delayMs: number;
  startedAt: number | null;
  errorMessage: string | null;
  now: number;
}) {
  const remainingMs = startedAt == null
    ? delayMs
    : Math.max(0, delayMs - Math.max(0, now - startedAt));
  const remainingSeconds = Math.max(1, Math.ceil(remainingMs / 1000));

  return (
    <div className="rounded-md border border-border px-3 py-2">
      <div className="mb-2 text-xs uppercase tracking-wide text-secondary-text">
        Retrying ({attempt}/{limit || 0}) in {remainingSeconds}s
      </div>
      {errorMessage && (
        <pre className="op-activity-panel-text whitespace-pre-wrap">{errorMessage}</pre>
      )}
    </div>
  );
}

export function EmbeddedQuestionsCard({
  awaitingUser,
  busy,
  error,
  highlightedIndex,
  onPrevious,
  onNext,
  onSelectOption,
  onCustomAnswerChange,
  onSkip,
  onCancel,
  onContinue,
}: {
  awaitingUser: AwaitingUserState;
  busy: boolean;
  error: string | null;
  highlightedIndex: number;
  onPrevious: () => void;
  onNext: () => void;
  onSelectOption: (index: number) => void;
  onCustomAnswerChange: (value: string) => void;
  onSkip: () => void;
  onCancel: () => void;
  onContinue: () => void;
}) {
  const question = getCurrentAwaitingQuestion(awaitingUser);
  if (!question) {
    return null;
  }
  const items = buildAwaitingQuestionOptionItems(awaitingUser);
  const currentCustomAnswer = getCurrentCustomAnswer(awaitingUser);
  const continueDisabled = busy || !canContinueAwaitingUser(awaitingUser);
  const freeformOnly = isFreeformOnlyQuestion(question);
  const questionNumber = awaitingUser.currentIndex + 1;
  const totalQuestions = awaitingUser.questions.length;
  const previousDisabled = busy || questionNumber <= 1;
  const nextDisabled = busy || questionNumber >= totalQuestions;

  return (
    <div className="mt-3 rounded-md border border-border/70 bg-editor-bg/60 px-3 py-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2 text-sm font-medium text-prime-text">
          <ChatLineIcon className="w-4 h-4 text-secondary-text" />
          <span>Questions</span>
        </div>
        <div className="flex items-center gap-1.5 text-xs text-secondary-text">
          <button
            type="button"
            className="icon-gutter-btn-sm icon-button-inline disabled:opacity-40"
            onClick={onPrevious}
            disabled={previousDisabled}
            aria-label="Previous question"
            title="Previous question"
          >
            <span className="block rotate-180">
              <ChevronRightIcon className="w-3 h-3" />
            </span>
          </button>
          <span className="min-w-[42px] text-center">{questionNumber} of {totalQuestions}</span>
          <button
            type="button"
            className="icon-gutter-btn-sm icon-button-inline disabled:opacity-40"
            onClick={onNext}
            disabled={nextDisabled}
            aria-label="Next question"
            title="Next question"
          >
            <ChevronRightIcon className="w-3 h-3" />
          </button>
          <button
            type="button"
            className="icon-gutter-btn-sm icon-button-inline disabled:opacity-40"
            onClick={onCancel}
            disabled={busy}
            aria-label="Cancel questions"
            title="Cancel questions"
          >
            <CloseTinyIcon className="w-3 h-3" />
          </button>
        </div>
      </div>

      <div className="mt-2 flex items-start gap-2.5">
        <div className="pt-0.5 text-[17px] font-semibold leading-6 text-prime-text">
          {questionNumber}.
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-[15px] font-medium leading-6 text-prime-text whitespace-pre-wrap">
            {question.question}
          </div>

          {freeformOnly ? (
            <div className="mt-2.5">
              <input
                value={currentCustomAnswer}
                onChange={(event) => onCustomAnswerChange(event.target.value)}
                placeholder="Answer (optional)"
                disabled={busy}
                className="w-full rounded border border-border bg-editor-bg px-3 py-2 text-sm text-prime-text outline-none focus:border-active-border disabled:opacity-50"
              />
            </div>
          ) : (
            <div className="mt-2.5 space-y-1">
              {items.map((item, index) => {
                const isHighlighted = index === highlightedIndex;
                return (
                  <div key={`${awaitingUser.requestID}:${awaitingUser.currentIndex}:${item.badge}`}>
                    <button
                      type="button"
                      className={`flex w-full items-start gap-3 rounded px-2 py-1.5 text-left transition-colors ${
                        item.selected
                          ? 'bg-hover-bg text-prime-text'
                          : isHighlighted
                            ? 'bg-hover-bg/40 text-prime-text'
                            : 'text-secondary-text hover:bg-hover-bg/60 hover:text-prime-text'
                      }`}
                      onClick={() => onSelectOption(index)}
                      disabled={busy}
                    >
                      <span className={`inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md border text-[13px] font-semibold ${
                        item.selected
                          ? 'border-highlight bg-highlight text-sidebar-bg'
                          : 'border-border text-secondary-text'
                      }`}>
                        {item.badge}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block text-[14px] font-medium leading-6">
                          {item.label}
                        </span>
                        {item.description && (
                          <span className="block text-[12px] leading-5 text-secondary-text">
                            {item.description}
                          </span>
                        )}
                      </span>
                    </button>
                    {item.kind === 'custom' && item.selected && (
                      <div className="mt-1.5 pl-11">
                        <input
                          value={currentCustomAnswer}
                          onChange={(event) => onCustomAnswerChange(event.target.value)}
                          placeholder="Type your own answer"
                          disabled={busy}
                          className="w-full rounded border border-border bg-editor-bg px-3 py-2 text-sm text-prime-text outline-none focus:border-active-border disabled:opacity-50"
                        />
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <div className="mt-2.5 flex items-center justify-between gap-3">
        <div className={`text-xs ${error ? 'text-accent' : 'text-secondary-text'}`}>
          {error || (freeformOnly
            ? 'Esc cancel · Enter continue'
            : 'Esc cancel · Enter select/continue · Ctrl/Cmd+Enter continue custom answer')}
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="ui-pill-btn-secondary px-3 py-1.5 text-xs disabled:opacity-50"
            onClick={onSkip}
            disabled={busy}
          >
            Skip
          </button>
          <button
            type="button"
            className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor px-3 py-1.5 text-xs disabled:opacity-50"
            onClick={onContinue}
            disabled={continueDisabled}
          >
            Continue
          </button>
        </div>
      </div>
    </div>
  );
}

function formatReviewHistoryMetric(reviews: ThreadReviewState[]): string {
  return `${reviews.length} review${reviews.length === 1 ? '' : 's'}`;
}

function formatReviewSummary(review: ThreadReviewState): string {
  const parts: string[] = [`${review.files.length} file${review.files.length === 1 ? '' : 's'}`];
  if ((review.conflictCount || 0) > 0) {
    parts.push(`${review.conflictCount} conflict${review.conflictCount === 1 ? '' : 's'}`);
  }
  if (review.unresolved > 0) {
    parts.push(`${review.unresolved} pending`);
  }
  if (review.approvedCount > 0) {
    parts.push(`${review.approvedCount} approved`);
  }
  if (review.rolledBackCount > 0) {
    parts.push(`${review.rolledBackCount} rolled back`);
  }
  if (review.rejectedCount > 0) {
    parts.push(`${review.rejectedCount} rejected`);
  }
  return parts.join(' · ');
}

function formatReviewTimestamp(createdAt: string): string {
  const date = new Date(createdAt);
  if (Number.isNaN(date.getTime())) {
    return createdAt;
  }
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function canShowReviewOverlay(file: ThreadReviewFile): boolean {
  if ((file.changedRanges || []).length === 0 && (file.hunks || []).length === 0) {
    return false;
  }
  return file.status === 'pending';
}

function findReviewFileForOverlay(
  reviews: ThreadReviewState[],
  overlay: { turnID: string; filePath: string } | null
): ThreadReviewFile | null {
  if (!overlay) {
    return null;
  }
  for (const review of reviews) {
    if (review.turnID !== overlay.turnID) {
      continue;
    }
    for (const file of review.files) {
      if (file.path === overlay.filePath) {
        return file;
      }
    }
  }
  return null;
}

function splitReviewPath(filePath: string): { name: string; dir: string } {
  const normalized = (filePath || '').replace(/\\/g, '/').replace(/\/+$/, '');
  const parts = normalized.split('/');
  const name = parts.pop() || normalized || 'Untitled';
  return {
    name,
    dir: parts.length > 0 ? (parts.join('/') || '/') : '',
  };
}

function getReviewFileStateBadge(file: ThreadReviewFile): string | null {
  if (file.status !== 'pending' && file.status !== 'approved') {
    return null;
  }
  switch (file.mergeState) {
    case 'userEdited':
      return 'edited';
    case 'conflicted':
      return 'conflict';
    case 'missing':
      return 'missing';
    case 'userUndone':
      return 'undone';
    default:
      return null;
  }
}

function canUndoReviewFile(file: ThreadReviewFile): boolean {
  if (file.status !== 'pending') {
    return false;
  }
  if (file.mergeState === 'conflicted' || file.mergeState === 'missing') {
    return false;
  }
  return file.canUndo !== false;
}

function canRollbackReviewFile(file: ThreadReviewFile): boolean {
  if (file.status !== 'approved') {
    return false;
  }
  if (file.mergeState === 'conflicted' || file.mergeState === 'missing') {
    return false;
  }
  return file.canUndo !== false;
}

function canKeepReviewFile(file: ThreadReviewFile): boolean {
  if (file.status !== 'pending') {
    return false;
  }
  return file.mergeState !== 'missing';
}

function getKeepReviewFileLabel(file: ThreadReviewFile): string {
  return file.hasUserEdits || file.mergeState === 'conflicted' || file.mergeState === 'userUndone'
    ? 'Keep current'
    : 'Keep';
}

function getLatestStreamingSegmentText(segments: LiveTextSegment[]): string {
  for (let index = segments.length - 1; index >= 0; index -= 1) {
    const text = segments[index]?.text || '';
    if (text.trim()) {
      return text;
    }
  }
  return '';
}

function tokenUsageFromSnapshot(snapshot: ThreadSnapshot | null): TokenUsage {
  const usage = snapshot?.contextUsage;
  const contextWindow = Number(usage?.contextWindow || 0);
  const contextTokens = Number(usage?.tokens || 0);
  const percentMilli = Number(usage?.percentMilli || 0);
  return {
    inputTokens: 0,
    outputTokens: 0,
    totalTokens: 0,
    ...(contextTokens > 0 ? { contextTokens } : {}),
    ...(contextWindow > 0 ? { contextWindow } : {}),
    ...(contextWindow > 0 ? { contextKnown: usage?.known === true || contextTokens > 0 } : {}),
    ...(percentMilli > 0 ? { contextPercent: percentMilli / 1000 } : {}),
  };
}

function hasTokenUsage(usage: TokenUsage): boolean {
  return Boolean(
    usage.totalTokens
    || usage.inputTokens
    || usage.outputTokens
    || usage.contextWindow
    || usage.contextTokens
    || typeof usage.contextPercent === 'number'
  );
}

function mergeTokenUsage(snapshot: ThreadSnapshot | null, liveUsage: TokenUsage): TokenUsage {
  return hasTokenUsage(liveUsage) ? liveUsage : tokenUsageFromSnapshot(snapshot);
}

function latestTimelineText(items: ActivityTimelineItem[]): string {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index];
    if (item.kind === 'message' && item.message.body.trim()) {
      return item.message.body;
    }
    if (item.kind === 'step') {
      const content = getLiveStepContent(item.step) || '';
      if (content.trim()) {
        return content;
      }
    }
    if (item.kind === 'stream' && item.segment.text.trim()) {
      return item.segment.text;
    }
  }
  return '';
}

function resolveActivityDisplayName(
  profile?: ActivityUserProfileLike,
  email?: string,
  uid?: string,
): string {
  return (profile?.name || profile?.username || profile?.email || email || uid || '').trim();
}

function resolveActivityParticipantAvatarSrc(profile?: ActivityUserProfileLike | null): string | null {
  return resolveUserAvatarSrc(profile || undefined);
}

function messageStatusLabel(status: string | undefined): string {
  const normalized = (status || '').trim().toLowerCase();
  if (normalized === 'resolved') {
    return 'Closed';
  }
  if (normalized === 'open') {
    return 'Open';
  }
  return normalized;
}

function resolveActivityMessageParticipant(
  message: EntryMessage,
  participants: ActivityMessageParticipants,
): ActivityMessageParticipant {
  if (message.bubbleRole === 'user') {
    return participants.user;
  }
  // Agent-published request/status messages belong to the assistant speaker.
  // Routing them through the assistant participant keeps the agent name/avatar
  // consistent across a turn and lets the header show the request title as a
  // title rather than impersonating it as the participant name.
  if (message.role === 'agent' || message.bubbleRole === 'assistant') {
    return participants.assistant;
  }
  const label = messageLabel(message);
  return {
    kind: message.bubbleRole,
    name: label,
    avatarSrc: null,
    fallbackText: label,
  };
}

function ActivityMessageAvatar({ participant }: { participant: ActivityMessageParticipant }) {
  const [imageFailed, setImageFailed] = useState(false);
  const [resolvedAvatarSrc, setResolvedAvatarSrc] = useState('');
  const avatarSrc = participant.avatarSrc;

  useEffect(() => {
    let cancelled = false;
    setImageFailed(false);
    setResolvedAvatarSrc('');
    if (!avatarSrc) {
      return () => {
        cancelled = true;
      };
    }
    void resolveLooseResourceUrl(avatarSrc)
      .then((resolved) => {
        if (!cancelled) {
          setResolvedAvatarSrc((resolved || '').trim());
        }
      })
      .catch(() => {
        if (!cancelled) {
          setResolvedAvatarSrc('');
        }
      });
    return () => {
      cancelled = true;
    };
  }, [avatarSrc]);

  const fallbackName = participant.fallbackText || participant.name || 'Message';
  const initials = buildInitials(fallbackName);
  const backgroundColor = initialsBackgroundColor(fallbackName);
  return (
    <span className={`op-activity-panel-message-avatar is-${participant.kind}`} aria-hidden="true">
      {resolvedAvatarSrc && !imageFailed ? (
        <img
          src={resolvedAvatarSrc}
          alt=""
          className="op-activity-panel-message-avatar-img"
          onError={() => setImageFailed(true)}
        />
      ) : (
        <span
          className="op-activity-panel-message-avatar-fallback"
          style={{ backgroundColor }}
        >
          {initials}
        </span>
      )}
    </span>
  );
}

function ActivityMessageHeader({
  message,
  participant,
  hideTitle = false,
}: {
  message: EntryMessage;
  participant: ActivityMessageParticipant;
  hideTitle?: boolean;
}) {
  const status = messageStatusLabel(message.status);
  const title = !hideTitle && message.title && message.title !== participant.name ? message.title : '';
  return (
    <div className="op-activity-panel-message-meta">
      <span className="op-activity-panel-message-author">{participant.name}</span>
      {status ? <span className={`op-activity-panel-message-status is-${status.toLowerCase()}`}>{status}</span> : null}
      {title ? <span className="op-activity-panel-message-title">{title}</span> : null}
    </div>
  );
}

function ActivityParticipantFrame({
  participant,
  showParticipantHeader,
  headerMessage,
  hideHeaderTitle = false,
  children,
}: {
  participant: ActivityMessageParticipant;
  showParticipantHeader: boolean;
  headerMessage: EntryMessage;
  hideHeaderTitle?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className={`op-activity-panel-message-row is-${participant.kind}`}>
      {showParticipantHeader ? (
        <div className="op-activity-panel-message-author-column">
          <ActivityMessageAvatar participant={participant} />
        </div>
      ) : (
        <span className="op-activity-panel-message-author-column-spacer" aria-hidden="true" />
      )}
      <div className="op-activity-panel-message-stack">
        {showParticipantHeader ? (
          <ActivityMessageHeader message={headerMessage} participant={participant} hideTitle={hideHeaderTitle} />
        ) : null}
        {children}
      </div>
    </div>
  );
}

function StreamingTextView({
  text,
  participant,
  showParticipantHeader,
}: {
  text: string;
  participant: ActivityMessageParticipant;
  showParticipantHeader: boolean;
}) {
  const headerMessage: EntryMessage = {
    id: 'streaming',
    role: 'assistant',
    bubbleRole: 'assistant',
    body: text,
  };
  return (
    <ActivityParticipantFrame
      participant={participant}
      showParticipantHeader={showParticipantHeader}
      headerMessage={headerMessage}
    >
      <div className="op-activity-panel-message-bubble is-assistant is-streaming">
        <ActivityMarkdownView text={text} className="op-activity-panel-message-body" />
      </div>
    </ActivityParticipantFrame>
  );
}

function messageLabel(message: EntryMessage): string {
  if (message.title) {
    return message.title;
  }
  if (message.bubbleRole === 'request') {
    return 'request';
  }
  if (message.bubbleRole === 'status') {
    return 'status';
  }
  return message.role || 'message';
}

function requestAnswerForQuestion(
  answers: EntryMessageQuestionAnswer[] | undefined,
  questionID: string,
): EntryMessageQuestionAnswer | undefined {
  const normalizedQuestionID = questionID.trim();
  return (answers || []).find((answer) => (answer.questionID || '').trim() === normalizedQuestionID);
}

function RequestQuestionsReadonly({
  questions,
  answers,
}: {
  questions: EntryMessageQuestion[];
  answers?: EntryMessageQuestionAnswer[];
}) {
  return (
    <div className="op-activity-panel-question-list is-open-request">
      {questions.map((question) => {
        const questionID = (question.id || '').trim();
        const options = Array.isArray(question.options) ? question.options : [];
        const answer = requestAnswerForQuestion(answers, questionID);
        return (
          <div key={questionID || question.question} className="op-activity-panel-question">
            <div className="op-activity-panel-question-text">{question.question}</div>
            <ol className="op-activity-panel-choice-list is-request-decision is-answered" aria-label={question.question}>
              {options.map((option, optionIndex) => {
                const optionID = (option.id || '').trim();
                const selected = Boolean(
                  answer
                  && !answer.other
                  && (optionID ? (answer.optionID || '').trim() === optionID : (answer.label || '') === option.label),
                );
                const keycap = activityPanelActionKeycap(optionIndex);
                return (
                  <li key={optionID || option.label} className="op-activity-panel-choice-item">
                    <button
                      type="button"
                      className={['op-activity-panel-choice-button', selected ? 'is-selected' : ''].filter(Boolean).join(' ')}
                      disabled
                      aria-pressed={selected}
                      aria-label={`${keycap}. ${option.label}`}
                    >
                      <span className="op-activity-panel-choice-key" aria-hidden="true">{keycap}</span>
                      <span className="op-activity-panel-choice-label">
                        {option.label}
                        {selected ? (
                          <span className="op-activity-panel-choice-badge">Selected</span>
                        ) : null}
                      </span>
                    </button>
                  </li>
                );
              })}
              {answer?.other ? (
                <li className="op-activity-panel-choice-item">
                  <button
                    type="button"
                    className="op-activity-panel-choice-button is-selected"
                    disabled
                    aria-pressed="true"
                    aria-label={`${activityPanelActionKeycap(options.length)}. Other`}
                  >
                    <span className="op-activity-panel-choice-key" aria-hidden="true">
                      {activityPanelActionKeycap(options.length)}
                    </span>
                    <span className="op-activity-panel-choice-label">
                      {(answer.text || '').trim() || 'Other'}
                      <span className="op-activity-panel-choice-badge">Selected</span>
                    </span>
                  </button>
                </li>
              ) : null}
            </ol>
          </div>
        );
      })}
    </div>
  );
}

function EntryRequestAnswerCard({
  message,
  request,
}: {
  message: EntryMessage;
  request?: EntryMessage;
}) {
  const requestTitle = (request?.title || '').trim() || 'Request';
  const answers = message.answers || [];
  const questions = request?.questions || [];
  return (
    <div className="op-activity-panel-request-answer-card" aria-label="Request answer">
      <div className="op-activity-panel-request-answer-kicker">Answered request</div>
      <div className="op-activity-panel-open-request-title">{requestTitle}</div>
      {questions.length ? (
        <RequestQuestionsReadonly questions={questions} answers={answers} />
      ) : (
        <div className="op-activity-panel-request-answer-list">
          {answers.map((answer) => {
            const questionID = (answer.questionID || '').trim();
            const questionText = requestQuestionText(request, questionID);
            return (
              <div key={`${questionID}:${answer.optionID || answer.text || answer.label || 'answer'}`} className="op-activity-panel-request-answer-item">
                {questionText ? (
                  <div className="op-activity-panel-request-answer-question">{questionText}</div>
                ) : null}
                <div className="op-activity-panel-request-answer-value">{formatRequestAnswerLabel(answer)}</div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function EntryOpenRequestCard({
  message,
  questionTitle,
  messageKey,
  messageCanReply,
  pendingActionID,
  openOtherQuestionID,
  setOpenOtherQuestionID,
  otherAnswerByQuestionID,
  setOtherAnswerByQuestionID,
  onAnswer,
}: {
  message: EntryMessage;
  questionTitle: string;
  messageKey: string;
  messageCanReply: boolean;
  pendingActionID?: string | null;
  openOtherQuestionID: string | null;
  setOpenOtherQuestionID: React.Dispatch<React.SetStateAction<string | null>>;
  otherAnswerByQuestionID: Record<string, string>;
  setOtherAnswerByQuestionID: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  onAnswer?: (message: EntryMessage, answer: EntryMessageQuestionAnswer) => Promise<boolean> | boolean;
}) {
  const [detailsExpanded, setDetailsExpanded] = useState(false);

  useEffect(() => {
    setDetailsExpanded(false);
  }, [message.id, message.body]);

  const questionAnswerPendingKey = (questionID: string, answerKey: string) => (
    `${messageKey}:${questionID}:${answerKey}`
  );
  const submitQuestionAnswer = async (answer: EntryMessageQuestionAnswer) => {
    if (!messageCanReply || !onAnswer || pendingActionID) {
      return;
    }
    const ok = await onAnswer(message, answer);
    if (ok && answer.other) {
      setOpenOtherQuestionID(null);
      setOtherAnswerByQuestionID((current) => ({
        ...current,
        [answer.questionID]: '',
      }));
    }
  };

  return (
    <div className="op-activity-panel-open-request" aria-label="Open request">
      {questionTitle ? (
        <div className="op-activity-panel-open-request-title">{questionTitle}</div>
      ) : null}
      <div className="op-activity-panel-question-list is-open-request">
        {(message.questions || []).map((question) => {
          const questionID = (question.id || '').trim();
          const options = Array.isArray(question.options) ? question.options : [];
          const otherInputID = `op-activity-panel-other-${messageKey}-${questionID}`.replace(/[^a-zA-Z0-9_-]/g, '-');
          const otherText = otherAnswerByQuestionID[questionID] || '';
          const trimmedOtherText = otherText.trim();
          const otherPendingKey = questionAnswerPendingKey(questionID, 'other');
          const otherBusy = pendingActionID === otherPendingKey;
          return (
            <div key={questionID || question.question} className="op-activity-panel-question">
              <div className="op-activity-panel-question-text">{question.question}</div>
              <ol className="op-activity-panel-choice-list is-request-decision" aria-label={question.question}>
                {options.map((option, optionIndex) => {
                  const optionID = (option.id || '').trim();
                  const answerKey = optionID || option.label;
                  const pendingKey = questionAnswerPendingKey(questionID, answerKey);
                  const busy = pendingActionID === pendingKey;
                  const disabled = Boolean(pendingActionID) || !messageCanReply;
                  const keycap = activityPanelActionKeycap(optionIndex);
                  return (
                    <li key={optionID || option.label} className="op-activity-panel-choice-item">
                      <button
                        type="button"
                        className="op-activity-panel-choice-button"
                        disabled={disabled}
                        onClick={() => {
                          void submitQuestionAnswer({
                            questionID,
                            optionID,
                            label: option.label,
                          });
                        }}
                        aria-label={`${keycap}. ${option.label}`}
                      >
                        <span className="op-activity-panel-choice-key" aria-hidden="true">{keycap}</span>
                        <span className="op-activity-panel-choice-label">
                          {busy ? 'Sending...' : option.label}
                        </span>
                      </button>
                    </li>
                  );
                })}
                <li className="op-activity-panel-choice-item">
                  <button
                    type="button"
                    className={[
                      'op-activity-panel-choice-button',
                      openOtherQuestionID === questionID ? 'is-custom-open' : '',
                    ].filter(Boolean).join(' ')}
                    disabled={Boolean(pendingActionID) || !messageCanReply}
                    onClick={() => setOpenOtherQuestionID(questionID)}
                    aria-label={`${activityPanelActionKeycap(options.length)}. Other`}
                    aria-expanded={openOtherQuestionID === questionID}
                    aria-controls={openOtherQuestionID === questionID ? otherInputID : undefined}
                  >
                    <span className="op-activity-panel-choice-key" aria-hidden="true">
                      {activityPanelActionKeycap(options.length)}
                    </span>
                    <span className="op-activity-panel-choice-label">
                      {otherBusy ? 'Sending...' : 'Other...'}
                    </span>
                  </button>
                  {messageCanReply && openOtherQuestionID === questionID ? (
                    <form
                      className="op-activity-panel-choice-custom-form"
                      onSubmit={(event) => {
                        event.preventDefault();
                        if (!trimmedOtherText) {
                          return;
                        }
                        void submitQuestionAnswer({
                          questionID,
                          other: true,
                          label: 'Other',
                          text: trimmedOtherText,
                        });
                      }}
                    >
                      <span className="op-activity-panel-choice-custom-spacer" aria-hidden="true" />
                      <input
                        id={otherInputID}
                        type="text"
                        value={otherText}
                        onChange={(event) => setOtherAnswerByQuestionID((current) => ({
                          ...current,
                          [questionID]: event.target.value,
                        }))}
                        onKeyDown={(event) => {
                          if (event.key === 'Escape') {
                            event.preventDefault();
                            setOpenOtherQuestionID(null);
                          }
                        }}
                        placeholder="Type your own answer"
                        disabled={Boolean(pendingActionID)}
                        className="op-activity-panel-choice-custom-input"
                        aria-label="Custom answer"
                      />
                      <button
                        type="submit"
                        className="op-activity-panel-choice-custom-submit"
                        disabled={!trimmedOtherText || Boolean(pendingActionID)}
                      >
                        {otherBusy ? 'Sending...' : 'Send'}
                      </button>
                    </form>
                  ) : null}
                </li>
              </ol>
            </div>
          );
        })}
      </div>
      {message.body ? (
        <div className="op-activity-panel-open-request-details">
          <button
            type="button"
            className="op-activity-panel-open-request-details-toggle"
            onClick={() => setDetailsExpanded((current) => !current)}
            aria-expanded={detailsExpanded}
          >
            {detailsExpanded ? 'Hide details' : 'Show details'}
          </button>
          {detailsExpanded ? (
            <ActivityMarkdownView text={message.body} className="op-activity-panel-message-body" />
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function EntryMessageView({
  message,
  participant,
  showParticipantHeader,
  onAction,
  onAnswer,
  answeredRequest,
  pendingActionID,
  requestAnswerDetails,
  replyToRequest,
}: {
  message: EntryMessage;
  participant: ActivityMessageParticipant;
  showParticipantHeader: boolean;
  onAction?: (message: EntryMessage, actionID: string) => void;
  onAnswer?: (message: EntryMessage, answer: EntryMessageQuestionAnswer) => Promise<boolean> | boolean;
  answeredRequest?: boolean;
  pendingActionID?: string | null;
  requestAnswerDetails?: EntryMessageQuestionAnswer[];
  replyToRequest?: EntryMessage;
}) {
  const bodyWrapRef = useRef<HTMLDivElement | null>(null);
  const [bodyExpanded, setBodyExpanded] = useState(false);
  const [bodyCanCollapse, setBodyCanCollapse] = useState(false);
  const [requestExpanded, setRequestExpanded] = useState(false);
  const [openOtherQuestionID, setOpenOtherQuestionID] = useState<string | null>(null);
  const [otherAnswerByQuestionID, setOtherAnswerByQuestionID] = useState<Record<string, string>>({});

  useEffect(() => {
    setBodyExpanded(false);
    setBodyCanCollapse(false);
    setRequestExpanded(false);
  }, [message.id, message.body]);

  useEffect(() => {
    setOpenOtherQuestionID(null);
    setOtherAnswerByQuestionID({});
  }, [message.id]);

  useLayoutEffect(() => {
    if (bodyExpanded || !message.body) {
      return;
    }
    const body = bodyWrapRef.current;
    if (!body) {
      return;
    }
    const measure = () => {
      setBodyCanCollapse(body.scrollHeight > body.clientHeight + 1);
    };
    measure();
    if (typeof ResizeObserver === 'undefined') {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(body);
    return () => observer.disconnect();
  }, [bodyExpanded, message.body, message.id]);

  const shouldMeasureClamp = !bodyExpanded;
  const shouldShowCollapsedState = bodyCanCollapse && !bodyExpanded;
  const messageKey = message.recordID || message.id;
  const requestAnswered = Boolean(answeredRequest);
  const messageCanReply = message.status === 'open' && !requestAnswered;
  const showQuestionChoices = Boolean(message.questions?.length && messageCanReply);
  const showOpenRequestCard = message.bubbleRole === 'request' && showQuestionChoices;
  const isResolvedRequest = message.bubbleRole === 'request' && !messageCanReply;
  const resolvedAnswerSummary = summarizeRequestAnswers(requestAnswerDetails);
  const showStructuredAnswerCard = Boolean(
    message.bubbleRole === 'user'
    && message.replyToMessageID
    && message.answers?.length,
  );
  const questionTitle = (message.title || '').trim();
  const isRequestReadonlySurface = showStructuredAnswerCard || isResolvedRequest;
  const isRequestSurface = showOpenRequestCard || isRequestReadonlySurface;
  const questionAnswerPendingKey = (questionID: string, answerKey: string) => (
    `${messageKey}:${questionID}:${answerKey}`
  );
  const submitQuestionAnswer = async (answer: EntryMessageQuestionAnswer) => {
    if (!messageCanReply || !onAnswer || pendingActionID) {
      return;
    }
    const ok = await onAnswer(message, answer);
    if (ok && answer.other) {
      setOpenOtherQuestionID(null);
      setOtherAnswerByQuestionID((current) => ({
        ...current,
        [answer.questionID]: '',
      }));
    }
  };
  return (
    <ActivityParticipantFrame
      participant={participant}
      showParticipantHeader={showParticipantHeader}
      headerMessage={message}
      hideHeaderTitle={isRequestSurface}
    >
      <div className={[
        'op-activity-panel-message-bubble',
        `is-${message.bubbleRole}`,
        showOpenRequestCard ? 'is-open-request is-request-card' : '',
        isRequestReadonlySurface ? 'is-request-card-readonly' : '',
      ].filter(Boolean).join(' ')}>
          {showStructuredAnswerCard ? (
            <EntryRequestAnswerCard
              message={message}
              request={replyToRequest}
            />
          ) : null}
          {showOpenRequestCard ? (
            <EntryOpenRequestCard
              message={message}
              questionTitle={questionTitle}
              messageKey={messageKey}
              messageCanReply={messageCanReply}
              pendingActionID={pendingActionID}
              openOtherQuestionID={openOtherQuestionID}
              setOpenOtherQuestionID={setOpenOtherQuestionID}
              otherAnswerByQuestionID={otherAnswerByQuestionID}
              setOtherAnswerByQuestionID={setOtherAnswerByQuestionID}
              onAnswer={onAnswer}
            />
          ) : null}
          {!showStructuredAnswerCard && isResolvedRequest ? (
            <div className="op-activity-panel-request-resolved">
              <button
                type="button"
                className="op-activity-panel-request-resolved-summary"
                onClick={() => setRequestExpanded((current) => !current)}
                aria-expanded={requestExpanded}
              >
                <span className="op-activity-panel-request-resolved-title">
                  {questionTitle || messageLabel(message)}
                </span>
                {resolvedAnswerSummary ? (
                  <span className="op-activity-panel-request-resolved-answer">{resolvedAnswerSummary}</span>
                ) : null}
                <span className="op-activity-panel-request-resolved-status">Closed</span>
              </button>
              {requestExpanded ? (
                <div className="op-activity-panel-request-resolved-body">
                  {message.questions?.length ? (
                    <RequestQuestionsReadonly
                      questions={message.questions}
                      answers={requestAnswerDetails}
                    />
                  ) : null}
                  {message.body ? (
                    <ActivityMarkdownView text={message.body} className="op-activity-panel-message-body" />
                  ) : null}
                </div>
              ) : null}
            </div>
          ) : null}
          {!showStructuredAnswerCard && !isResolvedRequest && !showOpenRequestCard && message.body ? (
            <>
              <div
                ref={bodyWrapRef}
                className={[
                  'op-activity-panel-message-body-wrap',
                  shouldMeasureClamp ? 'is-collapse-measure' : '',
                  shouldShowCollapsedState ? 'is-collapsed' : '',
                ].filter(Boolean).join(' ')}
                style={{ '--op-activity-panel-message-collapse-lines': ACTIVITY_PANEL_MESSAGE_COLLAPSE_LINES } as React.CSSProperties}
              >
                <ActivityMarkdownView text={message.body} className="op-activity-panel-message-body" />
              </div>
              {bodyCanCollapse ? (
                <button
                  type="button"
                  className="op-activity-panel-message-toggle"
                  onClick={() => setBodyExpanded((current) => !current)}
                >
                  {bodyExpanded ? 'Show less' : 'Show more'}
                </button>
              ) : null}
            </>
          ) : null}
          {messageCanReply && message.actions?.length ? (
            <ol className="op-activity-panel-choice-list" aria-label="Message choices">
              {message.actions.map((action, actionIndex) => {
                const actionKey = `${message.recordID || message.id}:${action.id}`;
                const busy = pendingActionID === actionKey;
                const actionToneClass = action.tone === 'danger'
                  ? 'is-danger'
                  : action.tone === 'primary'
                    ? 'is-primary'
                    : '';
                const keycap = activityPanelActionKeycap(actionIndex);
                return (
                  <li key={action.id} className="op-activity-panel-choice-item">
                    <button
                      type="button"
                      className={['op-activity-panel-choice-button', actionToneClass].filter(Boolean).join(' ')}
                      disabled={Boolean(pendingActionID)}
                      onClick={() => {
                        onAction?.(message, action.id);
                      }}
                      aria-label={`${keycap}. ${action.label}`}
                    >
                      <span className="op-activity-panel-choice-key" aria-hidden="true">{keycap}</span>
                      <span className="op-activity-panel-choice-label">
                        {busy ? 'Sending...' : action.label}
                        {action.tone === 'primary' ? (
                          <span className="op-activity-panel-choice-badge">Recommended</span>
                        ) : null}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ol>
          ) : null}
          {showQuestionChoices && !showOpenRequestCard ? (
            <div className="op-activity-panel-question-list">
              {questionTitle ? (
                <div className="op-activity-panel-question-title">{questionTitle}</div>
              ) : null}
              {(message.questions || []).map((question) => {
                const questionID = (question.id || '').trim();
                const options = Array.isArray(question.options) ? question.options : [];
                const otherInputID = `op-activity-panel-other-${messageKey}-${questionID}`.replace(/[^a-zA-Z0-9_-]/g, '-');
                const otherText = otherAnswerByQuestionID[questionID] || '';
                const trimmedOtherText = otherText.trim();
                const otherPendingKey = questionAnswerPendingKey(questionID, 'other');
                const otherBusy = pendingActionID === otherPendingKey;
                return (
                  <div key={questionID || question.question} className="op-activity-panel-question">
                    <div className="op-activity-panel-question-text">{question.question}</div>
                    <ol className="op-activity-panel-choice-list" aria-label={question.question}>
                      {options.map((option, optionIndex) => {
                        const optionID = (option.id || '').trim();
                        const answerKey = optionID || option.label;
                        const pendingKey = questionAnswerPendingKey(questionID, answerKey);
                        const busy = pendingActionID === pendingKey;
                        const disabled = Boolean(pendingActionID) || !messageCanReply;
                        const keycap = activityPanelActionKeycap(optionIndex);
                        return (
                          <li key={optionID || option.label} className="op-activity-panel-choice-item">
                            <button
                              type="button"
                              className="op-activity-panel-choice-button"
                              disabled={disabled}
                              onClick={() => {
                                void submitQuestionAnswer({
                                  questionID,
                                  optionID,
                                  label: option.label,
                                });
                              }}
                              aria-label={`${keycap}. ${option.label}`}
                            >
                              <span className="op-activity-panel-choice-key" aria-hidden="true">{keycap}</span>
                              <span className="op-activity-panel-choice-label">
                                {busy ? 'Sending...' : option.label}
                              </span>
                            </button>
                          </li>
                        );
                      })}
                      <li className="op-activity-panel-choice-item">
                        <button
                          type="button"
                          className={[
                            'op-activity-panel-choice-button',
                            openOtherQuestionID === questionID ? 'is-custom-open' : '',
                          ].filter(Boolean).join(' ')}
                          disabled={Boolean(pendingActionID) || !messageCanReply}
                          onClick={() => setOpenOtherQuestionID(questionID)}
                          aria-label={`${activityPanelActionKeycap(options.length)}. Other`}
                          aria-expanded={openOtherQuestionID === questionID}
                          aria-controls={openOtherQuestionID === questionID ? otherInputID : undefined}
                        >
                          <span className="op-activity-panel-choice-key" aria-hidden="true">
                            {activityPanelActionKeycap(options.length)}
                          </span>
                          <span className="op-activity-panel-choice-label">
                            {otherBusy ? 'Sending...' : 'Other...'}
                          </span>
                        </button>
                        {messageCanReply && openOtherQuestionID === questionID ? (
                          <form
                            className="op-activity-panel-choice-custom-form"
                            onSubmit={(event) => {
                              event.preventDefault();
                              if (!trimmedOtherText) {
                                return;
                              }
                              void submitQuestionAnswer({
                                questionID,
                                other: true,
                                label: 'Other',
                                text: trimmedOtherText,
                              });
                            }}
                          >
                            <span className="op-activity-panel-choice-custom-spacer" aria-hidden="true" />
                            <input
                              id={otherInputID}
                              type="text"
                              value={otherText}
                              onChange={(event) => setOtherAnswerByQuestionID((current) => ({
                                ...current,
                                [questionID]: event.target.value,
                              }))}
                              onKeyDown={(event) => {
                                if (event.key === 'Escape') {
                                  event.preventDefault();
                                  setOpenOtherQuestionID(null);
                                }
                              }}
                              placeholder="Type your own answer"
                              disabled={Boolean(pendingActionID)}
                              className="op-activity-panel-choice-custom-input"
                              aria-label="Custom answer"
                            />
                            <button
                              type="submit"
                              className="op-activity-panel-choice-custom-submit"
                              disabled={!trimmedOtherText || Boolean(pendingActionID)}
                            >
                              {otherBusy ? 'Sending...' : 'Send'}
                            </button>
                          </form>
                        ) : null}
                      </li>
                    </ol>
                  </div>
                );
              })}
            </div>
          ) : null}
      </div>
    </ActivityParticipantFrame>
  );
}

function LiveStepRow({
  step,
  participant,
  showParticipantHeader = false,
}: {
  step: LiveStep;
  participant?: ActivityMessageParticipant | null;
  showParticipantHeader?: boolean;
}) {
  const detail = getLiveStepContent(step);
  const toolInput = step.type === 'toolcall'
    ? (step.toolCall?.rawArguments || step.detail || '').trim()
    : '';
  const toolOutput = step.type === 'toolcall'
    ? (step.toolOutput || '').trim()
    : '';
  const summary = getLiveStepSummary(step);
  const isToolcall = step.type === 'toolcall';
  const showSummary = Boolean(summary && summary !== step.label && (isToolcall || step.type === 'reasoning'));
  const canCollapse = (isToolcall || step.type === 'reasoning')
    && step.status !== 'running'
    && Boolean(detail || toolInput || toolOutput);
  const [collapsed, setCollapsed] = useState(canCollapse);

  useEffect(() => {
    if (isToolcall && step.status === 'running') {
      setCollapsed(false);
    } else if (canCollapse) {
      setCollapsed(true);
    }
  }, [isToolcall, step.status, canCollapse]);

  const showMarkdownDetail = step.type !== 'toolcall' && step.status !== 'running';
  const icon = step.type === 'notice'
    ? (step.status === 'done' ? '⤓' : step.status === 'error' ? '✗' : '⟳')
    : (step.status === 'done' ? '✓' : step.status === 'error' ? '✗' : '⟳');
  const statusColor =
    step.status === 'done' ? 'text-green-600' : step.status === 'error' ? 'text-accent' : 'text-secondary-text';

  const handleToggle = () => {
    if (canCollapse) setCollapsed((prev) => !prev);
  };

  const stepNode = (
    <div className={`op-activity-panel-step is-${step.type}`}>
      <div
        className={`flex items-start gap-2 text-secondary-text ${canCollapse ? 'op-activity-step-toggle' : ''}`}
        onClick={canCollapse ? handleToggle : undefined}
      >
        <span className={`shrink-0 ${statusColor}`}>{icon}</span>
        {canCollapse && (
          <span className={`op-activity-step-chevron ${collapsed ? '' : 'is-expanded'}`} aria-hidden="true">
            <ChevronRightIcon className="w-3 h-3" />
          </span>
        )}
        <span className="op-activity-panel-step-label">
          <span className="op-activity-panel-step-name">{step.label}</span>
          {showSummary ? (
            <span className="op-activity-step-summary">{summary}</span>
          ) : null}
        </span>
        <span className={`ml-auto shrink-0 ${statusColor}`}>
          {step.status}
        </span>
      </div>
      {!collapsed && isToolcall && (toolInput || toolOutput) ? (
        <div className="op-activity-panel-step-content">
          {toolInput ? (
            <div className="op-activity-panel-step-section">
              <div className="op-activity-panel-step-section-label">Input</div>
              <pre className="op-activity-panel-text op-activity-panel-step-detail">{toolInput}</pre>
            </div>
          ) : null}
          {toolOutput ? (
            <div className="op-activity-panel-step-section">
              <div className="op-activity-panel-step-section-label">Output</div>
              <pre className="op-activity-panel-text op-activity-panel-step-detail">{toolOutput}</pre>
            </div>
          ) : null}
        </div>
      ) : null}
      {!collapsed && !isToolcall && detail != null && (
        showMarkdownDetail
          ? <ActivityMarkdownView text={detail} className="op-activity-panel-step-detail op-activity-panel-step-content" />
          : (
            <pre className="op-activity-panel-text op-activity-panel-step-detail op-activity-panel-step-content">
              {detail}
            </pre>
          )
      )}
    </div>
  );

  if (!participant) {
    return stepNode;
  }

  return (
    <ActivityParticipantFrame
      participant={participant}
      showParticipantHeader={showParticipantHeader}
      headerMessage={{
        id: step.id,
        role: 'assistant',
        bubbleRole: 'assistant',
        body: '',
      }}
    >
      {stepNode}
    </ActivityParticipantFrame>
  );
}

function PlanTodoList({
  snapshot,
  loading,
  error,
}: {
  snapshot: PlanChecklistSnapshot | null;
  loading: boolean;
  error: string | null;
}) {
  return (
    <div className="mt-3 rounded-md border border-border/70 bg-editor-bg/60 px-3 py-3">
      <div className="flex items-center gap-2 text-sm">
        <span className="font-medium text-prime-text">Todo List</span>
        {snapshot && (
          <span className="ml-auto text-xs text-secondary-text">
            {snapshot.completedCount}/{snapshot.totalCount}
          </span>
        )}
      </div>
      {loading && !snapshot && <div className="mt-2 text-xs text-secondary-text">Loading plan...</div>}
      {error && <div className="mt-2 text-xs text-accent whitespace-pre-wrap">{error}</div>}
      {snapshot && snapshot.items.length === 0 && !error && (
        <div className="mt-2 text-xs text-secondary-text">No checklist items found in the bound plan.</div>
      )}
      {snapshot && snapshot.items.length > 0 && (
        <div className="mt-2 space-y-1.5">
          {snapshot.items.map((item) => (
            <div key={item.id} className="flex items-start gap-2 text-sm">
              <span className={item.checked ? 'text-green-600' : 'text-secondary-text'}>
                {item.checked ? '✓' : '○'}
              </span>
              <span className={item.checked ? 'text-secondary-text line-through' : 'text-prime-text'}>
                {item.text}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function PlanBuildCard({
  title,
  planPath,
  buildConfig,
  setBuildConfig,
  agentOptions,
  enabledModels,
  onExecute,
  onClearContextExecute,
  onThinkingChange,
  busy,
  canExecute,
  error,
}: {
  title: string;
  planPath: string;
  buildConfig: BuildConfigState;
  setBuildConfig: React.Dispatch<React.SetStateAction<BuildConfigState | null>>;
  agentOptions: BuildAgentOption[];
  enabledModels: ModelEntry[];
  onExecute: () => void;
  onClearContextExecute: () => void;
  onThinkingChange: (level: ThinkingLevel) => void;
  busy: boolean;
  canExecute: boolean;
  error: string | null;
}) {
  const selectedAgent = agentOptions.find((option) => option.id === buildConfig.agentID) || agentOptions[0] || null;
  const selectedModel = enabledModels.find((model) => model.key === (buildConfig.modelKey || '')) || null;
  const thinkingLevels = useMemo(
    () => buildConfig.modelKey ? getThinkingPickerLevels(selectedModel) : (['off'] as ThinkingLevel[]),
    [buildConfig.modelKey, selectedModel]
  );
  const effectiveThinkingLevel = useMemo(
    () => normalizeThinkingLevelForModel(selectedModel, buildConfig.thinkingLevel),
    [buildConfig.thinkingLevel, selectedModel]
  );
  const agentSelectOptions = useMemo(
    () => agentOptions.map((option) => ({
      value: option.id,
      label: formatAgentOptionLabel(option),
    })),
    [agentOptions]
  );
  const modelSelectOptions = useMemo(
    () => enabledModels.map((model) => buildModelSelectOption(model)),
    [enabledModels]
  );
  const thinkingSelectOptions = useMemo(
    () =>
      thinkingLevels.map((level) => ({
        value: level,
        label: formatThinkingLabel(level),
      })),
    [thinkingLevels]
  );

  return (
    <div className="mt-3 rounded-md border border-border/70 bg-editor-bg/60 px-3 py-3">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium text-prime-text truncate">{title || 'Plan'}</div>
          <div className="mt-1 text-xs text-secondary-text break-all">{planPath}</div>
        </div>
      </div>

      <div className="mt-3 grid gap-2 sm:grid-cols-3">
        <label className="text-xs text-secondary-text">
          <span className="mb-1 block">Agent</span>
          <SelectMenu
            ariaLabel="Plan agent"
            options={agentSelectOptions}
            value={buildConfig.agentID}
            onChange={(nextAgentID) => {
              const nextAgent = agentOptions.find((option) => option.id === nextAgentID) || null;
              setBuildConfig((current) => ({
                ...(current || buildConfig),
                agentID: nextAgentID,
              }));
            }}
            disabled={busy}
            triggerClassName="px-2 py-1.5 text-sm text-prime-text"
            menuClassName="w-full"
          />
        </label>

        <label className="text-xs text-secondary-text">
          <span className="mb-1 block">Model</span>
          <SelectMenu
            ariaLabel="Plan model"
            options={modelSelectOptions}
            value={buildConfig.modelKey || ''}
            onChange={(nextModelKey) => {
              setBuildConfig((current) => ({
                ...(current || buildConfig),
                modelKey: nextModelKey || null,
              }));
            }}
            disabled={busy}
            triggerClassName="px-2 py-1.5 text-sm text-prime-text"
            menuClassName="w-full"
          />
        </label>

        <label className="text-xs text-secondary-text">
          <span className="mb-1 block">Thinking</span>
          <SelectMenu
            ariaLabel="Plan thinking level"
            options={thinkingSelectOptions}
            value={effectiveThinkingLevel}
            onChange={(nextThinkingLevel) => {
              onThinkingChange(nextThinkingLevel as ThinkingLevel);
            }}
            disabled={busy || !buildConfig.modelKey}
            triggerClassName="px-2 py-1.5 text-sm text-prime-text"
            menuClassName="w-full"
          />
        </label>
      </div>

      {selectedAgent && (
        <div className="mt-2 text-[11px] text-secondary-text break-all">
          cwd: {selectedAgent.cwd}
        </div>
      )}
      {error && <div className="mt-2 text-xs text-accent whitespace-pre-wrap">{error}</div>}

      <div className="mt-3 flex flex-wrap gap-2">
        <button
          type="button"
          className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor px-3 py-1.5 text-xs disabled:opacity-50"
          onClick={onExecute}
          disabled={busy || !canExecute}
        >
          Execute
        </button>
        <button
          type="button"
          className="ui-pill-btn-secondary px-3 py-1.5 text-xs disabled:opacity-50"
          onClick={onClearContextExecute}
          disabled={busy || !canExecute}
        >
          Clear Context Execute
        </button>
      </div>
    </div>
  );
}

type ReviewHistoryPanelProps = {
  title: string;
  reviews: ThreadReviewState[];
  busyKey: string | null;
  error: string | null;
  onNavigateFile: (review: ThreadReviewState, file: ThreadReviewFile) => void;
  onApproveAll: (review: ThreadReviewState) => void;
  onRejectAll: (review: ThreadReviewState) => void;
  onRollbackTurn: (review: ThreadReviewState) => void;
  onApproveFile: (review: ThreadReviewState, path: string) => void;
  onRejectFile: (review: ThreadReviewState, path: string) => void;
  onRollbackFile: (review: ThreadReviewState, path: string) => void;
};

function ReviewStatusBadge({ status }: { status: string }) {
  if (status === 'approved' || status === 'resolved') {
    return (
      <span className="op-activity-review-status is-kept">
        <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="none">
          <circle cx="8" cy="8" r="7" stroke="currentColor" strokeWidth="1.5" fill="currentColor" fillOpacity="0.12" />
          <path d="M5 8.5l2 2 4-4.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" fill="none" />
        </svg>
        kept
      </span>
    );
  }
  const label = status === 'rejected'
    ? 'undone'
    : status === 'rolledBack'
      ? 'rolled back'
      : status === 'conflict'
        ? 'conflict'
        : status;
  return (
    <span className={`op-activity-review-status is-${status}`}>
      {label}
    </span>
  );
}

function ReviewActionButton({
  children,
  onClick,
  disabled,
  title,
  tone = 'neutral',
}: {
  children: React.ReactNode;
  onClick: () => void;
  disabled: boolean;
  title?: string;
  tone?: 'neutral' | 'keep' | 'undo';
}) {
  return (
    <button
      type="button"
      className={`op-activity-review-action is-${tone}`}
      onClick={onClick}
      disabled={disabled}
      title={title}
    >
      {children}
    </button>
  );
}

function ReviewHistoryPanel({
  title,
  reviews,
  busyKey,
  error,
  defaultCollapsed = false,
  onNavigateFile,
  onApproveAll,
  onRejectAll,
  onRollbackTurn,
  onApproveFile,
  onRejectFile,
  onRollbackFile,
}: ReviewHistoryPanelProps & { defaultCollapsed?: boolean }) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);
  return (
    <div className="op-activity-review-panel mt-3 p-2">
      <div
        className="flex items-center gap-2 text-sm op-activity-step-toggle"
        onClick={() => setCollapsed((prev) => !prev)}
      >
        <span className={`op-activity-step-chevron ${collapsed ? '' : 'is-expanded'}`} aria-hidden="true">
          <ChevronRightIcon className="w-3 h-3" />
        </span>
        <span className="font-medium text-prime-text">{title}</span>
        <span className="ml-auto text-xs text-secondary-text">{formatReviewHistoryMetric(reviews)}</span>
      </div>
      {!collapsed && error ? <div className="op-activity-review-error">{error}</div> : null}
      {!collapsed && (<div className="mt-2 space-y-2">
        {reviews.map((review) => {
          const hasTurnActions = review.status === 'pending' || review.canRollback;
          return (
            <div key={review.turnID} className="op-activity-review-file">
              <div className="op-activity-review-turn-row">
                <div className="min-w-0 flex-1 text-left">
                  <span className="block text-xs text-prime-text">{formatReviewTimestamp(review.createdAt)}</span>
                  <span className="mt-0.5 block text-[11px] text-secondary-text truncate">
                    {formatReviewSummary(review)}
                  </span>
                </div>
                {review.status !== 'pending' ? <ReviewStatusBadge status={review.status} /> : null}
                {hasTurnActions && (
                  <div className="op-activity-review-actions">
                    {review.status === 'pending' && (
                      <>
                        <ReviewActionButton
                          tone="keep"
                          onClick={() => onApproveAll(review)}
                          disabled={busyKey != null}
                        >
                          Keep all
                        </ReviewActionButton>
                        <ReviewActionButton
                          tone="undo"
                          onClick={() => onRejectAll(review)}
                          disabled={busyKey != null}
                        >
                          Undo all
                        </ReviewActionButton>
                      </>
                    )}
                    {review.canRollback && (
                      <ReviewActionButton
                        onClick={() => onRollbackTurn(review)}
                        disabled={busyKey != null}
                      >
                        Rollback
                      </ReviewActionButton>
                    )}
                  </div>
                )}
              </div>

              <div className="op-activity-review-file-list">
                {review.files.map((file) => {
                  const displayPath = splitReviewPath(file.path);
                  const mergeBadge = getReviewFileStateBadge(file);
                  const pendingFileActions = review.status === 'pending' && file.status === 'pending';
                  const fileMeta = [
                    displayPath.dir,
                    file.firstChangedLine ? `Line ${file.firstChangedLine}` : '',
                  ].filter(Boolean).join(' · ');
                  const hasFileActions = pendingFileActions
                    || (review.canRollback && file.status === 'approved');
                  const keepDisabled = busyKey != null || !canKeepReviewFile(file);
                  const undoDisabled = busyKey != null || !canUndoReviewFile(file);
                  const rollbackDisabled = busyKey != null || !canRollbackReviewFile(file);
                  const conflictTitle = file.conflictMessage || (
                    file.mergeState === 'conflicted'
                      ? 'This change was edited after the agent wrote it.'
                      : file.mergeState === 'missing'
                        ? 'The file is missing.'
                        : undefined
                  );
                  return (
                    <div key={`${review.turnID}:${file.path}`} className="op-activity-review-file-row">
                      <div className="flex min-w-0 flex-1 items-center gap-2">
                        <button
                          type="button"
                          className="op-activity-review-file-link group"
                          onClick={() => onNavigateFile(review, file)}
                          title={file.path}
                        >
                          <span className="block truncate text-sm text-link-text group-hover:text-prime-text">
                            {displayPath.name}
                          </span>
                          {fileMeta ? (
                            <span className="mt-0.5 block truncate text-[11px] text-secondary-text">
                              {fileMeta}
                            </span>
                          ) : null}
                        </button>
                        {mergeBadge ? <ReviewStatusBadge status={mergeBadge} /> : null}
                        {file.status !== 'pending' ? <ReviewStatusBadge status={file.status} /> : null}
                      </div>
                      {hasFileActions && (
                        <div className="op-activity-review-actions">
                          {pendingFileActions && (
                            <>
                              <ReviewActionButton
                                tone="keep"
                                onClick={() => onApproveFile(review, file.path)}
                                disabled={keepDisabled}
                                title={keepDisabled ? conflictTitle : undefined}
                              >
                                {getKeepReviewFileLabel(file)}
                              </ReviewActionButton>
                              <ReviewActionButton
                                tone="undo"
                                onClick={() => onRejectFile(review, file.path)}
                                disabled={undoDisabled}
                                title={undoDisabled ? conflictTitle : undefined}
                              >
                                Undo
                              </ReviewActionButton>
                            </>
                          )}
                          {review.canRollback && file.status === 'approved' && (
                            <ReviewActionButton
                              onClick={() => onRollbackFile(review, file.path)}
                              disabled={rollbackDisabled}
                              title={rollbackDisabled ? conflictTitle : undefined}
                            >
                              Rollback
                            </ReviewActionButton>
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          );
        })}
      </div>
      )}
    </div>
  );
}

export function ActivityPanel({
  bodyMaxHeight = DEFAULT_ACTIVITY_PANEL_BODY_MAX_HEIGHT,
  forceVisible = false,
  onTopLeftResizeStart,
  onTopRightResizeStart,
  onHeaderPointerDown,
  shouldToggleOnHeaderClick,
  horizontalDragging = false,
  activeResizeCorner = null,
}: {
  bodyMaxHeight?: number;
  forceVisible?: boolean;
  onTopLeftResizeStart?: (event: React.PointerEvent<HTMLDivElement>) => void;
  onTopRightResizeStart?: (event: React.PointerEvent<HTMLDivElement>) => void;
  onHeaderPointerDown?: (event: React.PointerEvent<HTMLDivElement>) => void;
  shouldToggleOnHeaderClick?: () => boolean;
  horizontalDragging?: boolean;
  activeResizeCorner?: 'left' | 'right' | null;
}) {
  const selectedConversationTarget = useChatWorkspaceStore((s) => s.selectedConversationTarget);
  const liveOverlay = useChatWorkspaceStore((s) => s.getLiveOverlayForTarget(s.selectedConversationTarget));
  const threadSnapshot = useChatWorkspaceStore((s) => s.getThreadSnapshotForTarget(s.selectedConversationTarget));
  const threadState = useChatWorkspaceStore((s) => s.getThreadStateForTarget(s.selectedConversationTarget));
  const streamingText = liveOverlay.streamingText;
  const streamingStartedAt = liveOverlay.streamingStartedAt;
  const streamingSegments = liveOverlay.streamingSegments;
  const errorMessage = (liveOverlay.errorMessage || '').trim();
  const errorCode = (liveOverlay.errorCode || '').trim();
  const autoRetryActive = liveOverlay.autoRetryActive;
  const autoRetryAttempt = liveOverlay.autoRetryAttempt;
  const autoRetryLimit = liveOverlay.autoRetryLimit;
  const autoRetryDelayMs = liveOverlay.autoRetryDelayMs;
  const autoRetryStartedAt = liveOverlay.autoRetryStartedAt;
  const autoRetryErrorMessage = (liveOverlay.autoRetryErrorMessage || '').trim();
  const reconnectAttempt = liveOverlay.reconnectAttempt;
  const reconnectLimit = liveOverlay.reconnectLimit;
  const reconnectingMessage = (liveOverlay.reconnectingMessage || '').trim();
  const steps = liveOverlay.steps;
  const expanded = liveOverlay.expanded;
  const loopUsage = mergeTokenUsage(threadSnapshot, liveOverlay.loopUsage);
  const userOverride = liveOverlay.userOverride;
  const threadTailStatus = threadState.tailStatus;
  const setExpanded = useChatWorkspaceStore((s) => s.setActivityExpanded);
  const queuedMessages = useChatWorkspaceStore((s) => s.getQueuedMessagesForTarget(s.selectedConversationTarget));
  const reviews = useChatWorkspaceStore((s) => s.getReviews(
    s.getTargetChatPath(s.selectedConversationTarget)
  ));
  const threadMeta = useChatWorkspaceStore((s) => s.getThreadMeta(
    s.getTargetChatPath(s.selectedConversationTarget)
  ));
  const planRevision = useChatWorkspaceStore((s) => s.getPlanRevision(
    s.getTargetChatPath(s.selectedConversationTarget)
  ));
  const setReviews = useChatWorkspaceStore((s) => s.setReviews);
  const setAwaitingUser = useChatWorkspaceStore((s) => s.setAwaitingUser);
  const inProgress = useChatWorkspaceStore((s) => s.isTargetInProgress(s.selectedConversationTarget));
  const awaitingUser = useChatWorkspaceStore((s) => (
    s.selectedConversationTarget?.kind === 'thread'
      ? s.getAwaitingUser(s.getTargetChatPath(s.selectedConversationTarget))
      : null
  ));
  const listThreadReviews = useAppStore((s) => s.listThreadReviews);
  const resolveThreadReview = useAppStore((s) => s.resolveThreadReview);
  const rollbackThreadReview = useAppStore((s) => s.rollbackThreadReview);
  const reloadOpenTabsByPaths = useAppStore((s) => s.reloadOpenTabsByPaths);
  const openFile = useAppStore((s) => s.openFile);
  const agentNodes = useAppStore((s) => s.agentNodes);
  const resolveAgentByID = useAppStore((s) => s.resolveAgentByID);
  const ensureAgentRecord = useAppStore((s) => s.ensureAgentRecord);
  const currentReviewOverlay = useAppStore((s) => s.currentReviewOverlay);
  const setCurrentReviewOverlay = useAppStore((s) => s.setCurrentReviewOverlay);
  const replyMessenger = useAppStore((s) => s.replyMessenger);
  const loadMessengerChannel = useAppStore((s) => s.loadMessengerChannel);
  const authLoggedIn = useAuthStore((s) => s.loggedIn);
  const authProfile = useAuthStore((s) => s.profile);
  const authEmail = useAuthStore((s) => s.email);
  const authUID = useAuthStore((s) => s.uid);
  const modelsConfig = useModelsStore((s) => s.config);
  const chatThinkingLevel = useUiStore((s) => s.chatThinkingLevel);
  const pushToast = useToastStore((s) => s.pushToast);
  const prevInProgressRef = useRef(inProgress);
  const prevActivityKeyRef = useRef<string>('');
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const wasExpandedRef = useRef(expanded);
  const shouldFollowBottomRef = useRef(true);
  const windowLoadingRef = useRef(false);
  const lastRevealedAwaitingRequestRef = useRef('');
  const pendingAwaitingRevealRef = useRef(false);
  const [reviewBusyKey, setReviewBusyKey] = useState<string | null>(null);
  const [reviewError, setReviewError] = useState<string | null>(null);
  const [highlightedQuestionOptionIndex, setHighlightedQuestionOptionIndex] = useState(0);
  const [buildConfig, setBuildConfig] = useState<BuildConfigState | null>(null);
  const [planActionBusy, setPlanActionBusy] = useState(false);
  const [planActionError, setPlanActionError] = useState<string | null>(null);
  const [windowLoading, setWindowLoading] = useState(false);
  const [windowError, setWindowError] = useState<string | null>(null);
  const [resolvedAgentRecord, setResolvedAgentRecord] = useState<{
    agentID: string;
    name: string | null;
    avatar: string | null;
  } | null>(null);
  const [pendingMessageActionID, setPendingMessageActionID] = useState<string | null>(null);
  const refreshMessageRequestState = async (message: EntryMessage) => {
    const threadID = (threadSnapshot?.meta.threadID || '').trim();
    const chatPath = (threadSnapshot?.meta.chatPath || threadSnapshot?.meta.path || '').trim();
    const modelKey = resolveDefaultChatModelSelection(modelsConfig).modelKey;
    await Promise.all([
      threadID || chatPath
        ? loadThreadSnapshotWindow({
          threadID,
          chatPath,
          modelKey,
          mode: 'tail',
          limit: 400,
        }).catch(() => null)
        : Promise.resolve(null),
      message.channelID ? loadMessengerChannel(message.channelID).catch(() => undefined) : Promise.resolve(undefined),
    ]);
  };
  const isRequestAlreadyClosedError = (error: unknown): boolean => (
    error instanceof Error && /request is not open/i.test(error.message)
  );
  const handleMessageAction = async (message: EntryMessage, actionID: string) => {
    const normalizedChannelID = (message.channelID || '').trim();
    const normalizedRecordID = (message.recordID || '').trim();
    const normalizedActionID = (actionID || '').trim();
    if (!normalizedChannelID || !normalizedRecordID || !normalizedActionID || pendingMessageActionID) {
      return;
    }
    const pendingID = `${normalizedRecordID}:${normalizedActionID}`;
    const action = message.actions?.find((item) => item.id === normalizedActionID);
    setPendingMessageActionID(pendingID);
    try {
      await replyMessenger({
        channelID: normalizedChannelID,
        replyToMessageID: normalizedRecordID,
        actionID: normalizedActionID,
        ...(action?.label ? { text: action.label } : {}),
      });
    } catch (error) {
      if (isRequestAlreadyClosedError(error)) {
        await refreshMessageRequestState(message);
      } else {
        pushToast(error instanceof Error ? error.message : 'Failed to send reply');
      }
    } finally {
      setPendingMessageActionID(null);
    }
  };
  const handleMessageQuestionAnswer = async (
    message: EntryMessage,
    answer: EntryMessageQuestionAnswer,
  ): Promise<boolean> => {
    const normalizedChannelID = (message.channelID || '').trim();
    const normalizedRecordID = (message.recordID || '').trim();
    const normalizedQuestionID = (answer.questionID || '').trim();
    const normalizedOptionID = (answer.optionID || '').trim();
    const normalizedText = (answer.text || '').trim();
    if (!normalizedChannelID || !normalizedRecordID || !normalizedQuestionID || pendingMessageActionID) {
      return false;
    }
    if (!normalizedOptionID && !answer.other && !normalizedText) {
      return false;
    }
    const pendingKey = `${normalizedRecordID}:${normalizedQuestionID}:${answer.other ? 'other' : (normalizedOptionID || answer.label || 'answer')}`;
    setPendingMessageActionID(pendingKey);
    try {
      await replyMessenger({
        channelID: normalizedChannelID,
        replyToMessageID: normalizedRecordID,
        answers: [{
          questionID: normalizedQuestionID,
          ...(normalizedOptionID ? { optionID: normalizedOptionID } : {}),
          ...(answer.label ? { label: answer.label } : {}),
          ...(answer.other ? { other: true } : {}),
          ...(normalizedText ? { text: normalizedText } : {}),
        }],
      });
      return true;
    } catch (error) {
      if (isRequestAlreadyClosedError(error)) {
        await refreshMessageRequestState(message);
      } else {
        pushToast(error instanceof Error ? error.message : 'Failed to send reply');
      }
      return false;
    } finally {
      setPendingMessageActionID(null);
    }
  };
  const [retryNow, setRetryNow] = useState(() => Date.now());
  const activityKey = useMemo(() => {
    if (!selectedConversationTarget) {
      return '';
    }
    if (selectedConversationTarget.kind === 'thread') {
      return selectedConversationTarget.threadID;
    }
    if (selectedConversationTarget.kind === 'command') {
      return selectedConversationTarget.path;
    }
    return `pending:${selectedConversationTarget.id}`;
  }, [selectedConversationTarget]);
  const reviewChatPath = useChatWorkspaceStore((s) => (
    s.selectedConversationTarget?.kind === 'thread'
      ? s.getTargetChatPath(s.selectedConversationTarget)
      : null
  ));
  const agentOptions = useMemo(() => buildAgentOptions(agentNodes), [agentNodes]);
  const enabledModels = useMemo(() => modelsConfig.models.filter((model) => model.enabled), [modelsConfig.models]);
  const defaultChatModelKey = useMemo(
    () => resolveDefaultChatModelSelection(modelsConfig).modelKey,
    [modelsConfig],
  );
  const { planPath, snapshot: planSnapshot, loading: planLoading, error: planLoadError } = usePlanChecklistSnapshot(
    reviewChatPath,
    threadMeta,
    planRevision,
  );
  const hasValidPlanSnapshot = Boolean(planSnapshot && planSnapshot.totalCount > 0);
  const canExecutePlan = hasValidPlanSnapshot && !planLoadError;
  const showPlanCard = Boolean(
    reviewChatPath
    && !inProgress
    && threadMeta?.planPath
    && !threadMeta?.executionPlanPath
    && !errorMessage
  );
  const showTodoList = Boolean(reviewChatPath && threadMeta?.executionPlanPath);
  const hasContextUsage = Boolean(
    loopUsage.contextWindow
    && (
      loopUsage.contextKnown === false
      || (loopUsage.contextTokens || 0) > 0
      || typeof loopUsage.contextPercent === 'number'
    )
  );
  const threadEntries = useMemo(
    () => (Array.isArray(threadSnapshot?.entries) ? threadSnapshot.entries : []),
    [threadSnapshot?.revision, threadSnapshot?.entries],
  );
  const entryWindow = threadSnapshot?.entryWindow || null;
  const firstEntryId = useMemo(() => {
    for (const entry of threadEntries) {
      const id = (entry.id || '').trim();
      if (id) {
        return id;
      }
    }
    return '';
  }, [threadEntries]);
  const canLoadOlderWindow = Boolean(entryWindow?.hasBefore && firstEntryId);
  const isHistoricalWindow = Boolean(entryWindow?.hasAfter);
  const durableTimelineItems = useMemo(() => {
    const entryItems = threadEntryTimelineItems(threadEntries);
    return [
      ...entryItems,
      ...threadMessageRecordTimelineItems(threadSnapshot, entryItems.length * 100),
    ];
  }, [threadEntries, threadSnapshot, threadSnapshot?.messageRecords]);
  const answeredRequestByID = useMemo(
    () => buildAnsweredRequestMap(durableTimelineItems),
    [durableTimelineItems],
  );
  const messageRecordByID = useMemo(
    () => buildMessageRecordByID(durableTimelineItems),
    [durableTimelineItems],
  );
  const requestAnswerDetailsByID = useMemo(
    () => buildRequestAnswerDetailsByID(durableTimelineItems),
    [durableTimelineItems],
  );
  const activeAgentID = (threadSnapshot?.meta.agentID || threadMeta?.agentID || '').trim();

  useEffect(() => {
    let cancelled = false;
    setResolvedAgentRecord(null);
    if (!activeAgentID) {
      return () => {
        cancelled = true;
      };
    }
    void ensureAgentRecord(activeAgentID)
      .then((record) => {
        if (cancelled) {
          return;
        }
        const meta = asRecord(record?.meta);
        const name = asString(meta?.name);
        const avatar = asString(meta?.avatar);
        setResolvedAgentRecord({
          agentID: activeAgentID,
          name: name || null,
          avatar: avatar || null,
        });
      })
      .catch(() => {
        if (!cancelled) {
          setResolvedAgentRecord(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [activeAgentID, ensureAgentRecord]);

  const activityMessageParticipants = useMemo<ActivityMessageParticipants>(() => {
    const userName = authLoggedIn
      ? resolveActivityDisplayName(authProfile, authEmail, authUID)
      : '';
    const indexedAgent = activeAgentID ? resolveAgentByID(activeAgentID) : null;
    const recordAgent = resolvedAgentRecord?.agentID === activeAgentID ? resolvedAgentRecord : null;
    const agentName = (recordAgent?.name || indexedAgent?.name || activeAgentID || 'Agent').trim();
    return {
      user: {
        kind: 'user',
        name: userName || 'User',
        avatarSrc: authLoggedIn ? resolveActivityParticipantAvatarSrc(authProfile) : null,
        fallbackText: userName || authEmail || authUID || 'User',
      },
      assistant: {
        kind: 'assistant',
        name: agentName,
        avatarSrc: (recordAgent?.avatar || indexedAgent?.avatar || '').trim() || null,
        fallbackText: agentName,
      },
    };
  }, [
    activeAgentID,
    authEmail,
    authLoggedIn,
    authProfile,
    authUID,
    resolveAgentByID,
    resolvedAgentRecord,
  ]);
  const retainedAnswerText = useMemo(
    () => getLatestStreamingSegmentText(streamingSegments) || latestTimelineText(durableTimelineItems),
    [durableTimelineItems, streamingSegments]
  );
  const hasContent = Boolean(
    durableTimelineItems.length > 0
    || streamingText
    || streamingSegments.length > 0
    || errorMessage
    || steps.length > 0
    || awaitingUser
    || hasContextUsage
  );
  const pendingReviews = useMemo(() => reviews.filter(isPendingReview), [reviews]);
  const completedReviews = useMemo(() => reviews.filter((review) => !isPendingReview(review)), [reviews]);
  const hasReviews = reviews.length > 0;
  const questionOptionItems = useMemo(
    () => (awaitingUser ? buildAwaitingQuestionOptionItems(awaitingUser) : []),
    [awaitingUser],
  );
  const queuedMessageCount = queuedMessages.steering.length + queuedMessages.followUp.length;
  const activityErrorInfo = useMemo(
    () => resolveActivityErrorInfo(errorMessage, errorCode),
    [errorCode, errorMessage],
  );
  const headerViewModel = buildActivityHeaderViewModel({
    steps,
    streamingText,
    retainedAnswerText,
    inProgress,
    errorMessage,
    errorCode,
    autoRetryActive,
    autoRetryAttempt,
    autoRetryLimit,
    autoRetryErrorMessage,
    reconnectAttempt,
    reconnectLimit,
    reconnectingMessage,
    awaitingUser,
    planErrorMessage: (showPlanCard || showTodoList) ? planLoadError : null,
    tokenUsage: loopUsage,
    threadTailStatus,
    queuedMessageCount,
    reviewCount: reviews.length,
    showPlanCard,
    showTodoList,
    planSnapshot: planSnapshot
      ? {
        totalCount: planSnapshot.totalCount,
        completedCount: planSnapshot.completedCount,
      }
      : null,
  });
  const showLoadingAnimation = (
    headerViewModel.tone === 'running'
    && !headerViewModel.preview
    && !awaitingUser
  );
  const timelineStreamingSegments = streamingSegments.length > 0
    ? streamingSegments
    : (
      streamingText && streamingStartedAt != null
        ? [{ id: 'stream', text: streamingText, ts: streamingStartedAt, order: 0 }]
        : []
    );
  const liveTimelineSteps = isHistoricalWindow ? [] : steps;
  const liveTimelineStreamingSegments = isHistoricalWindow ? [] : timelineStreamingSegments;
  const timelineItems: ActivityTimelineItem[] = [
    ...durableTimelineItems,
    ...liveTimelineSteps.map((step) => ({ kind: 'step' as const, ts: step.ts, order: durableTimelineItems.length * 100 + (step.order ?? 0), step })),
    ...liveTimelineStreamingSegments.map((segment) => ({
      kind: 'stream' as const,
      ts: segment.ts,
      order: durableTimelineItems.length * 100 + segment.order,
      segment,
    })),
  ].sort((a, b) => {
    if (a.order != null && b.order != null && a.order !== b.order) {
      return a.order - b.order;
    }
    if (a.ts !== b.ts) {
      return a.ts - b.ts;
    }
    if (a.kind === b.kind) {
      return 0;
    }
    return getActivityTimelineKindRank(a.kind) - getActivityTimelineKindRank(b.kind);
  });
  const timelineRenderItems = buildActivityTimelineRenderItems(timelineItems);
  const activeThreadID = (
    threadSnapshot?.meta.threadID
    || (selectedConversationTarget?.kind === 'thread' ? selectedConversationTarget.threadID : '')
  ).trim();
  const activeThreadLinkTarget = buildThreadLinkTarget(activeThreadID);
  const activeChatPath = (
    threadSnapshot?.meta.chatPath
    || reviewChatPath
    || (selectedConversationTarget?.kind === 'command' ? selectedConversationTarget.path : '')
  ).trim();

  const handleActivityThreadIDClick = useCallback((event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    if (!activeThreadLinkTarget) {
      return;
    }
    void navigateFrontmatterLink(activeThreadLinkTarget);
  }, [activeThreadLinkTarget]);

  const threadMetadataViewModel = useMemo(
    () => buildActivityThreadMetadataViewModel({ threadID: activeThreadID, chatPath: activeChatPath }),
    [activeThreadID, activeChatPath],
  );

  const handleActivityChatFileClick = useCallback((event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    const chatPath = (activeChatPath || '').trim();
    if (!chatPath) {
      return;
    }
    void openFile(chatPath).catch((error) => {
      useToastStore.getState().pushToast(
        error instanceof Error ? error.message : 'Failed to open chat file',
      );
    });
  }, [activeChatPath, openFile]);

  const loadOlderWindow = useCallback(async () => {
    const body = bodyRef.current;
    if (!body || !canLoadOlderWindow || !firstEntryId || windowLoadingRef.current) {
      return;
    }
    const previousScrollHeight = body.scrollHeight;
    const previousScrollTop = body.scrollTop;
    windowLoadingRef.current = true;
    setWindowLoading(true);
    setWindowError(null);
    try {
      await loadThreadSnapshotWindow({
        threadID: activeThreadID,
        chatPath: activeChatPath,
        mode: 'before',
        anchorId: firstEntryId,
        limit: 200,
      });
      window.requestAnimationFrame(() => {
        const currentBody = bodyRef.current;
        if (!currentBody) {
          return;
        }
        const delta = currentBody.scrollHeight - previousScrollHeight;
        currentBody.scrollTop = previousScrollTop + delta;
        shouldFollowBottomRef.current = false;
      });
    } catch (error) {
      setWindowError(error instanceof Error ? error.message : 'Failed to load earlier messages.');
    } finally {
      windowLoadingRef.current = false;
      setWindowLoading(false);
    }
  }, [activeChatPath, activeThreadID, canLoadOlderWindow, firstEntryId]);

  const jumpToLatestWindow = useCallback(async () => {
    if (windowLoadingRef.current || (!activeThreadID && !activeChatPath)) {
      return;
    }
    windowLoadingRef.current = true;
    setWindowLoading(true);
    setWindowError(null);
    try {
      await loadThreadSnapshotWindow({
        threadID: activeThreadID,
        chatPath: activeChatPath,
        mode: 'tail',
        limit: 400,
      });
      window.requestAnimationFrame(() => {
        const body = bodyRef.current;
        if (!body) {
          return;
        }
        body.scrollTop = body.scrollHeight;
        shouldFollowBottomRef.current = true;
      });
    } catch (error) {
      pushToast(error instanceof Error ? error.message : 'Failed to load latest messages.');
    } finally {
      windowLoadingRef.current = false;
      setWindowLoading(false);
    }
  }, [activeChatPath, activeThreadID, pushToast]);

  useEffect(() => {
    if (!autoRetryActive) {
      return;
    }
    const timer = setInterval(() => {
      setRetryNow(Date.now());
    }, 250);
    return () => {
      clearInterval(timer);
    };
  }, [autoRetryActive]);

  useEffect(() => {
    if (!reviewChatPath) {
      setBuildConfig(null);
      setPlanActionError(null);
      return;
    }
    const threadAgentID = (threadMeta?.agentID || '').trim();
    const fallbackAgentID = threadAgentID || agentOptions[0]?.id || '';
    setBuildConfig({
      agentID: fallbackAgentID,
      modelKey: defaultChatModelKey,
      thinkingLevel: chatThinkingLevel,
    });
    setPlanActionError(null);
  }, [agentOptions, chatThinkingLevel, defaultChatModelKey, reviewChatPath, threadMeta?.agentID]);

  useEffect(() => {
    if (!defaultChatModelKey) {
      return;
    }
    setBuildConfig((current) => (
      current && !current.modelKey ? { ...current, modelKey: defaultChatModelKey } : current
    ));
  }, [defaultChatModelKey]);

  useEffect(() => {
    setBuildConfig((current) => (
      current ? { ...current, thinkingLevel: chatThinkingLevel } : current
    ));
  }, [chatThinkingLevel]);

  useEffect(() => {
    const wasInProgress = prevInProgressRef.current;
    prevInProgressRef.current = inProgress;

    // Run finished.
    if (wasInProgress && !inProgress && activityKey && hasContent && !showPlanCard && !showTodoList) {
      const latestStep = steps[steps.length - 1];
      if (shouldKeepActivityPanelExpandedAfterRun(latestStep, activityErrorInfo)) {
        if (userOverride !== 'collapsed') {
          setExpanded(activityKey, true);
        }
        return;
      }
    }
  }, [activityErrorInfo, activityKey, hasContent, inProgress, setExpanded, showPlanCard, showTodoList, steps, userOverride]);

  useEffect(() => {
    if (prevActivityKeyRef.current === activityKey) {
      return;
    }
    prevActivityKeyRef.current = activityKey;
    setReviewError(null);
    setWindowError(null);
    setWindowLoading(false);
    windowLoadingRef.current = false;
    setHighlightedQuestionOptionIndex(0);
  }, [activityKey]);

  useEffect(() => {
    setHighlightedQuestionOptionIndex(0);
  }, [awaitingUser?.requestID, awaitingUser?.currentIndex]);

  useEffect(() => {
    const reviewThreadID = (threadMeta?.threadID || '').trim();
    if (!reviewChatPath || !reviewThreadID) {
      return;
    }
    void listThreadReviews(reviewThreadID)
      .then((nextReviews) => {
        setReviews(reviewChatPath, nextReviews);
      })
      .catch(() => {
        // Ignore best-effort review hydration failures.
      });
  }, [listThreadReviews, reviewChatPath, threadMeta?.threadID, setReviews]);

  useEffect(() => {
    if (!reviewChatPath || !currentReviewOverlay) {
      return;
    }
    const overlayFile = findReviewFileForOverlay(reviews, currentReviewOverlay);
    if (overlayFile && canShowReviewOverlay(overlayFile)) {
      return;
    }
    setCurrentReviewOverlay(null);
  }, [currentReviewOverlay, reviewChatPath, reviews, setCurrentReviewOverlay]);

  useEffect(() => {
    if (!activityKey || (!showPlanCard && !showTodoList) || userOverride === 'collapsed') {
      return;
    }
    setExpanded(activityKey, true);
  }, [activityKey, setExpanded, showPlanCard, showTodoList, userOverride]);

  useLayoutEffect(() => {
    const awaitingRequestKey = activityKey && awaitingUser?.requestID
      ? `${activityKey}:${awaitingUser.requestID}`
      : '';
    if (!awaitingRequestKey) {
      lastRevealedAwaitingRequestRef.current = '';
      pendingAwaitingRevealRef.current = false;
      return;
    }
    if (lastRevealedAwaitingRequestRef.current === awaitingRequestKey) {
      return;
    }
    lastRevealedAwaitingRequestRef.current = awaitingRequestKey;
    pendingAwaitingRevealRef.current = true;
    setExpanded(activityKey, true);
  }, [activityKey, awaitingUser?.requestID, setExpanded]);

  useEffect(() => {
    if (!activityKey || pendingReviews.length === 0 || userOverride === 'collapsed') {
      return;
    }
    setExpanded(activityKey, true);
  }, [activityKey, pendingReviews.length, setExpanded, userOverride]);

  useEffect(() => {
    if (!expanded) {
      shouldFollowBottomRef.current = true;
      return;
    }
    const body = bodyRef.current;
    if (!body) {
      return;
    }
    const updateFollowBottom = (allowLoadOlder: boolean) => {
      shouldFollowBottomRef.current = isActivityPanelNearBottom(body);
      if (allowLoadOlder && body.scrollTop <= ACTIVITY_PANEL_LOAD_OLDER_THRESHOLD_PX) {
        void loadOlderWindow();
      }
    };
    updateFollowBottom(false);
    const handleScroll = () => updateFollowBottom(true);
    body.addEventListener('scroll', handleScroll, { passive: true });
    return () => body.removeEventListener('scroll', handleScroll);
  }, [expanded, loadOlderWindow]);

  useLayoutEffect(() => {
    if (!awaitingUser || !expanded || !pendingAwaitingRevealRef.current) {
      return;
    }
    const body = bodyRef.current;
    if (!body) {
      return;
    }
    const revealAwaitingQuestion = () => {
      body.scrollTop = 0;
      shouldFollowBottomRef.current = false;
    };
    revealAwaitingQuestion();
    const frame = window.requestAnimationFrame(() => {
      revealAwaitingQuestion();
      pendingAwaitingRevealRef.current = false;
    });
    return () => window.cancelAnimationFrame(frame);
  }, [awaitingUser?.requestID, bodyMaxHeight, expanded]);

  useLayoutEffect(() => {
    const body = bodyRef.current;
    const wasExpanded = wasExpandedRef.current;
    wasExpandedRef.current = expanded;
    if (!body || !expanded) {
      return;
    }
    if (awaitingUser) {
      return;
    }
    if (isHistoricalWindow) {
      return;
    }
    const hasLiveStreamingOutput = inProgress || Boolean(streamingText);
    if (!hasLiveStreamingOutput) {
      return;
    }
    const justExpanded = !wasExpanded;
    if (!justExpanded && !shouldFollowBottomRef.current) {
      return;
    }
    const scrollToBottom = () => {
      body.scrollTop = body.scrollHeight;
      shouldFollowBottomRef.current = true;
    };
    scrollToBottom();
    const frame = window.requestAnimationFrame(scrollToBottom);
    return () => window.cancelAnimationFrame(frame);
  }, [
    awaitingUser?.currentIndex,
    awaitingUser?.requestID,
    bodyMaxHeight,
    completedReviews.length,
    errorMessage,
    expanded,
    inProgress,
    isHistoricalWindow,
    pendingReviews.length,
    showPlanCard,
    showTodoList,
    steps.length,
    streamingText,
    threadSnapshot?.revision,
  ]);

  useEffect(() => {
    if (!awaitingUser || !expanded) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!awaitingUser) {
        return;
      }
      if (isEditableQuestionTarget(event.target)) {
        if (event.key.toLowerCase() === 'escape') {
          event.preventDefault();
          handleAwaitingUserCancel();
          return;
        }
        if (event.key.toLowerCase() !== 'enter') {
          return;
        }
        const action = resolveAwaitingUserKeyboardAction({
          awaitingUser,
          highlightedIndex: highlightedQuestionOptionIndex,
          key: event.key,
          metaKey: event.metaKey,
          ctrlKey: event.ctrlKey,
          altKey: event.altKey,
        });
        if (action?.kind === 'continue') {
          event.preventDefault();
          handleQuestionContinue();
        }
        return;
      }
      const action = resolveAwaitingUserKeyboardAction({
        awaitingUser,
        highlightedIndex: highlightedQuestionOptionIndex,
        key: event.key,
        metaKey: event.metaKey,
        ctrlKey: event.ctrlKey,
        altKey: event.altKey,
      });
      if (!action) {
        return;
      }
      event.preventDefault();
      if (action.kind === 'move') {
        if (questionOptionItems.length === 0) {
          return;
        }
        setHighlightedQuestionOptionIndex((current) => {
          const next = current + action.direction;
          if (next < 0) {
            return questionOptionItems.length - 1;
          }
          if (next >= questionOptionItems.length) {
            return 0;
          }
          return next;
        });
        return;
      }
      if (action.kind === 'navigate') {
        updateAwaitingDraft(navigateAwaitingUserQuestion(awaitingUser, action.direction));
        return;
      }
      if (action.kind === 'selectHighlighted') {
        if (questionOptionItems.length === 0) {
          return;
        }
        handleQuestionOptionSelect(highlightedQuestionOptionIndex);
        return;
      }
      if (action.kind === 'continue') {
        handleQuestionContinue();
        return;
      }
      handleAwaitingUserCancel();
    };
    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [expanded, awaitingUser, highlightedQuestionOptionIndex, questionOptionItems.length]);

  const showThreadActivityEmptyState = forceVisible
    && !hasContent
    && !inProgress
    && !hasReviews
    && !showPlanCard
    && !showTodoList;

  if (!activityKey) return null;
  if (!hasContent && !inProgress && !hasReviews && !showPlanCard && !showTodoList && !forceVisible) return null;

  const refreshReviewHistory = async () => {
    const reviewThreadID = (threadMeta?.threadID || '').trim();
    if (!reviewChatPath || !reviewThreadID) {
      return [];
    }
    const nextReviews = await listThreadReviews(reviewThreadID);
    setReviews(reviewChatPath, nextReviews);
    return nextReviews;
  };

  const applyReviewUpdate = async (affectedPaths: string[]) => {
    await refreshReviewHistory();
    if (affectedPaths.length > 0) {
      await reloadOpenTabsByPaths(affectedPaths, { skipDirty: true });
    }
  };

  const runReviewAction = async (busyKey: string, task: () => Promise<ThreadReviewState | null>) => {
    if (!reviewChatPath) {
      return;
    }
    setReviewBusyKey(busyKey);
    setReviewError(null);
    try {
      const nextReview = await task();
      const affectedPaths = Array.from(new Set((nextReview?.files || []).map((file) => file.path)));
      await applyReviewUpdate(affectedPaths);
    } catch (error) {
      setReviewError(formatReviewActionError(error));
    } finally {
      setReviewBusyKey(null);
    }
  };

  const handleNavigateReviewFile = (review: ThreadReviewState, file: ThreadReviewFile) => {
    const reviewOverlay = canShowReviewOverlay(file)
      ? {
          filePath: file.path,
          threadID: review.threadID,
          turnID: review.turnID,
          chatPath: review.chatPath,
          changedRanges: file.changedRanges || [],
          hunks: file.hunks || [],
        }
      : null;
    void openFile(file.path, {
      ...(file.firstChangedLine && file.firstChangedLine > 0
        ? { reveal: { line: file.firstChangedLine, column: file.firstChangedColumn } }
        : {}),
      reviewOverlay,
      focusEditor: true,
    });
  };

  const reviewPanelActionProps = {
    onNavigateFile: handleNavigateReviewFile,
    onApproveAll: (review: ThreadReviewState) => runReviewAction(`approve-all:${review.turnID}`, () => resolveThreadReview({
      threadID: review.threadID,
      turnID: review.turnID,
      decision: 'approveAll',
    })),
    onRejectAll: (review: ThreadReviewState) => runReviewAction(`reject-all:${review.turnID}`, () => resolveThreadReview({
      threadID: review.threadID,
      turnID: review.turnID,
      decision: 'rejectAll',
    })),
    onRollbackTurn: (review: ThreadReviewState) => runReviewAction(`rollback-turn:${review.turnID}`, () => rollbackThreadReview({
      threadID: review.threadID,
      turnID: review.turnID,
      scope: 'turn',
    })),
    onApproveFile: (review: ThreadReviewState, path: string) => runReviewAction(`approve:${review.turnID}:${path}`, () => resolveThreadReview({
      threadID: review.threadID,
      turnID: review.turnID,
      decision: 'approve',
      path,
    })),
    onRejectFile: (review: ThreadReviewState, path: string) => runReviewAction(`reject:${review.turnID}:${path}`, () => resolveThreadReview({
      threadID: review.threadID,
      turnID: review.turnID,
      decision: 'reject',
      path,
    })),
    onRollbackFile: (review: ThreadReviewState, path: string) => runReviewAction(`rollback:${review.turnID}:${path}`, () => rollbackThreadReview({
      threadID: review.threadID,
      turnID: review.turnID,
      scope: 'file',
      path,
    })),
  };

  const handleGlobalThinkingChange = (level: ThinkingLevel) => {
    setBuildConfig((current) => (
      current ? { ...current, thinkingLevel: level } : current
    ));
    void persistGlobalThinkingLevel(level).catch((error) => {
      pushToast(error instanceof Error ? error.message : '保存 thinking 失败');
    });
  };

  const runPlanAction = async (mode: 'execute' | 'clear-context') => {
    if (!reviewChatPath || !buildConfig) {
      return;
    }
    const selectedAgent = agentOptions.find((option) => option.id === buildConfig.agentID) || null;
    if (!selectedAgent) {
      setPlanActionError('Select an agent first.');
      return;
    }
    if (!planPath) {
      setPlanActionError('Plan file is unavailable.');
      return;
    }
    if (!planSnapshot || planLoadError) {
      setPlanActionError(planLoadError || 'Plan file is invalid.');
      return;
    }
    setPlanActionBusy(true);
    setPlanActionError(null);
    try {
      const request: PlanBuildRequest = {
        chatPath: reviewChatPath,
        planPath,
        planTitle: planSnapshot?.title || threadMeta?.title || 'Plan',
        agentID: selectedAgent.id,
        agentName: selectedAgent.name,
        agentCwd: selectedAgent.cwd,
        modelKey: buildConfig.modelKey,
        thinkingLevel: buildConfig.thinkingLevel,
      };
      if (mode === 'execute') {
        await executePlanInCurrentThread(request);
      } else {
        await clearContextExecutePlan(request);
      }
    } catch (error) {
      setPlanActionError((error as Error)?.message || 'Plan execution failed');
    } finally {
      setPlanActionBusy(false);
    }
  };

  const handleChevronClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    setExpanded(activityKey, !expanded, { userAction: true });
  };

  const handleHeaderClick = () => {
    if (shouldToggleOnHeaderClick && !shouldToggleOnHeaderClick()) {
      return;
    }
    setExpanded(activityKey, !expanded, { userAction: true });
  };

  const handleTopLeftResizePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.stopPropagation();
    onTopLeftResizeStart?.(event);
  };

  const handleTopRightResizePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.stopPropagation();
    onTopRightResizeStart?.(event);
  };

  const updateAwaitingDraft = (nextAwaiting: AwaitingUserState) => {
    if (!reviewChatPath) {
      return;
    }
    setAwaitingUser(reviewChatPath, nextAwaiting);
  };

  const handleQuestionOptionSelect = (optionIndex: number) => {
    if (!awaitingUser) {
      return;
    }
    updateAwaitingDraft(selectAwaitingUserOption(awaitingUser, optionIndex));
    setHighlightedQuestionOptionIndex(optionIndex);
  };

  const handleQuestionCustomAnswerChange = (value: string) => {
    if (!awaitingUser) {
      return;
    }
    updateAwaitingDraft(updateAwaitingUserCustomAnswer(awaitingUser, value));
  };

  const handleQuestionSkip = () => {
    if (!awaitingUser) {
      return;
    }
    const skipped = skipAwaitingUserQuestion(awaitingUser);
    if (isLastAwaitingUserQuestion(skipped)) {
      updateAwaitingDraft(skipped);
      return;
    }
    updateAwaitingDraft(advanceAwaitingUserQuestion(skipped));
  };

  const handleAwaitingUserCancel = () => {
    if (reviewChatPath) {
      setAwaitingUser(reviewChatPath, null);
    }
  };

  const handleQuestionNavigate = (direction: -1 | 1) => {
    if (!awaitingUser) {
      return;
    }
    updateAwaitingDraft(navigateAwaitingUserQuestion(awaitingUser, direction));
  };

  const handleQuestionContinue = () => {
    if (!awaitingUser || !canContinueAwaitingUser(awaitingUser)) {
      return;
    }
    if (isLastAwaitingUserQuestion(awaitingUser)) {
      updateAwaitingDraft(awaitingUser);
      return;
    }
    updateAwaitingDraft(advanceAwaitingUserQuestion(awaitingUser));
  };

  return (
    <div className={`op-activity-panel ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
      {expanded && onTopLeftResizeStart && (
        <div
          className={`op-activity-panel-corner-handle is-left ${activeResizeCorner === 'left' ? 'is-active' : ''}`}
          onPointerDown={handleTopLeftResizePointerDown}
        >
          <span className="op-activity-panel-corner-visual" aria-hidden="true" />
        </div>
      )}
      {expanded && onTopRightResizeStart && (
        <div
          className={`op-activity-panel-corner-handle is-right ${activeResizeCorner === 'right' ? 'is-active' : ''}`}
          onPointerDown={handleTopRightResizePointerDown}
        >
          <span className="op-activity-panel-corner-visual" aria-hidden="true" />
        </div>
      )}
      <div className={`op-activity-panel-header ${expanded ? 'is-expanded' : ''}`}>
        <div
          className={`op-activity-panel-statusbar ${onHeaderPointerDown ? 'is-draggable' : ''} ${horizontalDragging ? 'is-dragging' : ''}`}
          onPointerDown={onHeaderPointerDown}
          onClick={handleHeaderClick}
        >
          <div className="op-activity-panel-statusbar-inner">
            <div className="op-activity-panel-statusbar-main">
              <button
                type="button"
                className="op-activity-panel-statusbar-chevron-hit"
                onPointerDownCapture={(event) => event.stopPropagation()}
                onClick={handleChevronClick}
                aria-label={expanded ? 'Collapse activity panel' : 'Expand activity panel'}
              >
                <span
                  className={`op-activity-panel-statusbar-chevron ${expanded ? 'is-expanded' : ''}`}
                  aria-hidden="true"
                >
                  <ChevronRightIcon className="w-4 h-4" />
                </span>
              </button>
              {/* Done chip pill on panel header — shell border is --op-sg-border on .op-activity-panel */}
              <span
                className={`${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_ACTIVITY_HEADER} op-activity-panel-status-chip is-${headerViewModel.tone}`}
              >
                {headerViewModel.label}
              </span>
            </div>
            {shouldShowActivityHeaderPreview(headerViewModel.preview, expanded, headerViewModel.tone) && (
              <div className="op-activity-panel-statusbar-live" aria-live="polite">
                {headerViewModel.preview}
              </div>
            )}
            {showLoadingAnimation && (
              <div
                className="op-activity-panel-statusbar-live is-cooking"
                aria-label={`${headerViewModel.label} in progress`}
                role="status"
              >
                <CookingAnimation />
              </div>
            )}
            <div className="op-activity-panel-statusbar-meta">
              {headerViewModel.hint && (
                <span className="op-activity-panel-statusbar-hint">{headerViewModel.hint}</span>
              )}
              <span className="op-activity-panel-statusbar-metric">{headerViewModel.metric}</span>
            </div>
          </div>
        </div>
      </div>

      {expanded && (
        <ThreadActivityView
          bodyRef={bodyRef}
          bodyMaxHeight={bodyMaxHeight}
          showEmptyState={showThreadActivityEmptyState}
        >
            {activeThreadID && (
              <div className="op-activity-panel-metadata op-md-frontmatter-properties" aria-label="Thread metadata">
                <div className="op-activity-panel-metadata-inner">
                  <div className="op-md-frontmatter-property-row">
                    <div className="op-md-frontmatter-property-label">
                      <span>Thread</span>
                    </div>
                    <div className="op-md-frontmatter-property-content">
                      <button
                        type="button"
                        className="op-md-frontmatter-property-value op-md-frontmatter-property-value-link"
                        title={activeThreadID}
                        aria-label={`Open thread file: ${activeThreadID}`}
                        onClick={handleActivityThreadIDClick}
                      >
                        {activeThreadID}
                      </button>
                    </div>
                  </div>
                  {threadMetadataViewModel.chatFileName && (
                    <div className="op-md-frontmatter-property-row">
                      <div className="op-md-frontmatter-property-label">
                        <span>File</span>
                      </div>
                      <div className="op-md-frontmatter-property-content">
                        <button
                          type="button"
                          className="op-md-frontmatter-property-value op-md-frontmatter-property-value-link"
                          title={activeChatPath}
                          aria-label={`Open chat file: ${threadMetadataViewModel.chatFileName}`}
                          onClick={handleActivityChatFileClick}
                        >
                          {threadMetadataViewModel.chatFileName}
                        </button>
                      </div>
                    </div>
                  )}
                  {threadMetadataViewModel.createdAtLabel && (
                    <div className="op-md-frontmatter-property-row">
                      <div className="op-md-frontmatter-property-label">
                        <span>Created</span>
                      </div>
                      <div className="op-md-frontmatter-property-content">
                        <span className="op-md-frontmatter-property-value">
                          {threadMetadataViewModel.createdAtLabel}
                        </span>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            )}
            {(canLoadOlderWindow || windowError) && (
              <div className="op-activity-panel-window-control is-top">
                {windowError ? (
                  <>
                    <span>{windowError}</span>
                    <button
                      type="button"
                      className="ui-pill-btn-secondary h-7 px-2.5 text-xs font-medium"
                      onClick={() => { void loadOlderWindow(); }}
                    >
                      Retry
                    </button>
                  </>
                ) : (
                  <button
                    type="button"
                    className="ui-pill-btn-secondary h-7 px-2.5 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-60"
                    disabled={!canLoadOlderWindow || windowLoading}
                    onClick={() => { void loadOlderWindow(); }}
                  >
                    {windowLoading ? 'Loading...' : 'Load earlier'}
                  </button>
                )}
              </div>
            )}
            {awaitingUser && (
              <EmbeddedQuestionsCard
                awaitingUser={awaitingUser}
                busy={false}
                error={null}
                highlightedIndex={highlightedQuestionOptionIndex}
                onPrevious={() => handleQuestionNavigate(-1)}
                onNext={() => handleQuestionNavigate(1)}
                onSelectOption={handleQuestionOptionSelect}
                onCustomAnswerChange={handleQuestionCustomAnswerChange}
                onSkip={handleQuestionSkip}
                onCancel={handleAwaitingUserCancel}
                onContinue={handleQuestionContinue}
              />
            )}
            {showPlanCard && buildConfig && (
              <PlanBuildCard
                title={planSnapshot?.title || threadMeta?.title || 'Plan'}
                planPath={planPath}
                buildConfig={buildConfig}
                setBuildConfig={setBuildConfig}
                agentOptions={agentOptions}
                enabledModels={enabledModels}
                onExecute={() => { void runPlanAction('execute'); }}
                onClearContextExecute={() => { void runPlanAction('clear-context'); }}
                onThinkingChange={handleGlobalThinkingChange}
                busy={planActionBusy || inProgress}
                canExecute={canExecutePlan}
                error={planActionError || planLoadError}
              />
            )}
            {showTodoList && (
              <PlanTodoList
                snapshot={planSnapshot}
                loading={planLoading}
                error={planActionError || planLoadError}
              />
            )}
            {pendingReviews.length > 0 && reviewChatPath && (
              <ReviewHistoryPanel
                title="Pending Review"
                reviews={pendingReviews}
                busyKey={reviewBusyKey}
                error={reviewError}
                {...reviewPanelActionProps}
              />
            )}
            {completedReviews.length > 0 && reviewChatPath && (
              <ReviewHistoryPanel
                title="Review History"
                reviews={completedReviews}
                busyKey={reviewBusyKey}
                error={pendingReviews.length > 0 ? null : reviewError}
                defaultCollapsed
                {...reviewPanelActionProps}
              />
            )}
            {autoRetryActive && (
              <RetryCard
                attempt={autoRetryAttempt}
                limit={autoRetryLimit}
                delayMs={autoRetryDelayMs}
                startedAt={autoRetryStartedAt}
                errorMessage={autoRetryErrorMessage}
                now={retryNow}
              />
            )}
            {errorMessage && (
              activityErrorInfo
                ? <BillingRestrictionCard info={activityErrorInfo} />
                : <ErrorCard message={errorMessage} />
            )}
            {timelineRenderItems.map(({ item, showParticipantHeader }) => {
              if (item.kind === 'message') {
                return (
                  <EntryMessageView
                    key={item.message.id}
                    message={item.message}
                    participant={resolveActivityMessageParticipant(item.message, activityMessageParticipants)}
                    showParticipantHeader={showParticipantHeader}
                    onAction={handleMessageAction}
                    onAnswer={handleMessageQuestionAnswer}
                    answeredRequest={item.message.recordID ? answeredRequestByID[item.message.recordID] : undefined}
                    requestAnswerDetails={item.message.recordID ? requestAnswerDetailsByID[item.message.recordID] : undefined}
                    replyToRequest={item.message.replyToMessageID ? messageRecordByID[item.message.replyToMessageID] : undefined}
                    pendingActionID={pendingMessageActionID}
                  />
                );
              }
              if (item.kind === 'stream') {
                return (
                  <StreamingTextView
                    key={item.segment.id}
                    text={item.segment.text}
                    participant={activityMessageParticipants.assistant}
                    showParticipantHeader={showParticipantHeader}
                  />
                );
              }
              const { step } = item;
              return (
                <LiveStepRow
                  key={step.id}
                  step={step}
                  participant={isAgentActivityTimelineItem(item) ? activityMessageParticipants.assistant : null}
                  showParticipantHeader={showParticipantHeader}
                />
              );
            })}
            {isHistoricalWindow && (
              <div className="op-activity-panel-window-control is-bottom">
                <button
                  type="button"
                  className="ui-pill-btn-secondary h-7 px-2.5 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={windowLoading}
                  onClick={() => { void jumpToLatestWindow(); }}
                >
                  Jump to latest
                </button>
              </div>
            )}
        </ThreadActivityView>
      )}
    </div>
  );
}
