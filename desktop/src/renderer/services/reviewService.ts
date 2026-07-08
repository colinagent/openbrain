import type { WSConnection } from './wsConnection';

export type ThreadReviewFileStatus = 'pending' | 'approved' | 'rejected' | 'rolledBack';
export type ThreadReviewTurnStatus = 'pending' | 'resolved' | 'rolledBack';
export type ThreadReviewMergeState = 'clean' | 'userEdited' | 'userUndone' | 'conflicted' | 'missing';
export type ThreadReviewDecision = 'approve' | 'reject' | 'approveAll' | 'rejectAll';
export type ThreadReviewRollbackScope = 'file' | 'turn';

export type ThreadReviewLineRange = {
  startLine: number;
  endLine: number;
};

export type ThreadReviewHunk = {
  oldStartLine: number;
  oldLineCount: number;
  newStartLine: number;
  newLineCount: number;
  removedLines?: string[];
  addedLines?: string[];
};

export type ThreadReviewFile = {
  path: string;
  status: ThreadReviewFileStatus;
  mergeState?: ThreadReviewMergeState;
  hasUserEdits?: boolean;
  canUndo?: boolean;
  conflictMessage?: string;
  diff: string;
  baselineExists: boolean;
  firstChangedLine?: number;
  firstChangedColumn?: number;
  lineCount?: number;
  changedRanges?: ThreadReviewLineRange[];
  hunks?: ThreadReviewHunk[];
};

export type ThreadReviewState = {
  threadID: string;
  turnID: string;
  chatPath: string;
  status: ThreadReviewTurnStatus;
  createdAt: string;
  canReview: boolean;
  canRollback: boolean;
  unresolved: number;
  approvedCount: number;
  rejectedCount: number;
  rolledBackCount: number;
  conflictCount?: number;
  files: ThreadReviewFile[];
};

type ThreadReviewListResult = {
  reviews?: ThreadReviewState[] | null;
};

type ThreadReviewResolveParams = {
  threadID?: string;
  chatPath?: string;
  turnID: string;
  decision: ThreadReviewDecision;
  path?: string;
};

type ThreadReviewResolveResult = {
  review?: ThreadReviewState | null;
};

type ThreadReviewRollbackParams = {
  threadID?: string;
  chatPath?: string;
  turnID: string;
  scope: ThreadReviewRollbackScope;
  path?: string;
};

type ThreadReviewRollbackResult = {
  review?: ThreadReviewState | null;
};

class ReviewService {
  constructor(private connection: WSConnection) {}

  async listReviews(threadID: string): Promise<ThreadReviewState[]> {
    const normalizedThreadID = (threadID || '').trim();
    if (!normalizedThreadID) {
      return [];
    }
    const result = await this.connection.request<ThreadReviewListResult>('thread/review/list', { threadID: normalizedThreadID });
    return result.reviews || [];
  }

  async resolveReview(params: ThreadReviewResolveParams): Promise<ThreadReviewState | null> {
    const result = await this.connection.request<ThreadReviewResolveResult>('thread/review/resolve', params);
    return result.review || null;
  }

  async rollbackReview(params: ThreadReviewRollbackParams): Promise<ThreadReviewState | null> {
    const result = await this.connection.request<ThreadReviewRollbackResult>('thread/review/rollback', params);
    return result.review || null;
  }
}

export function createReviewService(connection: WSConnection) {
  return new ReviewService(connection);
}
