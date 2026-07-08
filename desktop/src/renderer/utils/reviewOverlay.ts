import type { ThreadReviewState } from '../services/reviewService';
import type { EditorReviewOverlay } from '../store/appStore';

export function findPendingReviewOverlayForFile(
  reviews: ThreadReviewState[],
  filePath: string | null | undefined
): EditorReviewOverlay | null {
  const normalizedPath = (filePath || '').trim();
  if (!normalizedPath) {
    return null;
  }
  for (const review of reviews) {
    if (review.status !== 'pending') {
      continue;
    }
    for (const file of review.files) {
      if (file.path !== normalizedPath || file.status !== 'pending') {
        continue;
      }
      if ((file.hunks || []).length === 0 && (file.changedRanges || []).length === 0) {
        continue;
      }
      return {
        filePath: file.path,
        threadID: review.threadID,
        turnID: review.turnID,
        chatPath: review.chatPath,
        changedRanges: file.changedRanges || [],
        hunks: file.hunks || [],
      };
    }
  }
  return null;
}
