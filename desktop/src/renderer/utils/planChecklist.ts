export type PlanChecklistItem = {
  id: string;
  text: string;
  checked: boolean;
  indent: number;
  line: number;
};

export type PlanChecklistSnapshot = {
  title: string;
  sectionHeading: 'Tasks' | '任务';
  items: PlanChecklistItem[];
  completedCount: number;
  totalCount: number;
};

export type PlanChecklistParseErrorCode =
  | 'missing-task-section'
  | 'duplicate-task-section'
  | 'empty-task-section';

export type PlanChecklistParseResult =
  | ({
    ok: true;
  } & PlanChecklistSnapshot)
  | {
    ok: false;
    title: string;
    code: PlanChecklistParseErrorCode;
    error: string;
  };

const TASK_LINE_RE = /^(\s*)[-*+]\s+\[( |x|X)\]\s+(.*)$/;
const TASK_SECTION_HEADING_RE = /^##\s+(Tasks|任务)\s*$/;
const STOP_HEADING_RE = /^#{1,2}\s+\S/;

function resolvePlanTitle(content: string, fallbackTitle?: string | null): string {
  for (const line of content.replace(/\r\n/g, '\n').split('\n')) {
    const match = /^(#{1,6})\s+(.*)$/.exec(line.trim());
    if (match && match[2].trim()) {
      return match[2].trim();
    }
  }
  return (fallbackTitle || '').trim() || 'Plan';
}

export function parsePlanChecklist(
  content: string,
  options: { fallbackTitle?: string | null } = {},
): PlanChecklistParseResult {
  const normalized = typeof content === 'string' ? content.replace(/\r\n/g, '\n') : '';
  const title = resolvePlanTitle(normalized, options.fallbackTitle);
  const lines = normalized.split('\n');
  const taskSections: Array<{ heading: 'Tasks' | '任务'; line: number }> = [];

  for (let index = 0; index < lines.length; index += 1) {
    const match = TASK_SECTION_HEADING_RE.exec((lines[index] || '').trim());
    if (!match) {
      continue;
    }
    taskSections.push({
      heading: match[1] as 'Tasks' | '任务',
      line: index,
    });
  }

  if (taskSections.length === 0) {
    return {
      ok: false,
      title,
      code: 'missing-task-section',
      error: 'Plan 缺少专用任务区：只支持 `## Tasks` 或 `## 任务`。',
    };
  }

  if (taskSections.length > 1) {
    return {
      ok: false,
      title,
      code: 'duplicate-task-section',
      error: 'Plan 只能包含一个专用任务区：`## Tasks` 或 `## 任务`。',
    };
  }

  const taskSection = taskSections[0];
  const items: PlanChecklistItem[] = [];
  let sectionEnd = lines.length;
  for (let index = taskSection.line + 1; index < lines.length; index += 1) {
    const trimmedLine = (lines[index] || '').trim();
    if (!trimmedLine) {
      continue;
    }
    if (STOP_HEADING_RE.test(trimmedLine)) {
      sectionEnd = index;
      break;
    }
  }

  for (let index = taskSection.line + 1; index < sectionEnd; index += 1) {
    const match = TASK_LINE_RE.exec(lines[index] || '');
    if (!match) {
      continue;
    }
    const text = match[3].trim();
    if (!text) {
      continue;
    }
    items.push({
      id: `${index + 1}:${match[1].length}:${text}`,
      text,
      checked: match[2].toLowerCase() === 'x',
      indent: match[1].length,
      line: index + 1,
    });
  }

  if (items.length === 0) {
    return {
      ok: false,
      title,
      code: 'empty-task-section',
      error: 'Plan 的专用任务区缺少 checklist 条目。',
    };
  }

  const completedCount = items.filter((item) => item.checked).length;
  return {
    ok: true,
    title,
    sectionHeading: taskSection.heading,
    items,
    completedCount,
    totalCount: items.length,
  };
}
