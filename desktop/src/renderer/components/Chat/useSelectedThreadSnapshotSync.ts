import { useEffect } from 'react';
import { useChatWorkspaceStore } from '../../store/chatWorkspaceStore';
import { refreshThreadStateByThreadID } from '../../services/chatService';

export function useSelectedThreadSnapshotSync() {
  const selectedConversationTarget = useChatWorkspaceStore((state) => state.selectedConversationTarget);
  const selectedChatPath = useChatWorkspaceStore((state) => state.getTargetChatPath(state.selectedConversationTarget));
  const selectedModelKey = useChatWorkspaceStore((state) => state.getSelectedModelKey());
  const selectedTargetInProgress = useChatWorkspaceStore((state) => state.isTargetInProgress(state.selectedConversationTarget));

  useEffect(() => {
    if (selectedConversationTarget?.kind !== 'thread') {
      return;
    }
    const threadID = selectedConversationTarget.threadID.trim();
    if (!threadID) {
      return;
    }
    let cancelled = false;
    refreshThreadStateByThreadID(threadID, {
      chatPath: selectedChatPath || selectedConversationTarget.chatPath || null,
      modelKey: selectedModelKey || null,
    }).catch(() => {
      if (cancelled) {
        return;
      }
    });
    return () => {
      cancelled = true;
    };
  }, [
    selectedChatPath,
    selectedConversationTarget?.kind,
    selectedConversationTarget?.kind === 'thread' ? selectedConversationTarget.threadID : null,
    selectedConversationTarget?.kind === 'thread' ? selectedConversationTarget.chatPath : null,
    selectedTargetInProgress,
    selectedModelKey,
  ]);
}
