function coerceErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  if (typeof error === 'string') {
    return error;
  }
  return '';
}

function basename(path: string): string {
  return path.replace(/\\/g, '/').split('/').filter(Boolean).pop() || path;
}

export function formatReviewActionError(error: unknown): string {
  const raw = coerceErrorMessage(error).trim();
  if (!raw) {
    return 'Review action failed.';
  }

  const agentOutputConflict = raw.match(/review conflict for (.*?): file changed since agent output/i);
  if (agentOutputConflict) {
    const fileName = basename(agentOutputConflict[1].trim());
    return `${fileName} changed after the agent wrote it. Reload the file or ask the agent to update it again before reviewing.`;
  }

  const rejectionConflict = raw.match(/review conflict for (.*?): file changed since rejection/i);
  if (rejectionConflict) {
    const fileName = basename(rejectionConflict[1].trim());
    return `${fileName} changed after it was undone. Reload the file before changing this review again.`;
  }

  const rollbackConflict = raw.match(/rollback conflict for (.*?): file changed since approval/i);
  if (rollbackConflict) {
    const fileName = basename(rollbackConflict[1].trim());
    return `${fileName} changed after it was kept. Reload the file before rolling this review back.`;
  }

  return raw.replace(/^calling\s+"[^"]+":\s*/i, '');
}
