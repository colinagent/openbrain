export type RetargetConversationTarget =
  | { kind: 'thread'; threadID: string; chatPath?: string }
  | { kind: 'command'; path: string }
  | { kind: 'pending'; id: string }
  | null;

export type RetargetableChatSnapshot<
  TSnapshot = unknown,
  TOverlay = unknown,
  TComposerPlanState = unknown,
  TAgent = unknown,
> = {
  draftByTargetKey: Record<string, string>;
  composerPlanStateByTargetKey: Record<string, TComposerPlanState>;
  modelKeyByTargetKey: Record<string, string>;
  agentByTargetKey: Record<string, TAgent>;
  threadSnapshotByID: Record<string, TSnapshot>;
  liveOverlayByThreadID: Record<string, TOverlay>;
  targetChatPath: string | null;
  selectedConversationTarget: RetargetConversationTarget;
};

function normalizeChatPath(chatPath: string | null | undefined): string {
  return (chatPath || '').trim();
}

export function moveRecordValue<T>(record: Record<string, T>, fromKey: string, toKey: string): Record<string, T> {
  if (!fromKey || !toKey || fromKey === toKey || !(fromKey in record)) {
    return record;
  }
  const next = { ...record };
  next[toKey] = next[fromKey];
  delete next[fromKey];
  return next;
}

export function retargetChatSnapshot<
  TSnapshot = unknown,
  TOverlay = unknown,
  TComposerPlanState = unknown,
  TAgent = unknown,
>(
  snapshot: RetargetableChatSnapshot<TSnapshot, TOverlay, TComposerPlanState, TAgent>,
  oldPath: string,
  newPath: string
): RetargetableChatSnapshot<TSnapshot, TOverlay, TComposerPlanState, TAgent> {
  const from = normalizeChatPath(oldPath);
  const to = normalizeChatPath(newPath);
  if (!from || !to || from === to) {
    return snapshot;
  }

  const fromDraftKey = `command:${from}`;
  const toDraftKey = `command:${to}`;
  let selectedConversationTarget: RetargetConversationTarget = snapshot.selectedConversationTarget;
  if (snapshot.selectedConversationTarget?.kind === 'command' && snapshot.selectedConversationTarget.path === from) {
    selectedConversationTarget = { kind: 'command', path: to };
  } else if (snapshot.selectedConversationTarget?.kind === 'thread' && snapshot.selectedConversationTarget.chatPath === from) {
    selectedConversationTarget = {
      kind: 'thread',
      threadID: snapshot.selectedConversationTarget.threadID,
      chatPath: to,
    };
  }

  return {
    draftByTargetKey: moveRecordValue(snapshot.draftByTargetKey, fromDraftKey, toDraftKey),
    composerPlanStateByTargetKey: moveRecordValue(snapshot.composerPlanStateByTargetKey, fromDraftKey, toDraftKey),
    modelKeyByTargetKey: moveRecordValue(snapshot.modelKeyByTargetKey, fromDraftKey, toDraftKey),
    agentByTargetKey: moveRecordValue(snapshot.agentByTargetKey, fromDraftKey, toDraftKey),
    threadSnapshotByID: moveRecordValue(snapshot.threadSnapshotByID, from, to),
    liveOverlayByThreadID: moveRecordValue(snapshot.liveOverlayByThreadID, from, to),
    targetChatPath: snapshot.targetChatPath === from ? to : snapshot.targetChatPath,
    selectedConversationTarget,
  };
}
