export type ActivityThreadMetadataViewModel = {
  threadID: string;
  chatFileName: string;
  createdAtLabel: string;
};

const THREAD_ID_DATE_RE = /^thread-(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z-/;

export function parseThreadCreatedAtFromID(threadID: string | null | undefined): Date | null {
  const normalized = (threadID || '').trim();
  const match = THREAD_ID_DATE_RE.exec(normalized);
  if (!match) {
    return null;
  }
  const [, yearStr, monthStr, dayStr, hourStr, minuteStr, secondStr] = match;
  const year = Number(yearStr);
  const month = Number(monthStr);
  const day = Number(dayStr);
  const hour = Number(hourStr);
  const minute = Number(minuteStr);
  const second = Number(secondStr);
  if (
    Number.isNaN(year) || Number.isNaN(month) || Number.isNaN(day)
    || Number.isNaN(hour) || Number.isNaN(minute) || Number.isNaN(second)
  ) {
    return null;
  }
  const date = new Date(Date.UTC(year, month - 1, day, hour, minute, second));
  if (Number.isNaN(date.getTime())) {
    return null;
  }
  return date;
}

export function formatActivityThreadCreatedAt(date: Date | null | undefined): string {
  if (!date || Number.isNaN(date.getTime())) {
    return '';
  }
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

export function resolveActivityChatFileName(chatPath: string | null | undefined): string {
  const normalized = (chatPath || '').trim();
  if (!normalized) {
    return '';
  }
  return normalized.split('/').filter(Boolean).pop() || '';
}

export function buildActivityThreadMetadataViewModel(input: {
  threadID: string | null | undefined;
  chatPath: string | null | undefined;
}): ActivityThreadMetadataViewModel {
  const threadID = (input.threadID || '').trim();
  const chatFileName = resolveActivityChatFileName(input.chatPath);
  const createdAt = parseThreadCreatedAtFromID(threadID);
  const createdAtLabel = formatActivityThreadCreatedAt(createdAt);
  return { threadID, chatFileName, createdAtLabel };
}
