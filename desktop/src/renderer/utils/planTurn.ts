import { normalizePosixPath } from './markdownMedia';
import { parsePlanChecklist, type PlanChecklistParseResult } from './planChecklist';

export type PlanSkillContext = {
  planFilePath?: string;
  planDir?: string;
  title?: string;
};

export type ResolvedPlanFile = {
  path: string;
  title: string;
};

export type PlanFileSnapshot = {
  path: string;
  exists: boolean;
  isDir: boolean;
  content: string | null;
};

export type VerifiedPlanFileResult =
  | { ok: true; plan: ResolvedPlanFile }
  | { ok: false; error: string };

function formatPlanChecklistValidationError(path: string, result: Extract<PlanChecklistParseResult, { ok: false }>): string {
  switch (result.code) {
    case 'missing-task-section':
      return `plan 文件缺少 \`## Tasks\` 或 \`## 任务\` 任务区：${path}`;
    case 'duplicate-task-section':
      return `plan 文件包含多个 \`## Tasks\` / \`## 任务\` 任务区：${path}`;
    case 'empty-task-section':
      return `plan 文件的 \`## Tasks\` / \`## 任务\` 任务区缺少 checklist：${path}`;
    default:
      return `${result.error}：${path}`;
  }
}

function getFileStem(path: string): string {
  const fileName = normalizePosixPath((path || '').trim()).split('/').pop() || '';
  return fileName.replace(/\.md$/i, '').trim() || 'Plan';
}

export function buildPlanSkillContext(
  context: {
    planFilePath?: string | null;
    planDir?: string | null;
    title?: string | null;
  } | null | undefined,
): PlanSkillContext | null {
  const normalizedPlanFilePath = normalizePosixPath((context?.planFilePath || '').trim());
  const normalizedPlanDir = normalizePosixPath((context?.planDir || '').trim());
  const normalizedTitle = (context?.title || '').trim();
  if (!normalizedPlanFilePath && !normalizedPlanDir) {
    return null;
  }
  return {
    ...(normalizedPlanFilePath ? { planFilePath: normalizedPlanFilePath } : {}),
    ...(normalizedPlanDir ? { planDir: normalizedPlanDir } : {}),
    ...(normalizedTitle ? { title: normalizedTitle } : {}),
  };
}

export function normalizePlanSkillContext(
  context: {
    planFilePath?: string | null;
    planDir?: string | null;
    title?: string | null;
  } | null | undefined
): PlanSkillContext | null {
  return buildPlanSkillContext(context);
}

export function verifyBoundPlanFileSnapshot(params: {
  snapshot: PlanFileSnapshot;
}): VerifiedPlanFileResult {
  const path = normalizePosixPath((params.snapshot.path || '').trim());
  if (!path) {
    return { ok: false, error: '未提供绑定的 plan 文件路径。' };
  }
  if (!params.snapshot.exists) {
    return { ok: false, error: `绑定的 plan 文件未生成：${path}` };
  }
  if (params.snapshot.isDir) {
    return { ok: false, error: `绑定的 plan 路径不是文件：${path}` };
  }
  if (params.snapshot.content == null) {
    return { ok: false, error: `读取绑定的 plan 文件失败：${path}` };
  }
  const content = params.snapshot.content;
  if (!content.trim()) {
    return { ok: false, error: `绑定的 plan 文件为空：${path}` };
  }
  if (content.includes('<!-- openbrain-plan-seed -->')) {
    return { ok: false, error: `plan 文件仍是旧的占位内容：${path}` };
  }
  const checklist = parsePlanChecklist(content, {
    fallbackTitle: getFileStem(path),
  });
  if (!checklist.ok) {
    return { ok: false, error: formatPlanChecklistValidationError(path, checklist) };
  }
  return {
    ok: true,
    plan: {
      path,
      title: checklist.title,
    },
  };
}
