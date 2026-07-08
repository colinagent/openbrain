import { useStore } from 'zustand';
import { createStore, type StoreApi } from 'zustand/vanilla';
import { useTabManagerStore } from './tabManagerStore';
import { useModelsStore } from './modelsStore';
import { retargetChatSnapshot } from '../utils/chatRetarget';
import {
  areComposerPlanStatesEqual,
  removeComposerPlanBlock,
  type ComposerPlanState,
} from '../utils/chatPlanBlock';
import { resolveDefaultChatModelSelection } from '../utils/chatModelSelection';
import type { ThreadReviewState } from '../services/reviewService';
import type { ChatMessageRecord, ThreadEntry, ThreadSnapshot, ThreadSnapshotEntryWindow } from '../services/threadService';
import { normalizeModelKey } from '../../shared/modelKeys';

export type ChatInputMode = 'chat' | 'command';
export type LiveStepType = 'toolcall' | 'reasoning' | 'error' | 'tokenUsage' | 'notice';
export type LiveStepStatus = 'running' | 'done' | 'error';
export type ConversationRunStatus = 'running' | 'awaiting_user' | 'complete' | 'idle';
export type ThreadRunStatus = 'idle' | 'running';
export type ThreadTailStatus = 'empty' | 'complete' | 'needs_continuation';
export type ThreadContinuationReason = '' | 'user_tail' | 'tool_result_tail' | 'assistant_tool_use' | 'assistant_error' | 'assistant_aborted';

export type ThreadState = {
  runStatus: ThreadRunStatus;
  tailStatus: ThreadTailStatus;
  continuationReason: ThreadContinuationReason;
};

export type LiveStep = {
  id: string;
  type: LiveStepType;
  label: string;
  status: LiveStepStatus;
  detail?: string;
  toolCall?: {
    id?: string;
    name?: string;
    rawArguments?: string;
    arguments?: Record<string, unknown>;
    complete?: boolean;
  };
  toolOutput?: string;
  ts: number;
  order?: number;
};

export type LiveTextSegment = {
  id: string;
  text: string;
  ts: number;
  order: number;
};

export type TokenUsage = {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  contextTokens?: number | null;
  contextWindow?: number;
  contextPercent?: number | null;
  contextKnown?: boolean | null;
};

export type ThreadSummary = {
  threadID: string;
  fileID?: string;
  agentID: string;
  cwd: string;
  path?: string;
  chatPath: string;
  threadFilePath?: string;
  title: string;
  parentThreadID?: string;
  planPath?: string;
  executionPlanPath?: string;
};

type LiveStepPatch = Partial<Pick<LiveStep, 'status' | 'detail' | 'label' | 'toolCall' | 'toolOutput'>>;
export type ThreadSnapshotWindowMergeDirection = 'tail' | 'prepend' | 'append';

export type PendingConversation = {
  id: string;
  createdAt: number;
};

export type SelectedSkill = {
  id: string;
  slug: string;
  name: string;
};

export type SelectedSkillContext = {
  planFilePath?: string;
  planDir?: string;
  title?: string;
};

export type SelectedAgentTarget = {
  agentID: string;
  agentName: string | null;
  agentCwd: string;
};

const PLAN_SKILL_SLUG = 'plan';
export const THREAD_SNAPSHOT_ENTRY_WINDOW_MAX = 600;

export type QueuedMessageKind = 'steering' | 'follow_up';

export type QueuedMessage = {
  chatPath: string;
  id: string;
  kind: QueuedMessageKind;
  text: string;
  agentID?: string;
  agentName?: string | null;
  agentCwd?: string;
  selectedSkill: SelectedSkill | null;
  selectedSkillIDs: string[];
  selectedSkillContext: SelectedSkillContext | null;
  planTurn: boolean;
  pending?: boolean;
};

export type QueuedMessageBucket = {
  steering: QueuedMessage[];
  followUp: QueuedMessage[];
};

export type AwaitingUserState = {
  questions: Array<{
    header: string;
    question: string;
    options: Array<{
      label: string;
      description?: string;
    }>;
    multiple?: boolean;
    custom?: boolean;
  }>;
  currentIndex: number;
  answers: string[][];
  customModeByIndex: boolean[];
  requestID: string;
  requestedAt: number;
};

export type ConversationTarget =
  | { kind: 'thread'; threadID: string; chatPath?: string }
  | { kind: 'command'; path: string }
  | { kind: 'pending'; id: string }
  | null;

export type OpenComposerTarget = {
  threadID: string;
  chatPath?: string;
  title?: string;
  agentID?: string;
};

export type OpenThreadConversationOptions = {
  chatPath?: string | null;
  title?: string | null;
  agentID?: string | null;
};

export type GBrainQueryScopePublicSource = {
  sourceID: string;
  name?: string;
  workspaceID?: string;
  orgID?: string;
};

export type GBrainQueryScope =
  | {
    kind: 'source';
    label: string;
    sourceID: string;
    workspaceID?: string;
    orgID?: string;
  }
  | {
    kind: 'publicBrain';
    label: string;
    ownerUID: string;
    username?: string;
    sources: GBrainQueryScopePublicSource[];
  }
  | null;

export type ActiveCommandRun = {
  commandID: string;
  filePath: string;
};

type LiveOverlay = {
  streamingText: string;
  streamingStartedAt: number | null;
  streamingSegments: LiveTextSegment[];
  activeStreamingSegmentID: string | null;
  errorMessage: string | null;
  errorCode: string | null;
  autoRetryActive: boolean;
  autoRetryAttempt: number;
  autoRetryLimit: number;
  autoRetryDelayMs: number;
  autoRetryStartedAt: number | null;
  autoRetryErrorMessage: string | null;
  reconnectAttempt: number;
  reconnectLimit: number;
  reconnectingMessage: string | null;
  steps: LiveStep[];
  expanded: boolean;
  /**
   * Sticky user override for the thread.
   * 'expanded' = user manually opened  → don't auto-collapse
   * 'collapsed' = user manually closed → don't auto-expand
   * null = no override, programmatic behavior applies
   */
  userOverride: 'expanded' | 'collapsed' | null;
  loopUsage: TokenUsage;
};

type ReviewBucket = ThreadReviewState[];
type PendingComposerInsert = {
  id: string;
  markdown: string;
};

const EMPTY_THREAD_STATE: ThreadState = {
  runStatus: 'idle',
  tailStatus: 'empty',
  continuationReason: '',
};

const EMPTY_LIVE_OVERLAY: LiveOverlay = {
  streamingText: '',
  streamingStartedAt: null,
  streamingSegments: [],
  activeStreamingSegmentID: null,
  errorMessage: null,
  errorCode: null,
  autoRetryActive: false,
  autoRetryAttempt: 0,
  autoRetryLimit: 0,
  autoRetryDelayMs: 0,
  autoRetryStartedAt: null,
  autoRetryErrorMessage: null,
  reconnectAttempt: 0,
  reconnectLimit: 0,
  reconnectingMessage: null,
  steps: [],
  expanded: false,
  userOverride: null,
  loopUsage: { inputTokens: 0, outputTokens: 0, totalTokens: 0 },
};
const EMPTY_REVIEWS: ThreadReviewState[] = [];

const EMPTY_QUEUED_MESSAGES: QueuedMessageBucket = {
  steering: [],
  followUp: [],
};

const threadStateBySnapshot = new WeakMap<ThreadSnapshot, ThreadState>();

let pendingConversationSeq = 0;
let pendingComposerInsertSeq = 0;
let activityTimelineOrder = 0;

function nextActivityTimelineOrder(): number {
  activityTimelineOrder += 1;
  return activityTimelineOrder;
}

function createEmptyLiveOverlay(): LiveOverlay {
  return {
    streamingText: '',
    streamingStartedAt: null,
    streamingSegments: [],
    activeStreamingSegmentID: null,
    errorMessage: null,
    errorCode: null,
    autoRetryActive: false,
    autoRetryAttempt: 0,
    autoRetryLimit: 0,
    autoRetryDelayMs: 0,
    autoRetryStartedAt: null,
    autoRetryErrorMessage: null,
    reconnectAttempt: 0,
    reconnectLimit: 0,
    reconnectingMessage: null,
    steps: [],
    expanded: false,
    userOverride: null,
    loopUsage: { inputTokens: 0, outputTokens: 0, totalTokens: 0 },
  };
}

function clearReconnectState(bucket: LiveOverlay): LiveOverlay {
  if (!bucket.reconnectingMessage && bucket.reconnectAttempt === 0 && bucket.reconnectLimit === 0) {
    return bucket;
  }
  return {
    ...bucket,
    reconnectAttempt: 0,
    reconnectLimit: 0,
    reconnectingMessage: null,
  };
}

function clearAutoRetryState(bucket: LiveOverlay): LiveOverlay {
  if (
    !bucket.autoRetryActive
    && bucket.autoRetryAttempt === 0
    && bucket.autoRetryLimit === 0
    && bucket.autoRetryDelayMs === 0
    && bucket.autoRetryStartedAt == null
    && !bucket.autoRetryErrorMessage
  ) {
    return bucket;
  }
  return {
    ...bucket,
    autoRetryActive: false,
    autoRetryAttempt: 0,
    autoRetryLimit: 0,
    autoRetryDelayMs: 0,
    autoRetryStartedAt: null,
    autoRetryErrorMessage: null,
  };
}

function getThreadStateFromSnapshot(snapshot: ThreadSnapshot | null | undefined): ThreadState {
  if (!snapshot) {
    return EMPTY_THREAD_STATE;
  }
  const cached = threadStateBySnapshot.get(snapshot);
  if (cached) {
    return cached;
  }
  const threadState = {
    runStatus: snapshot.runStatus || EMPTY_THREAD_STATE.runStatus,
    tailStatus: snapshot.tailStatus || EMPTY_THREAD_STATE.tailStatus,
    continuationReason: snapshot.continuationReason || EMPTY_THREAD_STATE.continuationReason,
  };
  threadStateBySnapshot.set(snapshot, threadState);
  return threadState;
}

function patchSnapshotThreadState(snapshot: ThreadSnapshot, threadState: Partial<ThreadState>): ThreadSnapshot {
  return {
    ...snapshot,
    ...(threadState.runStatus ? { runStatus: threadState.runStatus } : {}),
    ...(threadState.tailStatus ? { tailStatus: threadState.tailStatus } : {}),
    ...(threadState.continuationReason !== undefined ? { continuationReason: threadState.continuationReason } : {}),
  };
}

function clearTransientRecoveryState(bucket: LiveOverlay): LiveOverlay {
  return clearReconnectState(clearAutoRetryState(bucket));
}

function clearTransientAttemptArtifacts(bucket: LiveOverlay): LiveOverlay {
  const nextSteps = bucket.steps.filter((step) => step.status !== 'running' && step.type !== 'error');
  const stepChanged = nextSteps.length !== bucket.steps.length;
  if (
    !stepChanged
    && !bucket.streamingText
    && bucket.streamingStartedAt == null
    && bucket.streamingSegments.length === 0
    && !bucket.errorMessage
  ) {
    return bucket;
  }
  return {
    ...bucket,
    streamingText: '',
    streamingStartedAt: null,
    streamingSegments: [],
    activeStreamingSegmentID: null,
    errorMessage: null,
    errorCode: null,
    steps: nextSteps,
  };
}

function clearLiveOverlayState(bucket: LiveOverlay): LiveOverlay {
  return {
    ...createEmptyLiveOverlay(),
    expanded: bucket.expanded,
    userOverride: bucket.userOverride,
  };
}

function prepareLiveOverlayForNewRunState(bucket: LiveOverlay): LiveOverlay {
  return {
    ...clearLiveOverlayState(bucket),
    activeStreamingSegmentID: null,
  };
}

function normalizeChatPath(chatPath: string | null | undefined): string {
  return (chatPath || '').trim();
}

function normalizeErrorCode(errorCode: string | null | undefined): string | null {
  const normalized = (errorCode || '').trim();
  return normalized || null;
}

function toCommandTarget(chatPath: string | null | undefined): ConversationTarget {
  const normalized = normalizeChatPath(chatPath);
  return normalized ? { kind: 'command', path: normalized } : null;
}

function toThreadTarget(threadID: string | null | undefined, chatPath: string | null | undefined): ConversationTarget {
  const normalizedThreadID = (threadID || '').trim();
  const normalizedChatPath = normalizeChatPath(chatPath);
  return normalizedThreadID
    ? {
      kind: 'thread',
      threadID: normalizedThreadID,
      ...(normalizedChatPath ? { chatPath: normalizedChatPath } : {}),
    }
    : null;
}

export function getConversationTargetKey(target: ConversationTarget): string | null {
  if (!target) {
    return null;
  }
  switch (target.kind) {
    case 'thread':
      return `thread:${target.threadID}`;
    case 'command':
      return `command:${target.path}`;
    case 'pending':
      return `pending:${target.id}`;
  }
}

function getActivityTargetKey(target: ConversationTarget): string | null {
  if (!target) {
    return null;
  }
  if (target.kind === 'thread') {
    return target.threadID;
  }
  if (target.kind === 'command') {
    return normalizeChatPath(target.path);
  }
  return `pending:${target.id}`;
}

type ChatIndexState = {
  pathToThreadID: Record<string, string>;
};

function resolveChatStateKey(source: ChatIndexState, chatPath: string | null | undefined): string {
  const normalizedPath = normalizeChatPath(chatPath);
  if (!normalizedPath) {
    return '';
  }
  return (source.pathToThreadID[normalizedPath] || '').trim() || normalizedPath;
}

function resolveConversationTargetForPath(source: ChatIndexState, chatPath: string | null | undefined): ConversationTarget {
  const normalizedPath = normalizeChatPath(chatPath);
  if (!normalizedPath) {
    return null;
  }
  const threadID = (source.pathToThreadID[normalizedPath] || '').trim();
  return threadID
    ? toThreadTarget(threadID, normalizedPath)
    : toCommandTarget(normalizedPath);
}

function resolveTargetStateKey(source: ChatIndexState, target: ConversationTarget): string | null {
  if (!target) {
    return null;
  }
  if (target.kind === 'pending') {
    return `pending:${target.id}`;
  }
  if (target.kind === 'thread') {
    return target.threadID;
  }
  return normalizeChatPath(target.path) || null;
}

function moveRecordValue<T>(record: Record<string, T>, fromKey: string, toKey: string): Record<string, T> {
  if (!fromKey || !toKey || fromKey === toKey || !(fromKey in record)) {
    return record;
  }
  const next = { ...record };
  next[toKey] = next[fromKey];
  delete next[fromKey];
  return next;
}

function moveRecordValueFromCandidates<T>(
  record: Record<string, T>,
  fromKeys: Array<string | null | undefined>,
  toKey: string,
): Record<string, T> {
  if (!toKey) {
    return record;
  }
  let next = record;
  for (const rawKey of fromKeys) {
    const fromKey = (rawKey || '').trim();
    if (!fromKey || fromKey === toKey || !(fromKey in next)) {
      continue;
    }
    if (next === record) {
      next = { ...record };
    }
    if (!(toKey in next)) {
      next[toKey] = next[fromKey];
    }
    delete next[fromKey];
  }
  return next;
}

function hasOwnSetting<T extends object>(value: T, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function createPendingConversation(): PendingConversation {
  pendingConversationSeq += 1;
  return {
    id: `pending-${Date.now()}-${pendingConversationSeq}`,
    createdAt: Date.now(),
  };
}

function upsertOpenComposerTarget(
  conversations: OpenComposerTarget[],
  next: OpenComposerTarget,
): OpenComposerTarget[] {
  const threadID = (next.threadID || '').trim();
  if (!threadID) {
    return conversations;
  }
  const normalized: OpenComposerTarget = {
    threadID,
    ...(normalizeChatPath(next.chatPath) ? { chatPath: normalizeChatPath(next.chatPath) } : {}),
    ...((next.title || '').trim() ? { title: (next.title || '').trim() } : {}),
    ...((next.agentID || '').trim() ? { agentID: (next.agentID || '').trim() } : {}),
  };
  const existingIndex = conversations.findIndex((item) => item.threadID === threadID);
  if (existingIndex < 0) {
    return [...conversations, normalized];
  }
  return conversations.map((item, index) => {
    if (index !== existingIndex) {
      return item;
    }
    return {
      ...item,
      ...normalized,
      chatPath: normalized.chatPath || item.chatPath,
      title: normalized.title || item.title,
      agentID: normalized.agentID || item.agentID,
    };
  });
}

function createPendingComposerInsert(markdown: string): PendingComposerInsert | null {
  const normalized = markdown.trim();
  if (!normalized) {
    return null;
  }
  pendingComposerInsertSeq += 1;
  return {
    id: `composer-insert-${Date.now()}-${pendingComposerInsertSeq}`,
    markdown: normalized,
  };
}

function initializeTargetModelState(
  modelKeyByTargetKey: Record<string, string>,
  rememberedModelKey: string | null | undefined,
  target: ConversationTarget,
): Record<string, string> {
  const draftKey = getConversationTargetKey(target);
  const initialModelKey = normalizeModelKey(rememberedModelKey)
    || resolveDefaultChatModelSelection(useModelsStore.getState().config).modelKey
    || '';
  if (!draftKey || !initialModelKey) {
    return modelKeyByTargetKey;
  }
  return {
    ...modelKeyByTargetKey,
    [draftKey]: initialModelKey,
  };
}

function normalizeSelectedAgentTarget(target: SelectedAgentTarget | null | undefined): SelectedAgentTarget | null {
  const agentID = (target?.agentID || '').trim();
  const agentCwd = normalizeChatPath(target?.agentCwd || '');
  if (!agentID) {
    return null;
  }
  return {
    agentID,
    agentName: (target?.agentName || '').trim() || null,
    agentCwd,
  };
}

function initializeTargetAgentState(
  agentByTargetKey: Record<string, SelectedAgentTarget>,
  target: ConversationTarget,
  agentTarget: SelectedAgentTarget | null | undefined,
): Record<string, SelectedAgentTarget> {
  const targetKey = getConversationTargetKey(target);
  const normalizedAgent = normalizeSelectedAgentTarget(agentTarget);
  if (!targetKey || !normalizedAgent) {
    return agentByTargetKey;
  }
  return {
    ...agentByTargetKey,
    [targetKey]: normalizedAgent,
  };
}

function clearAllComposerPlanDraftSpacers(
  draftByTargetKey: Record<string, string>,
  composerPlanStateByTargetKey: Record<string, ComposerPlanState>,
): Record<string, string> {
  if (Object.keys(composerPlanStateByTargetKey).length === 0) {
    return draftByTargetKey;
  }
  const nextDraftByTargetKey = { ...draftByTargetKey };
  for (const [targetKey, planState] of Object.entries(composerPlanStateByTargetKey)) {
    const currentDraft = nextDraftByTargetKey[targetKey];
    if (typeof currentDraft !== 'string') {
      continue;
    }
    const cleaned = removeComposerPlanBlock({
      content: currentDraft,
      cursor: currentDraft.length,
      planState,
    });
    if (cleaned.content) {
      nextDraftByTargetKey[targetKey] = cleaned.content;
    } else {
      delete nextDraftByTargetKey[targetKey];
    }
  }
  return nextDraftByTargetKey;
}

function createPendingTargetState(state: {
  pendingConversations: PendingConversation[];
  modelKeyByTargetKey: Record<string, string>;
  agentByTargetKey: Record<string, SelectedAgentTarget>;
  rememberedModelKey: string | null;
  agentID: string | null;
  agentName: string | null;
  agentCwd: string | null;
}) {
  const pending = createPendingConversation();
  const target: ConversationTarget = { kind: 'pending', id: pending.id };
  return {
    pending,
    target,
    pendingConversations: [...state.pendingConversations, pending],
    modelKeyByTargetKey: initializeTargetModelState(
      state.modelKeyByTargetKey,
      state.rememberedModelKey,
      target,
    ),
    agentByTargetKey: initializeTargetAgentState(
      state.agentByTargetKey,
      target,
      null,
    ),
  };
}

function cloneQueuedMessage(message: QueuedMessage, overrides?: Partial<QueuedMessage>): QueuedMessage {
  const cloned: QueuedMessage = {
    chatPath: message.chatPath,
    id: message.id,
    kind: message.kind,
    text: message.text,
    agentID: (message.agentID || '').trim() || undefined,
    agentName: (message.agentName || '').trim() || null,
    agentCwd: normalizeChatPath(message.agentCwd || '') || undefined,
    selectedSkill: message.selectedSkill ? { ...message.selectedSkill } : null,
    selectedSkillIDs: [...message.selectedSkillIDs],
    selectedSkillContext: message.selectedSkillContext ? { ...message.selectedSkillContext } : null,
    planTurn: message.planTurn,
    pending: message.pending === true ? true : undefined,
    ...overrides,
  };
  if (cloned.pending !== true) {
    delete cloned.pending;
  }
  if (!cloned.agentID) {
    delete cloned.agentID;
  }
  if (!cloned.agentCwd) {
    delete cloned.agentCwd;
  }
  if (!cloned.agentName) {
    delete cloned.agentName;
  }
  return cloned;
}

function cloneQueuedMessageBucket(bucket: QueuedMessageBucket): QueuedMessageBucket {
  return {
    steering: bucket.steering.map((message) => cloneQueuedMessage(message)),
    followUp: bucket.followUp.map((message) => cloneQueuedMessage(message)),
  };
}

function cloneThreadEntry(entry: ThreadEntry): ThreadEntry {
  return { ...entry };
}

function cloneChatMessageRecord(record: ChatMessageRecord): ChatMessageRecord {
  return {
    ...record,
    actions: Array.isArray(record.actions)
      ? record.actions.map((action) => ({ ...action }))
      : undefined,
    questions: Array.isArray(record.questions)
      ? record.questions.map((question) => ({
        ...question,
        options: Array.isArray(question.options)
          ? question.options.map((option) => ({ ...option }))
          : undefined,
      }))
      : undefined,
    answers: Array.isArray(record.answers)
      ? record.answers.map((answer) => ({ ...answer }))
      : undefined,
    meta: record.meta ? { ...record.meta } : undefined,
  };
}

function cloneThreadSnapshot(snapshot: ThreadSnapshot | null | undefined): ThreadSnapshot | null {
  const threadID = (snapshot?.meta?.threadID || '').trim();
  if (!snapshot || !threadID) {
    return null;
  }
  const revision = (snapshot.revision || '').trim() || threadID;
  return {
    ...snapshot,
    meta: {
      ...snapshot.meta,
      threadID,
      chatPath: normalizeChatPath(snapshot.meta.chatPath || snapshot.meta.path),
      path: normalizeChatPath(snapshot.meta.path || snapshot.meta.chatPath),
      title: (snapshot.meta.title || '').trim(),
      agentID: (snapshot.meta.agentID || '').trim(),
      cwd: normalizeChatPath(snapshot.meta.cwd),
    },
    entries: Array.isArray(snapshot.entries)
      ? snapshot.entries.map((entry) => cloneThreadEntry(entry))
      : [],
    messageRecords: Array.isArray(snapshot.messageRecords)
      ? snapshot.messageRecords.map((record) => cloneChatMessageRecord(record))
      : undefined,
    entryWindow: snapshot.entryWindow ? { ...snapshot.entryWindow } : undefined,
    revision,
  };
}

function upsertMessageRecordsForSnapshot(
  snapshot: ThreadSnapshot,
  records: ChatMessageRecord[],
): ThreadSnapshot {
  let messageRecords = Array.isArray(snapshot.messageRecords)
    ? snapshot.messageRecords.map((record) => cloneChatMessageRecord(record))
    : [];
  let changed = false;

  for (const record of records) {
    const id = (record?.id || '').trim();
    if (!id) {
      continue;
    }
    const index = messageRecords.findIndex((item) => item.id === id);
    if (record.status === 'archived') {
      if (index >= 0) {
        messageRecords = [
          ...messageRecords.slice(0, index),
          ...messageRecords.slice(index + 1),
        ];
        changed = true;
      }
      continue;
    }
    const nextRecord = cloneChatMessageRecord(record);
    if (index >= 0) {
      messageRecords = [
        ...messageRecords.slice(0, index),
        nextRecord,
        ...messageRecords.slice(index + 1),
      ];
    } else {
      messageRecords = [...messageRecords, nextRecord];
    }
    changed = true;
  }

  return changed ? { ...snapshot, messageRecords } : snapshot;
}

function positiveInt(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(0, Math.floor(value))
    : null;
}

function resolveSnapshotWindowStart(snapshot: ThreadSnapshot): number {
  return positiveInt(snapshot.entryWindow?.start) ?? 0;
}

function resolveSnapshotWindowTotal(snapshot: ThreadSnapshot, fallbackEnd: number): number {
  const total = positiveInt(snapshot.entryWindow?.total);
  return total == null ? fallbackEnd : Math.max(total, fallbackEnd);
}

function threadEntryMergeKey(entry: ThreadEntry, globalIndex: number, localIndex: number): string {
  const id = (typeof entry.id === 'string' ? entry.id : '').trim();
  if (id) {
    return `id:${id}`;
  }
  const type = (typeof entry.type === 'string' ? entry.type : '').trim();
  const timestamp = (typeof entry.timestamp === 'string' ? entry.timestamp : '').trim();
  return `anon:${type}:${timestamp}:${globalIndex}:${localIndex}`;
}

type IndexedThreadEntry = {
  entry: ThreadEntry;
  index: number;
  key: string;
};

function indexedThreadSnapshotEntries(snapshot: ThreadSnapshot): IndexedThreadEntry[] {
  const entries = Array.isArray(snapshot.entries) ? snapshot.entries : [];
  const start = resolveSnapshotWindowStart(snapshot);
  return entries.map((entry, localIndex) => {
    const index = start + localIndex;
    return {
      entry: cloneThreadEntry(entry),
      index,
      key: threadEntryMergeKey(entry, index, localIndex),
    };
  });
}

function uniqueIndexedEntries(entries: IndexedThreadEntry[]): IndexedThreadEntry[] {
  const seen = new Set<string>();
  const out: IndexedThreadEntry[] = [];
  for (const entry of entries) {
    if (seen.has(entry.key)) {
      continue;
    }
    seen.add(entry.key);
    out.push(entry);
  }
  return out;
}

function clampIndexedEntries(
  entries: IndexedThreadEntry[],
  direction: ThreadSnapshotWindowMergeDirection,
): IndexedThreadEntry[] {
  if (entries.length <= THREAD_SNAPSHOT_ENTRY_WINDOW_MAX) {
    return entries;
  }
  if (direction === 'prepend') {
    return entries.slice(0, THREAD_SNAPSHOT_ENTRY_WINDOW_MAX);
  }
  return entries.slice(entries.length - THREAD_SNAPSHOT_ENTRY_WINDOW_MAX);
}

function buildMergedEntryWindow(
  entries: IndexedThreadEntry[],
  snapshot: ThreadSnapshot,
  fallbackSnapshot?: ThreadSnapshot | null,
): ThreadSnapshotEntryWindow | undefined {
  const sourceWindow = snapshot.entryWindow || fallbackSnapshot?.entryWindow;
  if (!sourceWindow && entries.length === 0) {
    return undefined;
  }
  const fallbackStart = sourceWindow?.start ?? 0;
  const start = entries[0]?.index ?? fallbackStart;
  const end = entries.length > 0
    ? entries[entries.length - 1].index + 1
    : (sourceWindow?.end ?? fallbackStart);
  const fallbackTotal = fallbackSnapshot
    ? resolveSnapshotWindowTotal(fallbackSnapshot, end)
    : end;
  const total = resolveSnapshotWindowTotal(snapshot, fallbackTotal);
  return {
    ...(sourceWindow || {}),
    start,
    end,
    total,
    hasBefore: start > 0,
    hasAfter: end < total,
  };
}

function capThreadSnapshotWindow(
  snapshot: ThreadSnapshot,
  direction: ThreadSnapshotWindowMergeDirection,
): ThreadSnapshot {
  const indexedEntries = clampIndexedEntries(indexedThreadSnapshotEntries(snapshot), direction);
  return {
    ...snapshot,
    entries: indexedEntries.map((item) => cloneThreadEntry(item.entry)),
    entryWindow: buildMergedEntryWindow(indexedEntries, snapshot),
  };
}

function mergeThreadSnapshotWindowForStore(
  current: ThreadSnapshot | null | undefined,
  snapshot: ThreadSnapshot,
  direction: ThreadSnapshotWindowMergeDirection,
): ThreadSnapshot {
  if (!current || direction === 'tail') {
    return capThreadSnapshotWindow(snapshot, 'tail');
  }
  const currentThreadID = (current.meta.threadID || '').trim();
  const nextThreadID = (snapshot.meta.threadID || '').trim();
  const currentRevision = (current.revision || '').trim();
  const nextRevision = (snapshot.revision || '').trim();
  if (!currentThreadID || currentThreadID !== nextThreadID || currentRevision !== nextRevision) {
    return capThreadSnapshotWindow(snapshot, 'tail');
  }

  const currentEntries = indexedThreadSnapshotEntries(current);
  const nextEntries = indexedThreadSnapshotEntries(snapshot);
  const combined = direction === 'prepend'
    ? uniqueIndexedEntries([...nextEntries, ...currentEntries])
    : uniqueIndexedEntries([...currentEntries, ...nextEntries]);
  const sorted = combined.sort((a, b) => a.index - b.index);
  const capped = clampIndexedEntries(sorted, direction);
  return {
    ...snapshot,
    entries: capped.map((item) => cloneThreadEntry(item.entry)),
    entryWindow: buildMergedEntryWindow(capped, snapshot, current),
  };
}

function createEmptyThreadSnapshot(threadID: string, meta?: ThreadSummary | null): ThreadSnapshot {
  const normalizedThreadID = threadID.trim();
  return {
    meta: {
      threadID: normalizedThreadID,
      fileID: meta?.fileID,
      agentID: (meta?.agentID || '').trim(),
      cwd: normalizeChatPath(meta?.cwd || ''),
      path: normalizeChatPath(meta?.path || meta?.chatPath || ''),
      chatPath: normalizeChatPath(meta?.chatPath || meta?.path || ''),
      threadFilePath: meta?.threadFilePath,
      title: (meta?.title || normalizedThreadID).trim(),
      parentThreadID: meta?.parentThreadID,
      planPath: meta?.planPath,
      executionPlanPath: meta?.executionPlanPath,
    },
    entries: [],
    revision: normalizedThreadID,
    runStatus: EMPTY_THREAD_STATE.runStatus,
    tailStatus: EMPTY_THREAD_STATE.tailStatus,
    continuationReason: EMPTY_THREAD_STATE.continuationReason,
  };
}

function clearSettledLiveOverlayAfterSnapshot(overlay: LiveOverlay, snapshot: ThreadSnapshot): LiveOverlay {
  if (snapshot.runStatus === 'running') {
    return overlay;
  }
  if (
    !overlay.streamingText
    && overlay.streamingStartedAt == null
    && overlay.streamingSegments.length === 0
    && overlay.steps.length === 0
    && !overlay.autoRetryActive
    && !overlay.reconnectingMessage
  ) {
    return overlay;
  }
  return {
    ...overlay,
    streamingText: '',
    streamingStartedAt: null,
    streamingSegments: [],
    activeStreamingSegmentID: null,
    steps: [],
    autoRetryActive: false,
    autoRetryAttempt: 0,
    autoRetryLimit: 0,
    autoRetryDelayMs: 0,
    autoRetryStartedAt: null,
    autoRetryErrorMessage: null,
    reconnectAttempt: 0,
    reconnectLimit: 0,
    reconnectingMessage: null,
  };
}

const INITIAL_CHAT_STATE = {
  composerVisible: false,
  composerFocusRequestSeq: 0,
  pendingComposerInsertQueue: [] as PendingComposerInsert[],
  targetChatPath: null as string | null,
  selectedConversationTarget: null as ConversationTarget,
  openComposerTargets: [] as OpenComposerTarget[],
  pendingConversations: [] as PendingConversation[],
  draftByTargetKey: {} as Record<string, string>,
  composerPlanStateByTargetKey: {} as Record<string, ComposerPlanState>,
  inputMode: 'chat' as ChatInputMode,
  agentID: null as string | null,
  agentName: null as string | null,
  agentCwd: null as string | null,
  agentByTargetKey: {} as Record<string, SelectedAgentTarget>,
  gbrainQueryScope: null as GBrainQueryScope,
  queuedMessagesByThreadID: {} as Record<string, QueuedMessageBucket>,
  selectedSkill: null as SelectedSkill | null,
  rememberedModelKey: null as string | null,
  modelKeyByTargetKey: {} as Record<string, string>,
  activeCommand: null as ActiveCommandRun | null,
  inProgressByTargetKey: {} as Record<string, true>,
  pendingScrollToBottomByThreadID: {} as Record<string, true>,
  awaitingUserByThreadID: {} as Record<string, AwaitingUserState>,
  error: null as string | null,
  threadSnapshotByID: {} as Record<string, ThreadSnapshot>,
  liveOverlayByThreadID: {} as Record<string, LiveOverlay>,
  reviewByThreadID: {} as Record<string, ReviewBucket>,
  threadMetaByThreadID: {} as Record<string, ThreadSummary>,
  pathToThreadID: {} as Record<string, string>,
  planRevisionByThreadID: {} as Record<string, number>,
};

export type ChatWorkspaceState = typeof INITIAL_CHAT_STATE & {
  showComposer: () => void;
  hideComposer: () => void;
  setComposerVisible: (visible: boolean) => void;
  requestComposerFocus: () => void;
  requestComposerBlockInsert: (markdown: string) => string | null;
  consumeComposerBlockInsert: (id: string) => void;
  setTargetChatPath: (path: string | null) => void;
  selectChatConversation: (path: string | null) => void;
  selectThreadConversation: (threadID: string, chatPath?: string | null) => void;
  openThreadConversation: (threadID: string, options?: OpenThreadConversationOptions) => void;
  closeComposerTarget: (threadID: string) => void;
  createPendingConversation: () => string;
  selectPendingConversation: (id: string) => void;
  removePendingConversation: (id: string) => void;
  getDraftForTarget: (target: ConversationTarget) => string;
  getComposerPlanStateForTarget: (target: ConversationTarget) => ComposerPlanState | null;
  setDraftForTarget: (target: ConversationTarget, draft: string) => void;
  setDraftForSelectedTarget: (draft: string) => void;
  setComposerPlanStateForTarget: (target: ConversationTarget, planState: ComposerPlanState | null) => void;
  setComposerPlanStateForSelectedTarget: (planState: ComposerPlanState | null) => void;
  clearComposerPlanStateForTarget: (target: ConversationTarget) => void;
  clearComposerPlanStateForSelectedTarget: () => void;
  consumePendingConversation: (pendingId: string, chatPath: string, threadID?: string | null) => void;
  setInputMode: (mode: ChatInputMode) => void;
  setAgentInfo: (key: string | null, name: string | null, cwd: string | null) => void;
  getAgentForTarget: (target: ConversationTarget) => SelectedAgentTarget | null;
  getSelectedAgent: () => SelectedAgentTarget | null;
  setAgentForTarget: (target: ConversationTarget, agent: SelectedAgentTarget | null) => void;
  setAgentForSelectedTarget: (agent: SelectedAgentTarget | null) => void;
  setGBrainQueryScope: (scope: GBrainQueryScope) => void;
  clearGBrainQueryScope: () => void;
  getQueuedMessages: (chatPath: string | null | undefined) => QueuedMessageBucket;
  getQueuedMessagesForTarget: (target: ConversationTarget) => QueuedMessageBucket;
  syncQueuedMessages: (chatPath: string, queuedMessages: QueuedMessageBucket) => void;
  appendQueuedMessage: (chatPath: string, message: QueuedMessage) => void;
  removeQueuedMessage: (chatPath: string | null | undefined, itemID: string) => void;
  clearQueuedMessages: (chatPath: string | null | undefined) => void;
  clearAllQueuedMessages: () => void;
  setSelectedSkill: (skill: SelectedSkill | null) => void;
  clearSelectedSkill: () => void;
  setRememberedModelKey: (key: string | null) => void;
  getSelectedModelKey: () => string | null;
  getModelKeyForTarget: (target: ConversationTarget) => string | null;
  setSelectedModelKey: (key: string | null) => void;
  syncChatSettings: (path: string, settings: { modelKey?: string | null }) => void;
  retargetChatPath: (oldPath: string, newPath: string) => void;
  setActiveCommand: (command: ActiveCommandRun | null) => void;
  isTargetInProgress: (target: ConversationTarget) => boolean;
  getConversationRunStatus: (target: ConversationTarget) => ConversationRunStatus;
  hasAnyInProgress: () => boolean;
  setTargetInProgress: (target: ConversationTarget, inProgress: boolean) => void;
  setChatPathInProgress: (chatPath: string | null | undefined, inProgress: boolean) => void;
  requestChatScrollToBottom: (chatPath: string | null | undefined) => void;
  consumeChatScrollToBottom: (chatPath: string | null | undefined) => void;
  shouldScrollChatToBottom: (chatPath: string | null | undefined) => boolean;
  clearAllInProgress: () => void;
  setAwaitingUser: (chatPath: string, awaiting: AwaitingUserState | null) => void;
  clearAwaitingUser: (chatPath: string | null | undefined) => void;
  clearAllAwaitingUsers: () => void;
  getAwaitingUser: (chatPath: string | null | undefined) => AwaitingUserState | null;
  setError: (error: string | null, errorCode?: string | null) => void;
  setErrorForTarget: (target: ConversationTarget, error: string | null, errorCode?: string | null) => void;
  setErrorForChatPath: (chatPath: string | null | undefined, error: string | null, errorCode?: string | null) => void;
  resetChatUi: () => void;
  syncThreadSnapshot: (snapshot: ThreadSnapshot) => void;
  mergeThreadSnapshotWindow: (snapshot: ThreadSnapshot, direction: ThreadSnapshotWindowMergeDirection) => void;
  upsertThreadMessageRecords: (records: ChatMessageRecord[]) => void;
  getThreadSnapshot: (chatPath: string | null | undefined) => ThreadSnapshot | null;
  getThreadSnapshotForTarget: (target: ConversationTarget) => ThreadSnapshot | null;
  startStreamingTextBlock: (chatPath: string) => void;
  appendStreamingText: (chatPath: string, chunk: string) => void;
  pushLiveStep: (chatPath: string, step: LiveStep) => void;
  updateLiveStep: (chatPath: string, id: string, patch: LiveStepPatch) => void;
  setActivityExpanded: (chatPath: string, expanded: boolean, options?: { userAction?: boolean }) => void;
  setLiveTokenUsage: (chatPath: string, usage: TokenUsage) => void;
  setAutoRetryState: (
    chatPath: string,
    attempt: number,
    limit: number,
    delayMs: number,
    errorMessage: string | null,
  ) => void;
  clearAutoRetryState: (chatPath: string | null | undefined) => void;
  setReconnectState: (chatPath: string, attempt: number, limit: number, message: string | null) => void;
  clearReconnectState: (chatPath: string | null | undefined) => void;
  patchThreadSnapshotState: (chatPath: string, threadState: Partial<ThreadState>) => void;
  getThreadState: (chatPath: string | null | undefined) => ThreadState;
  getThreadStateForTarget: (target: ConversationTarget) => ThreadState;
  clearLiveOverlay: (chatPath: string) => void;
  prepareLiveOverlayForNewRun: (chatPath: string) => void;
  removeLiveOverlay: (chatPath: string) => void;
  getLiveOverlay: (chatPath: string | null | undefined) => LiveOverlay;
  getLiveOverlayForTarget: (target: ConversationTarget) => LiveOverlay;
  setReviews: (chatPath: string, reviews: ThreadReviewState[]) => void;
  getReviews: (chatPath: string | null | undefined) => ReviewBucket;
  upsertThreadMeta: (meta: ThreadSummary) => void;
  getThreadMeta: (chatPath: string | null | undefined) => ThreadSummary | null;
  getThreadMetaByThreadID: (threadID: string | null | undefined) => ThreadSummary | null;
  getTargetChatPath: (target: ConversationTarget) => string | null;
  bumpPlanRevision: (chatPath: string) => void;
  getPlanRevision: (chatPath: string | null | undefined) => number;
};

type ChatWorkspaceStore = StoreApi<ChatWorkspaceState>;
type ChatWorkspaceHook = {
  <T = ChatWorkspaceState>(selector?: (state: ChatWorkspaceState) => T): T;
  getState: () => ChatWorkspaceState;
  getStateByTabId: (tabId: string) => ChatWorkspaceState;
  getStoreByTabId: (tabId: string) => ChatWorkspaceStore;
};

const chatWorkspaceStores = new Map<string, ChatWorkspaceStore>();

function createChatWorkspaceStore(): ChatWorkspaceStore {
  return createStore<ChatWorkspaceState>((set, get) => ({
    ...INITIAL_CHAT_STATE,
    showComposer: () => set({ composerVisible: true }),
    hideComposer: () => set({ composerVisible: false }),
    setComposerVisible: (composerVisible) => set({ composerVisible }),
    requestComposerFocus: () => set((state) => ({
      composerFocusRequestSeq: state.composerFocusRequestSeq + 1,
    })),
    requestComposerBlockInsert: (markdown) => {
      const insert = createPendingComposerInsert(markdown);
      if (!insert) {
        return null;
      }
      set((state) => ({
        pendingComposerInsertQueue: [...state.pendingComposerInsertQueue, insert],
      }));
      return insert.id;
    },
    consumeComposerBlockInsert: (id) => set((state) => {
      const normalizedID = (id || '').trim();
      if (!normalizedID || state.pendingComposerInsertQueue.length === 0) {
        return {};
      }
      const nextQueue = state.pendingComposerInsertQueue.filter((item) => item.id !== normalizedID);
      if (nextQueue.length === state.pendingComposerInsertQueue.length) {
        return {};
      }
      return { pendingComposerInsertQueue: nextQueue };
    }),

    setTargetChatPath: (targetChatPath) => {
      get().selectChatConversation(targetChatPath);
    },

    selectChatConversation: (path) => {
      const normalizedPath = normalizeChatPath(path);
      const target = resolveConversationTargetForPath(get(), normalizedPath);
      set({
        targetChatPath: get().getTargetChatPath(target),
        selectedConversationTarget: target,
      });
    },

    selectThreadConversation: (threadID, chatPath) => {
      const target = toThreadTarget(threadID, chatPath);
      const normalizedThreadID = (threadID || '').trim();
      const normalizedChatPath = normalizeChatPath(chatPath);
      set({
        targetChatPath: get().getTargetChatPath(target),
        selectedConversationTarget: target,
        ...(normalizedThreadID ? {
          openComposerTargets: upsertOpenComposerTarget(get().openComposerTargets, {
            threadID: normalizedThreadID,
            ...(normalizedChatPath ? { chatPath: normalizedChatPath } : {}),
          }),
        } : {}),
      });
    },

    openThreadConversation: (threadID, options) => {
      const normalizedThreadID = (threadID || '').trim();
      if (!normalizedThreadID) {
        return;
      }
      const normalizedChatPath = normalizeChatPath(options?.chatPath);
      const title = (options?.title || '').trim();
      const agentID = (options?.agentID || '').trim();
      const target = toThreadTarget(normalizedThreadID, normalizedChatPath);
      set((state) => ({
        openComposerTargets: upsertOpenComposerTarget(state.openComposerTargets, {
          threadID: normalizedThreadID,
          ...(normalizedChatPath ? { chatPath: normalizedChatPath } : {}),
          ...(title ? { title } : {}),
          ...(agentID ? { agentID } : {}),
        }),
        targetChatPath: normalizeChatPath(state.threadMetaByThreadID[normalizedThreadID]?.chatPath || normalizedChatPath) || null,
        selectedConversationTarget: target,
        composerVisible: true,
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [normalizedThreadID]: {
            ...clearTransientRecoveryState(state.liveOverlayByThreadID[normalizedThreadID] || createEmptyLiveOverlay()),
            expanded: true,
            userOverride: 'expanded',
          },
        },
      }));
    },

    closeComposerTarget: (threadID) => set((state) => {
      const normalizedThreadID = (threadID || '').trim();
      if (!normalizedThreadID) {
        return {};
      }
      const nextOpen = state.openComposerTargets.filter((item) => item.threadID !== normalizedThreadID);
      const isSelected = state.selectedConversationTarget?.kind === 'thread'
        && state.selectedConversationTarget.threadID === normalizedThreadID;
      return {
        openComposerTargets: nextOpen,
        ...(isSelected ? {
          selectedConversationTarget: null,
          targetChatPath: null,
        } : {}),
      };
    }),

    createPendingConversation: () => {
      let pendingId = '';
      set((state) => {
        const nextPendingState = createPendingTargetState(state);
        pendingId = nextPendingState.pending.id;
        return {
          pendingConversations: nextPendingState.pendingConversations,
          selectedConversationTarget: nextPendingState.target,
          targetChatPath: null,
          modelKeyByTargetKey: nextPendingState.modelKeyByTargetKey,
          agentByTargetKey: nextPendingState.agentByTargetKey,
          error: null,
        };
      });
      return pendingId;
    },

    selectPendingConversation: (id) => set((state) => {
      if (!state.pendingConversations.some((pending) => pending.id === id)) {
        return {};
      }
      return {
        selectedConversationTarget: { kind: 'pending', id },
        targetChatPath: null,
      };
    }),

    removePendingConversation: (id) => set((state) => {
      if (!state.pendingConversations.some((pending) => pending.id === id)) {
        return {};
      }
      const nextDraftByTargetKey = { ...state.draftByTargetKey };
      delete nextDraftByTargetKey[`pending:${id}`];
      const nextComposerPlanStateByTargetKey = { ...state.composerPlanStateByTargetKey };
      delete nextComposerPlanStateByTargetKey[`pending:${id}`];
      const nextModelKeyByTargetKey = { ...state.modelKeyByTargetKey };
      delete nextModelKeyByTargetKey[`pending:${id}`];
      const nextAgentByTargetKey = { ...state.agentByTargetKey };
      delete nextAgentByTargetKey[`pending:${id}`];
      const nextInProgressByTargetKey = { ...state.inProgressByTargetKey };
      delete nextInProgressByTargetKey[`pending:${id}`];
      const isSelectedPending = state.selectedConversationTarget?.kind === 'pending'
        && state.selectedConversationTarget.id === id;
      return {
        pendingConversations: state.pendingConversations.filter((pending) => pending.id !== id),
        draftByTargetKey: nextDraftByTargetKey,
        composerPlanStateByTargetKey: nextComposerPlanStateByTargetKey,
        modelKeyByTargetKey: nextModelKeyByTargetKey,
        agentByTargetKey: nextAgentByTargetKey,
        inProgressByTargetKey: nextInProgressByTargetKey,
        ...(isSelectedPending
          ? {
            selectedConversationTarget: null,
            targetChatPath: null,
          }
          : {}),
      };
    }),

    getDraftForTarget: (target) => {
      const draftKey = getConversationTargetKey(target);
      if (!draftKey) {
        return '';
      }
      return get().draftByTargetKey[draftKey] || '';
    },

    getComposerPlanStateForTarget: (target) => {
      const targetKey = getConversationTargetKey(target);
      if (!targetKey) {
        return null;
      }
      return get().composerPlanStateByTargetKey[targetKey] || null;
    },

    setDraftForTarget: (target, draft) => set((state) => {
      const draftKey = getConversationTargetKey(target);
      if (!draftKey) {
        return {};
      }
      const nextDraftByTargetKey = { ...state.draftByTargetKey };
      if (draft) {
        nextDraftByTargetKey[draftKey] = draft;
      } else {
        delete nextDraftByTargetKey[draftKey];
      }
      return {
        draftByTargetKey: nextDraftByTargetKey,
        error: null,
      };
    }),

    setDraftForSelectedTarget: (draft) => set((state) => {
      let nextSelectedTarget = state.selectedConversationTarget;
      let nextPendingConversations = state.pendingConversations;
      let nextModelKeyByTargetKey = state.modelKeyByTargetKey;
      let nextAgentByTargetKey = state.agentByTargetKey;

      if (!nextSelectedTarget) {
        const nextPendingState = createPendingTargetState(state);
        nextPendingConversations = nextPendingState.pendingConversations;
        nextSelectedTarget = nextPendingState.target;
        nextModelKeyByTargetKey = nextPendingState.modelKeyByTargetKey;
        nextAgentByTargetKey = nextPendingState.agentByTargetKey;
      }

      const draftKey = getConversationTargetKey(nextSelectedTarget);
      if (!draftKey) {
        return {};
      }

      const nextDraftByTargetKey = { ...state.draftByTargetKey };
      if (draft) {
        nextDraftByTargetKey[draftKey] = draft;
      } else {
        delete nextDraftByTargetKey[draftKey];
      }

      return {
        pendingConversations: nextPendingConversations,
        selectedConversationTarget: nextSelectedTarget,
        targetChatPath: get().getTargetChatPath(nextSelectedTarget),
        modelKeyByTargetKey: nextModelKeyByTargetKey,
        agentByTargetKey: nextAgentByTargetKey,
        draftByTargetKey: nextDraftByTargetKey,
        error: null,
      };
    }),

    setComposerPlanStateForTarget: (target, planState) => set((state) => {
      const targetKey = getConversationTargetKey(target);
      if (!targetKey) {
        return {};
      }
      const existing = state.composerPlanStateByTargetKey[targetKey] || null;
      if (areComposerPlanStatesEqual(existing, planState)) {
        return {};
      }
      const nextComposerPlanStateByTargetKey = { ...state.composerPlanStateByTargetKey };
      if (planState) {
        nextComposerPlanStateByTargetKey[targetKey] = {
          anchor: planState.anchor,
          beforeSpacer: planState.beforeSpacer ? { ...planState.beforeSpacer } : null,
          afterSpacer: planState.afterSpacer ? { ...planState.afterSpacer } : null,
        };
      } else {
        delete nextComposerPlanStateByTargetKey[targetKey];
      }
      return {
        composerPlanStateByTargetKey: nextComposerPlanStateByTargetKey,
      };
    }),

    setComposerPlanStateForSelectedTarget: (planState) => {
      get().setComposerPlanStateForTarget(get().selectedConversationTarget, planState);
    },

    clearComposerPlanStateForTarget: (target) => {
      get().setComposerPlanStateForTarget(target, null);
    },

    clearComposerPlanStateForSelectedTarget: () => {
      get().setComposerPlanStateForTarget(get().selectedConversationTarget, null);
    },

    consumePendingConversation: (pendingId, chatPath, threadID) => {
      const target = toThreadTarget(threadID, chatPath) || toCommandTarget(chatPath);
      if (!target) {
        return;
      }
      set((state) => {
        const pendingDraftKey = `pending:${pendingId}`;
        const chatDraftKey = getConversationTargetKey(target);
        if (!chatDraftKey) {
          return {};
        }
        const nextDraftByTargetKey = { ...state.draftByTargetKey };
        if (pendingDraftKey in nextDraftByTargetKey) {
          nextDraftByTargetKey[chatDraftKey] = nextDraftByTargetKey[pendingDraftKey];
          delete nextDraftByTargetKey[pendingDraftKey];
        }
        const nextComposerPlanStateByTargetKey = { ...state.composerPlanStateByTargetKey };
        if (pendingDraftKey in nextComposerPlanStateByTargetKey) {
          nextComposerPlanStateByTargetKey[chatDraftKey] = nextComposerPlanStateByTargetKey[pendingDraftKey];
          delete nextComposerPlanStateByTargetKey[pendingDraftKey];
        }
        const nextModelKeyByTargetKey = { ...state.modelKeyByTargetKey };
        const pendingModelKey = nextModelKeyByTargetKey[pendingDraftKey];
        delete nextModelKeyByTargetKey[pendingDraftKey];
        if (pendingModelKey) {
          nextModelKeyByTargetKey[chatDraftKey] = pendingModelKey;
        }
        const nextAgentByTargetKey = { ...state.agentByTargetKey };
        const pendingAgent = nextAgentByTargetKey[pendingDraftKey];
        delete nextAgentByTargetKey[pendingDraftKey];
        if (pendingAgent) {
          nextAgentByTargetKey[chatDraftKey] = pendingAgent;
        }
        const nextInProgressByTargetKey = { ...state.inProgressByTargetKey };
        const pendingProgressKey = pendingDraftKey;
        const chatProgressKey = chatDraftKey;
        if (nextInProgressByTargetKey[pendingProgressKey]) {
          delete nextInProgressByTargetKey[pendingProgressKey];
          nextInProgressByTargetKey[chatProgressKey] = true;
        }
        return {
          pendingConversations: state.pendingConversations.filter((pending) => pending.id !== pendingId),
          draftByTargetKey: nextDraftByTargetKey,
          composerPlanStateByTargetKey: nextComposerPlanStateByTargetKey,
          modelKeyByTargetKey: nextModelKeyByTargetKey,
          agentByTargetKey: nextAgentByTargetKey,
          inProgressByTargetKey: nextInProgressByTargetKey,
          selectedConversationTarget: target,
          targetChatPath: get().getTargetChatPath(target),
        };
      });
    },

    setInputMode: (inputMode) => set({ inputMode }),
    setAgentInfo: (agentID, agentName, agentCwd) => set(() => {
      const normalizedAgent = normalizeSelectedAgentTarget({ agentID: agentID || '', agentName, agentCwd: agentCwd || '' });
      return {
        agentID: normalizedAgent?.agentID || null,
        agentName: normalizedAgent?.agentName || null,
        agentCwd: normalizedAgent?.agentCwd || null,
      };
    }),
    getAgentForTarget: (target) => {
      const targetKey = getConversationTargetKey(target);
      if (!targetKey) {
        return null;
      }
      return get().agentByTargetKey[targetKey] || null;
    },
    getSelectedAgent: () => get().getAgentForTarget(get().selectedConversationTarget),
    setAgentForTarget: (target, agent) => set((state) => {
      const targetKey = getConversationTargetKey(target);
      if (!targetKey) {
        return {};
      }
      const normalizedAgent = normalizeSelectedAgentTarget(agent);
      const nextAgentByTargetKey = { ...state.agentByTargetKey };
      if (normalizedAgent) {
        nextAgentByTargetKey[targetKey] = normalizedAgent;
      } else {
        delete nextAgentByTargetKey[targetKey];
      }
      return { agentByTargetKey: nextAgentByTargetKey };
    }),
    setAgentForSelectedTarget: (agent) => set((state) => {
      let nextSelectedTarget = state.selectedConversationTarget;
      let nextPendingConversations = state.pendingConversations;
      let nextModelKeyByTargetKey = state.modelKeyByTargetKey;
      let nextAgentByTargetKey = state.agentByTargetKey;
      const normalizedAgent = normalizeSelectedAgentTarget(agent);
      if (!nextSelectedTarget) {
        if (!normalizedAgent) {
          return {};
        }
        const nextPendingState = createPendingTargetState(state);
        nextPendingConversations = nextPendingState.pendingConversations;
        nextSelectedTarget = nextPendingState.target;
        nextModelKeyByTargetKey = nextPendingState.modelKeyByTargetKey;
        nextAgentByTargetKey = nextPendingState.agentByTargetKey;
      }
      const targetKey = getConversationTargetKey(nextSelectedTarget);
      if (!targetKey) {
        return {};
      }
      nextAgentByTargetKey = { ...nextAgentByTargetKey };
      if (normalizedAgent) {
        nextAgentByTargetKey[targetKey] = normalizedAgent;
      } else {
        delete nextAgentByTargetKey[targetKey];
      }
      return {
        pendingConversations: nextPendingConversations,
        selectedConversationTarget: nextSelectedTarget,
        targetChatPath: get().getTargetChatPath(nextSelectedTarget),
        modelKeyByTargetKey: nextModelKeyByTargetKey,
        agentByTargetKey: nextAgentByTargetKey,
      };
    }),
    setGBrainQueryScope: (gbrainQueryScope) => set({ gbrainQueryScope }),
    clearGBrainQueryScope: () => set({ gbrainQueryScope: null }),
    getQueuedMessages: (chatPath) => {
      const key = resolveChatStateKey(get(), chatPath);
      if (!key) {
        return EMPTY_QUEUED_MESSAGES;
      }
      return get().queuedMessagesByThreadID[key] || EMPTY_QUEUED_MESSAGES;
    },
    getQueuedMessagesForTarget: (target) => {
      const key = resolveTargetStateKey(get(), target);
      if (!key || target?.kind !== 'thread') {
        return EMPTY_QUEUED_MESSAGES;
      }
      return get().queuedMessagesByThreadID[key] || EMPTY_QUEUED_MESSAGES;
    },
    syncQueuedMessages: (chatPath, queuedMessages) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const nextBucket = cloneQueuedMessageBucket(queuedMessages);
      const isEmpty = nextBucket.steering.length === 0 && nextBucket.followUp.length === 0;
      const nextQueuedMessagesByThreadID = { ...state.queuedMessagesByThreadID };
      if (isEmpty) {
        if (!nextQueuedMessagesByThreadID[key]) {
          return {};
        }
        delete nextQueuedMessagesByThreadID[key];
        return { queuedMessagesByThreadID: nextQueuedMessagesByThreadID };
      }
      return {
        queuedMessagesByThreadID: {
          ...state.queuedMessagesByThreadID,
          [key]: nextBucket,
        }
      };
    }),
    appendQueuedMessage: (chatPath, message) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key || !message.id.trim() || !message.text.trim()) {
        return {};
      }
      const prev = state.queuedMessagesByThreadID[key] || EMPTY_QUEUED_MESSAGES;
      const nextMessage = cloneQueuedMessage(message, { chatPath: normalizeChatPath(message.chatPath || chatPath) });
      const nextBucket: QueuedMessageBucket = {
        steering: message.kind === 'steering'
          ? [...prev.steering.filter((queued) => queued.id !== message.id), nextMessage]
          : [...prev.steering],
        followUp: message.kind === 'follow_up'
          ? [...prev.followUp.filter((queued) => queued.id !== message.id), nextMessage]
          : [...prev.followUp],
      };
      return {
        queuedMessagesByThreadID: {
          ...state.queuedMessagesByThreadID,
          [key]: nextBucket,
        },
      };
    }),
    removeQueuedMessage: (chatPath, itemID) => set((state) => {
      const normalizedPath = resolveChatStateKey(state, chatPath);
      const trimmedItemID = (itemID || '').trim();
      const existing = normalizedPath ? state.queuedMessagesByThreadID[normalizedPath] : null;
      if (!existing || !trimmedItemID) {
        return {};
      }
      const nextBucket: QueuedMessageBucket = {
        steering: existing.steering.filter((message) => message.id !== trimmedItemID),
        followUp: existing.followUp.filter((message) => message.id !== trimmedItemID),
      };
      const changed = nextBucket.steering.length !== existing.steering.length
        || nextBucket.followUp.length !== existing.followUp.length;
      if (!changed) {
        return {};
      }
      const nextQueuedMessagesByThreadID = { ...state.queuedMessagesByThreadID };
      if (nextBucket.steering.length === 0 && nextBucket.followUp.length === 0) {
        delete nextQueuedMessagesByThreadID[normalizedPath];
      } else {
        nextQueuedMessagesByThreadID[normalizedPath] = nextBucket;
      }
      return {
        queuedMessagesByThreadID: nextQueuedMessagesByThreadID,
      };
    }),
    clearQueuedMessages: (chatPath) => set((state) => {
      const normalizedPath = resolveChatStateKey(state, chatPath);
      if (!normalizedPath || !state.queuedMessagesByThreadID[normalizedPath]) {
        return {};
      }
      const nextQueuedMessagesByThreadID = { ...state.queuedMessagesByThreadID };
      delete nextQueuedMessagesByThreadID[normalizedPath];
      return { queuedMessagesByThreadID: nextQueuedMessagesByThreadID };
    }),
    clearAllQueuedMessages: () => set({ queuedMessagesByThreadID: {} }),
    setSelectedSkill: (selectedSkill) => set((state) => {
      const normalizedSelectedSkill = selectedSkill ? { ...selectedSkill } : null;
      if (normalizedSelectedSkill?.slug === PLAN_SKILL_SLUG) {
        return {
          selectedSkill: normalizedSelectedSkill,
        };
      }
      if (Object.keys(state.composerPlanStateByTargetKey).length === 0) {
        return {
          selectedSkill: normalizedSelectedSkill,
        };
      }
      return {
        selectedSkill: normalizedSelectedSkill,
        draftByTargetKey: clearAllComposerPlanDraftSpacers(
          state.draftByTargetKey,
          state.composerPlanStateByTargetKey,
        ),
        composerPlanStateByTargetKey: {},
      };
    }),
    clearSelectedSkill: () => set((state) => {
      if (Object.keys(state.composerPlanStateByTargetKey).length === 0) {
        return { selectedSkill: null };
      }
      return {
        selectedSkill: null,
        draftByTargetKey: clearAllComposerPlanDraftSpacers(
          state.draftByTargetKey,
          state.composerPlanStateByTargetKey,
        ),
        composerPlanStateByTargetKey: {},
      };
    }),
    setRememberedModelKey: (rememberedModelKey) => set({
      rememberedModelKey: normalizeModelKey(rememberedModelKey) || null,
    }),
    getSelectedModelKey: () => {
      return get().getModelKeyForTarget(get().selectedConversationTarget);
    },
    getModelKeyForTarget: (target) => {
      const draftKey = getConversationTargetKey(target);
      if (!draftKey) {
        return null;
      }
      return get().modelKeyByTargetKey[draftKey] || null;
    },
    setSelectedModelKey: (selectedModelKey) => set((state) => {
      const normalizedSelectedModelKey = normalizeModelKey(selectedModelKey) || null;
      let nextSelectedTarget = state.selectedConversationTarget;
      let nextPendingConversations = state.pendingConversations;
      let nextModelKeyByTargetKey = { ...state.modelKeyByTargetKey };
      let nextAgentByTargetKey = state.agentByTargetKey;

      let draftKey = getConversationTargetKey(nextSelectedTarget);
      if (!draftKey) {
        if (!normalizedSelectedModelKey) {
          return {};
        }
        const created = createPendingTargetState(state);
        nextSelectedTarget = created.target;
        nextPendingConversations = created.pendingConversations;
        nextModelKeyByTargetKey = { ...created.modelKeyByTargetKey };
        nextAgentByTargetKey = created.agentByTargetKey;
        draftKey = getConversationTargetKey(nextSelectedTarget);
      }
      if (!draftKey) {
        return {};
      }
      if (normalizedSelectedModelKey) {
        nextModelKeyByTargetKey[draftKey] = normalizedSelectedModelKey;
      } else {
        delete nextModelKeyByTargetKey[draftKey];
      }
      return {
        pendingConversations: nextPendingConversations,
        selectedConversationTarget: nextSelectedTarget,
        modelKeyByTargetKey: nextModelKeyByTargetKey,
        agentByTargetKey: nextAgentByTargetKey,
      };
    }),
    syncChatSettings: (path, settings) => set((state) => {
      const normalizedPath = normalizeChatPath(path);
      const target = resolveConversationTargetForPath(state, normalizedPath);
      const draftKey = getConversationTargetKey(target);
      if (!draftKey) {
        return {};
      }
      const nextModelKeyByTargetKey = { ...state.modelKeyByTargetKey };
      if (hasOwnSetting(settings, 'modelKey')) {
        const normalizedModelKey = normalizeModelKey(settings.modelKey) || null;
        if (normalizedModelKey) {
          nextModelKeyByTargetKey[draftKey] = normalizedModelKey;
        } else {
          delete nextModelKeyByTargetKey[draftKey];
        }
      }
      return {
        modelKeyByTargetKey: nextModelKeyByTargetKey,
      };
    }),
    retargetChatPath: (oldPath, newPath) => set((state) => {
      const from = normalizeChatPath(oldPath);
      const to = normalizeChatPath(newPath);
      if (!from || !to || from === to) {
        return {};
      }
      const nextState = retargetChatSnapshot({
        draftByTargetKey: state.draftByTargetKey,
        composerPlanStateByTargetKey: state.composerPlanStateByTargetKey,
        modelKeyByTargetKey: state.modelKeyByTargetKey,
        agentByTargetKey: state.agentByTargetKey,
        threadSnapshotByID: state.threadSnapshotByID,
        liveOverlayByThreadID: state.liveOverlayByThreadID,
        targetChatPath: state.targetChatPath,
        selectedConversationTarget: state.selectedConversationTarget,
      }, from, to);
      const nextPathToThreadID = { ...state.pathToThreadID };
      const threadID = (nextPathToThreadID[from] || '').trim();
      if (threadID) {
        nextPathToThreadID[to] = threadID;
        delete nextPathToThreadID[from];
      }
      const nextReviewByThreadID = { ...state.reviewByThreadID };
      const nextQueuedMessagesByThreadID = { ...state.queuedMessagesByThreadID };
      const stateKeyFrom = resolveChatStateKey(state, from);
      const stateKeyTo = threadID || to;
      if (Object.prototype.hasOwnProperty.call(nextQueuedMessagesByThreadID, stateKeyFrom)) {
        const queuedBucket = nextQueuedMessagesByThreadID[stateKeyFrom];
        nextQueuedMessagesByThreadID[stateKeyTo] = {
          steering: queuedBucket.steering.map((message) => cloneQueuedMessage(message, { chatPath: to })),
          followUp: queuedBucket.followUp.map((message) => cloneQueuedMessage(message, { chatPath: to })),
        };
        if (stateKeyFrom !== stateKeyTo) {
          delete nextQueuedMessagesByThreadID[stateKeyFrom];
        }
      }
      const nextAwaitingUserByThreadID = { ...state.awaitingUserByThreadID };
      if (Object.prototype.hasOwnProperty.call(nextAwaitingUserByThreadID, stateKeyFrom)) {
        nextAwaitingUserByThreadID[stateKeyTo] = nextAwaitingUserByThreadID[stateKeyFrom];
        if (stateKeyFrom !== stateKeyTo) {
          delete nextAwaitingUserByThreadID[stateKeyFrom];
        }
      }
      const nextThreadMetaByThreadID = { ...state.threadMetaByThreadID };
      if (threadID && Object.prototype.hasOwnProperty.call(nextThreadMetaByThreadID, threadID)) {
        nextThreadMetaByThreadID[threadID] = {
          ...nextThreadMetaByThreadID[threadID],
          chatPath: to,
        };
      }
      const nextThreadSnapshotByID = { ...nextState.threadSnapshotByID };
      if (threadID && nextThreadSnapshotByID[threadID]) {
        nextThreadSnapshotByID[threadID] = {
          ...nextThreadSnapshotByID[threadID],
          meta: {
            ...nextThreadSnapshotByID[threadID].meta,
            chatPath: to,
            path: to,
          },
        };
      }
      const nextPlanRevisionByThreadID = { ...state.planRevisionByThreadID };
      if (Object.prototype.hasOwnProperty.call(nextPlanRevisionByThreadID, stateKeyFrom)) {
        nextPlanRevisionByThreadID[stateKeyTo] = nextPlanRevisionByThreadID[stateKeyFrom];
        if (stateKeyFrom !== stateKeyTo) {
          delete nextPlanRevisionByThreadID[stateKeyFrom];
        }
      }
      const nextPendingScrollToBottomByThreadID = { ...state.pendingScrollToBottomByThreadID };
      if (Object.prototype.hasOwnProperty.call(nextPendingScrollToBottomByThreadID, stateKeyFrom)) {
        nextPendingScrollToBottomByThreadID[stateKeyTo] = true;
        if (stateKeyFrom !== stateKeyTo) {
          delete nextPendingScrollToBottomByThreadID[stateKeyFrom];
        }
      }
	      const nextInProgressByTargetKey = { ...state.inProgressByTargetKey };
	      const fromProgressKey = threadID ? `thread:${threadID}` : `command:${from}`;
	      const toProgressKey = threadID ? `thread:${threadID}` : `command:${to}`;
	      if (fromProgressKey !== toProgressKey && nextInProgressByTargetKey[fromProgressKey]) {
	        delete nextInProgressByTargetKey[fromProgressKey];
	        nextInProgressByTargetKey[toProgressKey] = true;
	      }
      return {
        draftByTargetKey: nextState.draftByTargetKey,
        composerPlanStateByTargetKey: nextState.composerPlanStateByTargetKey,
        modelKeyByTargetKey: nextState.modelKeyByTargetKey,
        agentByTargetKey: nextState.agentByTargetKey,
        threadSnapshotByID: nextThreadSnapshotByID,
        queuedMessagesByThreadID: nextQueuedMessagesByThreadID,
        awaitingUserByThreadID: nextAwaitingUserByThreadID,
        inProgressByTargetKey: nextInProgressByTargetKey,
        liveOverlayByThreadID: nextState.liveOverlayByThreadID,
        reviewByThreadID: nextReviewByThreadID,
        threadMetaByThreadID: nextThreadMetaByThreadID,
        pathToThreadID: nextPathToThreadID,
        planRevisionByThreadID: nextPlanRevisionByThreadID,
        pendingScrollToBottomByThreadID: nextPendingScrollToBottomByThreadID,
        targetChatPath: nextState.targetChatPath,
        selectedConversationTarget: nextState.selectedConversationTarget,
        activeCommand: state.activeCommand?.filePath === from
          ? { ...state.activeCommand, filePath: to }
          : state.activeCommand,
      };
	    }),
	    setActiveCommand: (activeCommand) => set({ activeCommand }),
    isTargetInProgress: (target) => {
      const targetKey = getConversationTargetKey(target);
      return targetKey ? Boolean(get().inProgressByTargetKey[targetKey]) : false;
    },
    getConversationRunStatus: (target) => {
      if (!target) {
        return 'idle';
      }
      if (target.kind === 'pending') {
        return 'idle';
      }
      const key = resolveTargetStateKey(get(), target);
      if (key && get().awaitingUserByThreadID[key]) {
        return 'awaiting_user';
      }
      if (get().isTargetInProgress(target)) {
        return 'running';
      }
      const threadState = key ? getThreadStateFromSnapshot(get().threadSnapshotByID[key] || null) : EMPTY_THREAD_STATE;
      if (threadState.runStatus === 'idle' && threadState.tailStatus === 'complete') {
        return 'complete';
      }
      return 'idle';
    },
    hasAnyInProgress: () => Object.keys(get().inProgressByTargetKey).length > 0,
    setTargetInProgress: (target, inProgress) => set((state) => {
      const targetKey = getConversationTargetKey(target);
      if (!targetKey) {
        return {};
      }
      const nextInProgressByTargetKey = { ...state.inProgressByTargetKey };
      if (inProgress) {
        nextInProgressByTargetKey[targetKey] = true;
      } else if (nextInProgressByTargetKey[targetKey]) {
        delete nextInProgressByTargetKey[targetKey];
      } else {
        return {};
      }
      return { inProgressByTargetKey: nextInProgressByTargetKey };
    }),
    setChatPathInProgress: (chatPath, inProgress) => {
      get().setTargetInProgress(resolveConversationTargetForPath(get(), chatPath), inProgress);
    },
    requestChatScrollToBottom: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key || state.pendingScrollToBottomByThreadID[key]) {
        return {};
      }
      return {
        pendingScrollToBottomByThreadID: {
          ...state.pendingScrollToBottomByThreadID,
          [key]: true,
        },
      };
    }),
    consumeChatScrollToBottom: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key || !state.pendingScrollToBottomByThreadID[key]) {
        return {};
      }
      const nextPendingScrollToBottomByThreadID = { ...state.pendingScrollToBottomByThreadID };
      delete nextPendingScrollToBottomByThreadID[key];
      return {
        pendingScrollToBottomByThreadID: nextPendingScrollToBottomByThreadID,
      };
    }),
    shouldScrollChatToBottom: (chatPath) => {
      const key = resolveChatStateKey(get(), chatPath);
      return key ? Boolean(get().pendingScrollToBottomByThreadID[key]) : false;
    },
    clearAllInProgress: () => set({ inProgressByTargetKey: {} }),
    setAwaitingUser: (chatPath, awaiting) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const nextAwaitingUserByThreadID = { ...state.awaitingUserByThreadID };
      if (awaiting) {
        const requestID = (awaiting.requestID || '').trim();
        const questions = Array.isArray(awaiting.questions)
          ? awaiting.questions
            .map((question, index) => {
              const header = (question?.header || '').trim();
              const text = (question?.question || '').trim();
              const options = Array.isArray(question?.options)
                ? question.options
                  .map((option) => ({
                    label: (option?.label || '').trim(),
                    description: (option?.description || '').trim() || undefined,
                  }))
                  .filter((option) => option.label)
                : [];
              if (!text) {
                return null;
              }
              return {
                header: header || `Q${index + 1}`,
                question: text,
                options,
                multiple: question?.multiple ? true : undefined,
                custom: question?.custom === false ? false : true,
              };
            })
            .filter((question): question is NonNullable<typeof question> => Boolean(question))
          : [];
        if (!requestID || questions.length === 0) {
          return {};
        }
        const answers = Array.isArray(awaiting.answers)
          ? questions.map((_, index) => {
            const current = Array.isArray(awaiting.answers[index]) ? awaiting.answers[index] : [];
            return current
              .map((answer) => (typeof answer === 'string' ? answer : ''))
              .filter((answer) => answer.trim().length > 0);
          })
          : questions.map(() => []);
        const customModeByIndex = Array.isArray(awaiting.customModeByIndex)
          ? questions.map((_, index) => awaiting.customModeByIndex[index] === true)
          : questions.map(() => false);
        const maxIndex = questions.length > 0 ? questions.length - 1 : 0;
        const currentIndex = Number.isFinite(awaiting.currentIndex)
          ? Math.min(maxIndex, Math.max(0, Math.trunc(awaiting.currentIndex)))
          : 0;
        nextAwaitingUserByThreadID[key] = {
          questions,
          currentIndex,
          answers,
          customModeByIndex,
          requestID,
          requestedAt: awaiting.requestedAt,
        };
      } else if (nextAwaitingUserByThreadID[key]) {
        delete nextAwaitingUserByThreadID[key];
      } else {
        return {};
      }
      return {
        awaitingUserByThreadID: nextAwaitingUserByThreadID,
      };
    }),
    clearAwaitingUser: (chatPath) => {
      get().setAwaitingUser(normalizeChatPath(chatPath), null);
    },
    clearAllAwaitingUsers: () => set((state) => (
      Object.keys(state.awaitingUserByThreadID).length > 0
        ? { awaitingUserByThreadID: {} }
        : {}
    )),
    getAwaitingUser: (chatPath) => {
      const key = resolveChatStateKey(get(), chatPath);
      if (!key) {
        return null;
      }
      return get().awaitingUserByThreadID[key] || null;
    },
    setError: (error, errorCode) => {
      get().setErrorForTarget(get().selectedConversationTarget, error, errorCode);
    },
    setErrorForChatPath: (chatPath, error, errorCode) => {
      get().setErrorForTarget(resolveConversationTargetForPath(get(), chatPath), error, errorCode);
    },
    setErrorForTarget: (target, error, errorCode) => set((state) => {
      const normalizedError = (error || '').trim() || null;
      const normalizedErrorCode = normalizedError ? normalizeErrorCode(errorCode) : null;
      const activityKey = getActivityTargetKey(target);
      if (!activityKey) {
        return { error: normalizedError };
      }
      const prev = state.liveOverlayByThreadID[activityKey] || createEmptyLiveOverlay();
      return {
        error: normalizedError,
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [activityKey]: {
            ...clearTransientRecoveryState(prev),
            errorMessage: normalizedError,
            errorCode: normalizedErrorCode,
          },
        },
      };
    }),
    resetChatUi: () => set({
      ...INITIAL_CHAT_STATE,
      composerVisible: get().composerVisible,
      rememberedModelKey: get().rememberedModelKey,
    }),

    syncThreadSnapshot: (snapshot) => set((state) => {
      const cloned = cloneThreadSnapshot(snapshot);
      if (!cloned) {
        return {};
      }
      const threadID = cloned.meta.threadID.trim();
      const storedSnapshot = mergeThreadSnapshotWindowForStore(
        state.threadSnapshotByID[threadID] || null,
        cloned,
        'tail',
      );
      const chatPath = normalizeChatPath(cloned.meta.chatPath || cloned.meta.path);
      const nextPathToThreadID = { ...state.pathToThreadID };
      if (chatPath) {
        nextPathToThreadID[chatPath] = threadID;
      }
      const threadTargetKey = `thread:${threadID}`;
      const metaDefaultAgent = normalizeSelectedAgentTarget({
        agentID: cloned.meta.agentID,
        agentName: null,
        agentCwd: cloned.meta.cwd,
      });
      const nextAgentByTargetKey = metaDefaultAgent && !state.agentByTargetKey[threadTargetKey]
        ? {
          ...state.agentByTargetKey,
          [threadTargetKey]: metaDefaultAgent,
        }
        : state.agentByTargetKey;
      const selected = state.selectedConversationTarget;
      const nextSelectedTarget = selected?.kind === 'thread' && selected.threadID === threadID
        ? ({ kind: 'thread', threadID, ...(chatPath ? { chatPath } : {}) } as const)
        : selected;
      const nextTargetChatPath = chatPath && nextSelectedTarget?.kind === 'thread' && nextSelectedTarget.threadID === threadID
        ? chatPath
        : state.targetChatPath;

      let nextLiveOverlayByThreadID = state.liveOverlayByThreadID;
      const overlayFromPath = chatPath ? state.liveOverlayByThreadID[chatPath] : undefined;
      if (overlayFromPath && chatPath !== threadID) {
        nextLiveOverlayByThreadID = moveRecordValue(state.liveOverlayByThreadID, chatPath, threadID);
      }
      const overlay = nextLiveOverlayByThreadID[threadID];
      if (overlay) {
        nextLiveOverlayByThreadID = {
          ...nextLiveOverlayByThreadID,
          [threadID]: clearSettledLiveOverlayAfterSnapshot(overlay, cloned),
        };
      }

      return {
        pathToThreadID: nextPathToThreadID,
        selectedConversationTarget: nextSelectedTarget,
        targetChatPath: nextTargetChatPath,
        agentByTargetKey: nextAgentByTargetKey,
        openComposerTargets: upsertOpenComposerTarget(state.openComposerTargets, {
          threadID,
          ...(chatPath ? { chatPath } : {}),
          title: cloned.meta.title,
          agentID: cloned.meta.agentID,
        }),
        threadMetaByThreadID: {
          ...state.threadMetaByThreadID,
          [threadID]: {
            ...cloned.meta,
            ...(chatPath ? { chatPath, path: chatPath } : { chatPath: '', path: cloned.meta.path || '' }),
          },
        },
        threadSnapshotByID: {
          ...state.threadSnapshotByID,
          [threadID]: storedSnapshot,
        },
        liveOverlayByThreadID: nextLiveOverlayByThreadID,
      };
    }),

    mergeThreadSnapshotWindow: (snapshot, direction) => set((state) => {
      const cloned = cloneThreadSnapshot(snapshot);
      if (!cloned) {
        return {};
      }
      const threadID = cloned.meta.threadID.trim();
      const chatPath = normalizeChatPath(cloned.meta.chatPath || cloned.meta.path);
      const nextPathToThreadID = { ...state.pathToThreadID };
      if (chatPath) {
        nextPathToThreadID[chatPath] = threadID;
      }
      const threadTargetKey = `thread:${threadID}`;
      const metaDefaultAgent = normalizeSelectedAgentTarget({
        agentID: cloned.meta.agentID,
        agentName: null,
        agentCwd: cloned.meta.cwd,
      });
      const nextAgentByTargetKey = metaDefaultAgent && !state.agentByTargetKey[threadTargetKey]
        ? {
          ...state.agentByTargetKey,
          [threadTargetKey]: metaDefaultAgent,
        }
        : state.agentByTargetKey;
      const selected = state.selectedConversationTarget;
      const nextSelectedTarget = selected?.kind === 'thread' && selected.threadID === threadID
        ? ({ kind: 'thread', threadID, ...(chatPath ? { chatPath } : {}) } as const)
        : selected;
      const nextTargetChatPath = chatPath && nextSelectedTarget?.kind === 'thread' && nextSelectedTarget.threadID === threadID
        ? chatPath
        : state.targetChatPath;
      const merged = mergeThreadSnapshotWindowForStore(
        state.threadSnapshotByID[threadID] || null,
        cloned,
        direction,
      );
      return {
        pathToThreadID: nextPathToThreadID,
        selectedConversationTarget: nextSelectedTarget,
        targetChatPath: nextTargetChatPath,
        agentByTargetKey: nextAgentByTargetKey,
        openComposerTargets: upsertOpenComposerTarget(state.openComposerTargets, {
          threadID,
          ...(chatPath ? { chatPath } : {}),
          title: cloned.meta.title,
          agentID: cloned.meta.agentID,
        }),
        threadMetaByThreadID: {
          ...state.threadMetaByThreadID,
          [threadID]: {
            ...cloned.meta,
            ...(chatPath ? { chatPath, path: chatPath } : { chatPath: '', path: cloned.meta.path || '' }),
          },
        },
        threadSnapshotByID: {
          ...state.threadSnapshotByID,
          [threadID]: merged,
        },
      };
    }),

    upsertThreadMessageRecords: (records) => set((state) => {
      const recordsByThreadID = records.reduce<Record<string, ChatMessageRecord[]>>((acc, record) => {
        const threadID = (record?.threadID || '').trim();
        const id = (record?.id || '').trim();
        if (!threadID || !id) {
          return acc;
        }
        acc[threadID] = acc[threadID] || [];
        acc[threadID].push(record);
        return acc;
      }, {});
      const affectedThreadIDs = Object.keys(recordsByThreadID).filter((threadID) => state.threadSnapshotByID[threadID]);
      if (affectedThreadIDs.length === 0) {
        return {};
      }
      const threadSnapshotByID = { ...state.threadSnapshotByID };
      for (const threadID of affectedThreadIDs) {
        threadSnapshotByID[threadID] = upsertMessageRecordsForSnapshot(
          threadSnapshotByID[threadID],
          recordsByThreadID[threadID],
        );
      }
      return { threadSnapshotByID };
    }),

    getThreadSnapshot: (chatPath) => {
      const key = resolveChatStateKey(get(), chatPath);
      if (!key) {
        return null;
      }
      return get().threadSnapshotByID[key] || null;
    },

    getThreadSnapshotForTarget: (target) => {
      const key = resolveTargetStateKey(get(), target);
      if (!key) {
        return null;
      }
      return get().threadSnapshotByID[key] || null;
    },

    startStreamingTextBlock: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key] || createEmptyLiveOverlay();
      if (prev.activeStreamingSegmentID == null) {
        return {};
      }
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: {
            ...prev,
            activeStreamingSegmentID: null,
          },
        },
      };
    }),

    appendStreamingText: (chatPath, chunk) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key || !chunk) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key] || createEmptyLiveOverlay();
      const now = Date.now();
      const streamingSegments = prev.streamingSegments || [];
      const lastSegment = streamingSegments[streamingSegments.length - 1];
      const lastStepOrder = prev.steps.reduce((max, step) => Math.max(max, step.order ?? 0), 0);
      const canAppendLastSegment = Boolean(
        lastSegment
        && lastSegment.id === prev.activeStreamingSegmentID
        && lastSegment.order > lastStepOrder
      );
      const nextSegmentID = `stream-${now}-${streamingSegments.length + 1}`;
      const nextStreamingSegments = canAppendLastSegment
        ? streamingSegments.map((segment) => (
          segment.id === lastSegment.id
            ? { ...segment, text: segment.text + chunk }
            : segment
        ))
        : [
          ...streamingSegments,
          {
            id: nextSegmentID,
            text: chunk,
            ts: now,
            order: nextActivityTimelineOrder(),
          },
        ];
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: {
            ...clearTransientRecoveryState(prev),
            errorMessage: null,
            errorCode: null,
            streamingText: prev.streamingText + chunk,
            streamingStartedAt: prev.streamingStartedAt ?? now,
            streamingSegments: nextStreamingSegments,
            activeStreamingSegmentID: canAppendLastSegment ? lastSegment.id : nextSegmentID,
          },
        },
      };
    }),

    pushLiveStep: (chatPath, step) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key] || createEmptyLiveOverlay();
      const nextStep: LiveStep = {
        ...step,
        order: step.order ?? nextActivityTimelineOrder(),
      };
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: {
            ...clearTransientRecoveryState(prev),
            errorMessage: null,
            errorCode: null,
            steps: [...prev.steps, nextStep],
            activeStreamingSegmentID: null,
          },
        },
      };
    }),

    updateLiveStep: (chatPath, id, patch) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key];
      if (!prev) {
        return {};
      }
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: {
            ...clearTransientRecoveryState(prev),
            steps: prev.steps.map((step) => (step.id === id ? { ...step, ...patch } : step)),
          },
        },
      };
    }),

    setActivityExpanded: (chatPath, expanded, options) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key] || createEmptyLiveOverlay();
      const userAction = options?.userAction ?? false;
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: {
            ...clearTransientRecoveryState(prev),
            expanded,
            userOverride: userAction
              ? (expanded ? 'expanded' : 'collapsed')
              : prev.userOverride,
          },
        },
      };
    }),

    setLiveTokenUsage: (chatPath, usage) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key] || createEmptyLiveOverlay();
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: {
            ...clearTransientRecoveryState(prev),
            loopUsage: usage,
          },
        },
      };
    }),

    setAutoRetryState: (chatPath, attempt, limit, delayMs, errorMessage) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key] || createEmptyLiveOverlay();
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: {
            ...clearReconnectState(clearTransientAttemptArtifacts(prev)),
            autoRetryActive: true,
            autoRetryAttempt: Math.max(0, attempt || 0),
            autoRetryLimit: Math.max(0, limit || 0),
            autoRetryDelayMs: Math.max(0, delayMs || 0),
            autoRetryStartedAt: Date.now(),
            autoRetryErrorMessage: (errorMessage || '').trim() || null,
          },
        },
      };
    }),

    clearAutoRetryState: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key];
      if (!prev) {
        return {};
      }
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: clearAutoRetryState(prev),
        },
      };
    }),

    setReconnectState: (chatPath, attempt, limit, message) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key] || createEmptyLiveOverlay();
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: {
            ...prev,
            reconnectAttempt: Math.max(0, attempt || 0),
            reconnectLimit: Math.max(0, limit || 0),
            reconnectingMessage: (message || '').trim() || null,
          },
        },
      };
    }),

    clearReconnectState: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key];
      if (!prev) {
        return {};
      }
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: clearReconnectState(prev),
        },
      };
    }),

    patchThreadSnapshotState: (chatPath, threadState) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.threadSnapshotByID[key]
        || createEmptyThreadSnapshot(key, state.threadMetaByThreadID[key]);
      return {
        threadSnapshotByID: {
          ...state.threadSnapshotByID,
          [key]: patchSnapshotThreadState(prev, threadState),
        },
      };
    }),

    getThreadState: (chatPath) => {
      const key = resolveChatStateKey(get(), chatPath);
      if (!key) {
        return EMPTY_THREAD_STATE;
      }
      return getThreadStateFromSnapshot(get().threadSnapshotByID[key] || null);
    },
    getThreadStateForTarget: (target) => {
      const key = resolveTargetStateKey(get(), target);
      if (!key) {
        return EMPTY_THREAD_STATE;
      }
      return getThreadStateFromSnapshot(get().threadSnapshotByID[key] || null);
    },

    clearLiveOverlay: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key];
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: prev ? clearLiveOverlayState(prev) : createEmptyLiveOverlay(),
        },
      };
    }),

    prepareLiveOverlayForNewRun: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      const prev = state.liveOverlayByThreadID[key];
      return {
        liveOverlayByThreadID: {
          ...state.liveOverlayByThreadID,
          [key]: prev ? prepareLiveOverlayForNewRunState(prev) : createEmptyLiveOverlay(),
        },
      };
    }),

    removeLiveOverlay: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key || !state.liveOverlayByThreadID[key]) {
        return {};
      }
      const next = { ...state.liveOverlayByThreadID };
      delete next[key];
      return { liveOverlayByThreadID: next };
    }),

    getLiveOverlay: (chatPath) => {
      const key = resolveChatStateKey(get(), chatPath);
      if (!key) {
        return EMPTY_LIVE_OVERLAY;
      }
      return get().liveOverlayByThreadID[key] || EMPTY_LIVE_OVERLAY;
    },
    getLiveOverlayForTarget: (target) => {
      const key = resolveTargetStateKey(get(), target);
      if (!key) {
        return EMPTY_LIVE_OVERLAY;
      }
      return get().liveOverlayByThreadID[key] || EMPTY_LIVE_OVERLAY;
    },

    setReviews: (chatPath, reviews) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      return {
        reviewByThreadID: {
          ...state.reviewByThreadID,
          [key]: [...reviews],
        },
      };
    }),

    getReviews: (chatPath) => {
      const key = resolveChatStateKey(get(), chatPath);
      if (!key) {
        return EMPTY_REVIEWS;
      }
      return get().reviewByThreadID[key] || EMPTY_REVIEWS;
    },

    upsertThreadMeta: (meta) => set((state) => {
      const chatPath = normalizeChatPath(meta.chatPath || meta.path);
      const threadID = (meta.threadID || '').trim();
      if (!threadID) {
        return {};
      }
      const previousMeta = state.threadMetaByThreadID[threadID] || null;
      const previousChatPath = normalizeChatPath(previousMeta?.chatPath);
      const nextPathToThreadID = { ...state.pathToThreadID };
      let previousThreadID = '';
      for (const [path, existingThreadID] of Object.entries(nextPathToThreadID)) {
        if ((existingThreadID || '').trim() !== threadID) {
          continue;
        }
        previousThreadID = threadID;
        if (path !== chatPath) {
          delete nextPathToThreadID[path];
        }
      }
      if (!previousThreadID) {
        previousThreadID = (state.pathToThreadID[chatPath] || '').trim();
      }
      if (chatPath) {
        nextPathToThreadID[chatPath] = threadID;
      }
      const fromKeyCandidates = [previousThreadID, previousChatPath, chatPath, threadID];
      const toKey = threadID;
      const nextQueuedMessagesByThreadID = moveRecordValueFromCandidates(state.queuedMessagesByThreadID, fromKeyCandidates, toKey);
      const nextAwaitingUserByThreadID = moveRecordValueFromCandidates(state.awaitingUserByThreadID, fromKeyCandidates, toKey);
      const movedThreadSnapshotByID = moveRecordValueFromCandidates(state.threadSnapshotByID, fromKeyCandidates, toKey);
      const nextThreadSnapshotByID = movedThreadSnapshotByID[toKey]
        ? {
          ...movedThreadSnapshotByID,
          [toKey]: {
            ...movedThreadSnapshotByID[toKey],
            meta: {
              ...movedThreadSnapshotByID[toKey].meta,
              ...meta,
              ...(chatPath ? { chatPath, path: chatPath } : {}),
            },
          },
        }
        : movedThreadSnapshotByID;
      const nextLiveOverlayByThreadID = moveRecordValueFromCandidates(state.liveOverlayByThreadID, fromKeyCandidates, toKey);
      const nextReviewByThreadID = moveRecordValueFromCandidates(state.reviewByThreadID, fromKeyCandidates, toKey);
      const nextPlanRevisionByThreadID = moveRecordValueFromCandidates(state.planRevisionByThreadID, fromKeyCandidates, toKey);
      const nextPendingScrollToBottomByThreadID = moveRecordValueFromCandidates(state.pendingScrollToBottomByThreadID, fromKeyCandidates, toKey);
      const threadTargetKey = `thread:${threadID}`;
      const commandTargetKeys = chatPath
        ? Array.from(new Set([
          `command:${chatPath}`,
          `chat:${chatPath}`,
          ...(previousChatPath ? [`command:${previousChatPath}`, `chat:${previousChatPath}`] : []),
        ]))
        : [];
      let nextDraftByTargetKey = state.draftByTargetKey;
      let nextComposerPlanStateByTargetKey = state.composerPlanStateByTargetKey;
      let nextModelKeyByTargetKey = state.modelKeyByTargetKey;
      let nextAgentByTargetKey = state.agentByTargetKey;
      let nextInProgressByTargetKey = state.inProgressByTargetKey;
      for (const commandTargetKey of commandTargetKeys) {
        nextDraftByTargetKey = moveRecordValue(nextDraftByTargetKey, commandTargetKey, threadTargetKey);
        nextComposerPlanStateByTargetKey = moveRecordValue(nextComposerPlanStateByTargetKey, commandTargetKey, threadTargetKey);
        nextModelKeyByTargetKey = moveRecordValue(nextModelKeyByTargetKey, commandTargetKey, threadTargetKey);
        nextAgentByTargetKey = moveRecordValue(nextAgentByTargetKey, commandTargetKey, threadTargetKey);
        nextInProgressByTargetKey = moveRecordValue(nextInProgressByTargetKey, commandTargetKey, threadTargetKey);
      }
      const metaDefaultAgent = normalizeSelectedAgentTarget({
        agentID: meta.agentID,
        agentName: null,
        agentCwd: meta.cwd,
      });
      if (metaDefaultAgent && !nextAgentByTargetKey[threadTargetKey]) {
        nextAgentByTargetKey = {
          ...nextAgentByTargetKey,
          [threadTargetKey]: metaDefaultAgent,
        };
      }
      const selected = state.selectedConversationTarget;
      const shouldSelectThread = selected?.kind === 'thread'
        ? selected.threadID === threadID
        : selected?.kind === 'command' && chatPath
          ? selected.path === chatPath || (!!previousChatPath && selected.path === previousChatPath)
          : false;
      const nextSelectedTarget = shouldSelectThread
        ? ({ kind: 'thread', threadID, ...(chatPath ? { chatPath } : {}) } as const)
        : selected;
      const nextTargetChatPath = chatPath && (
        state.targetChatPath === chatPath
        || (!!previousChatPath && state.targetChatPath === previousChatPath)
        || (nextSelectedTarget?.kind === 'thread' && nextSelectedTarget.threadID === threadID)
      )
        ? chatPath
        : state.targetChatPath;
      return {
        draftByTargetKey: nextDraftByTargetKey,
        composerPlanStateByTargetKey: nextComposerPlanStateByTargetKey,
        modelKeyByTargetKey: nextModelKeyByTargetKey,
        agentByTargetKey: nextAgentByTargetKey,
        inProgressByTargetKey: nextInProgressByTargetKey,
        selectedConversationTarget: nextSelectedTarget,
        targetChatPath: nextTargetChatPath,
        openComposerTargets: upsertOpenComposerTarget(state.openComposerTargets, {
          threadID,
          ...(chatPath ? { chatPath } : {}),
          title: meta.title,
          agentID: meta.agentID,
        }),
        threadMetaByThreadID: {
          ...state.threadMetaByThreadID,
          [threadID]: {
            ...meta,
            ...(chatPath ? { chatPath, path: chatPath } : { chatPath: '', path: meta.path || '' }),
          },
        },
        pathToThreadID: nextPathToThreadID,
        queuedMessagesByThreadID: nextQueuedMessagesByThreadID,
        awaitingUserByThreadID: nextAwaitingUserByThreadID,
        threadSnapshotByID: nextThreadSnapshotByID,
        liveOverlayByThreadID: nextLiveOverlayByThreadID,
        reviewByThreadID: nextReviewByThreadID,
        planRevisionByThreadID: nextPlanRevisionByThreadID,
        pendingScrollToBottomByThreadID: nextPendingScrollToBottomByThreadID,
      };
    }),

    getThreadMeta: (chatPath) => {
      const threadID = resolveChatStateKey(get(), chatPath);
      if (!threadID) {
        return null;
      }
      return get().threadMetaByThreadID[threadID] || null;
    },

    getThreadMetaByThreadID: (threadID) => {
      const normalizedThreadID = (threadID || '').trim();
      return normalizedThreadID ? get().threadMetaByThreadID[normalizedThreadID] || null : null;
    },

    getTargetChatPath: (target) => {
      if (!target) {
        return null;
      }
      if (target.kind === 'thread') {
        const meta = get().threadMetaByThreadID[target.threadID] || null;
        return normalizeChatPath(meta?.chatPath || target.chatPath) || null;
      }
      if (target.kind === 'command') {
        return normalizeChatPath(target.path) || null;
      }
      return null;
    },

    bumpPlanRevision: (chatPath) => set((state) => {
      const key = resolveChatStateKey(state, chatPath);
      if (!key) {
        return {};
      }
      return {
        planRevisionByThreadID: {
          ...state.planRevisionByThreadID,
          [key]: (state.planRevisionByThreadID[key] || 0) + 1,
        },
      };
    }),

    getPlanRevision: (chatPath) => {
      const key = resolveChatStateKey(get(), chatPath);
      if (!key) {
        return 0;
      }
      return get().planRevisionByThreadID[key] || 0;
    },
  }));
}

export function getChatWorkspaceStore(tabId: string): ChatWorkspaceStore {
  const existing = chatWorkspaceStores.get(tabId);
  if (existing) {
    return existing;
  }
  const store = createChatWorkspaceStore();
  chatWorkspaceStores.set(tabId, store);
  return store;
}

export function getActiveChatWorkspaceStore(): ChatWorkspaceStore {
  const { activeTabId } = useTabManagerStore.getState();
  return getChatWorkspaceStore(activeTabId);
}

export function removeChatWorkspaceStore(tabId: string) {
  chatWorkspaceStores.delete(tabId);
}

export const useChatWorkspaceStore = (<T = ChatWorkspaceState>(selector?: (state: ChatWorkspaceState) => T): T => {
  const activeTabId = useTabManagerStore((state) => state.activeTabId);
  const store = getChatWorkspaceStore(activeTabId);
  const select = selector ?? ((state) => state as unknown as T);
  return useStore(store, select);
}) as ChatWorkspaceHook;

useChatWorkspaceStore.getState = () => getActiveChatWorkspaceStore().getState();
useChatWorkspaceStore.getStateByTabId = (tabId: string) => getChatWorkspaceStore(tabId).getState();
useChatWorkspaceStore.getStoreByTabId = (tabId: string) => getChatWorkspaceStore(tabId);
