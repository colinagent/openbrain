import type { ConversationTarget } from '../store/chatWorkspaceStore';

export function shouldSyncConversationSelectionWithActiveChat(
  activeChatPath: string,
  selectedConversationTarget: ConversationTarget,
): boolean {
  if (!activeChatPath) {
    return false;
  }
  if (selectedConversationTarget?.kind === 'pending') {
    return false;
  }
  if (selectedConversationTarget?.kind === 'thread') {
    return selectedConversationTarget.chatPath !== activeChatPath;
  }
  if (selectedConversationTarget?.kind === 'command') {
    return selectedConversationTarget.path !== activeChatPath;
  }
  return true;
}
