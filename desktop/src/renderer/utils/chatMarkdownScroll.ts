export const CHAT_MARKDOWN_BOTTOM_THRESHOLD_PX = 48;

export type ChatMarkdownScrollSnapshot = {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
};

export function isChatMarkdownScrollNearBottom(
  snapshot: ChatMarkdownScrollSnapshot,
  thresholdPx = CHAT_MARKDOWN_BOTTOM_THRESHOLD_PX
): boolean {
  const scrollTop = Number.isFinite(snapshot.scrollTop) ? snapshot.scrollTop : 0;
  const scrollHeight = Number.isFinite(snapshot.scrollHeight) ? snapshot.scrollHeight : 0;
  const clientHeight = Number.isFinite(snapshot.clientHeight) ? snapshot.clientHeight : 0;
  const threshold = Number.isFinite(thresholdPx) ? Math.max(0, thresholdPx) : CHAT_MARKDOWN_BOTTOM_THRESHOLD_PX;
  return scrollHeight - scrollTop - clientHeight <= threshold;
}

export function shouldFollowChatMarkdownUpdate(input: {
  isConversation: boolean;
  userDetached: boolean;
  wasNearBottom: boolean;
}): boolean {
  return input.isConversation && !input.userDetached && input.wasNearBottom;
}
