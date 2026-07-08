function stripBom(text: string) {
  return (text || '').replace(/^\uFEFF/, '');
}

function normalizeNewlines(text: string) {
  return stripBom(text).replace(/\r\n/g, '\n');
}

function unquote(value: string) {
  const v = value.trim();
  if (v.length >= 2) {
    const first = v[0];
    const last = v[v.length - 1];
    if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
      return v.slice(1, -1).trim();
    }
  }
  return v;
}

export function extractAgentNameFromMarkdown(markdown: string): string | null {
  const text = normalizeNewlines(markdown);
  const lines = text.split('\n');
  if (lines.length === 0) {
    return null;
  }

  const readNameFromLine = (line: string) => {
    const trimmed = line.trim();
    if (!trimmed.toLowerCase().startsWith('name:')) {
      return null;
    }
    const raw = trimmed.slice(5).trim();
    if (!raw) {
      return null;
    }
    const name = unquote(raw);
    return name || null;
  };

  if (lines[0].trim() === '---') {
    for (let i = 1; i < lines.length; i += 1) {
      const t = lines[i].trim();
      if (t === '---' || t === '...') {
        break;
      }
      const name = readNameFromLine(lines[i]);
      if (name) {
        return name;
      }
    }
    return null;
  }

  for (const line of lines) {
    const name = readNameFromLine(line);
    if (name) {
      return name;
    }
  }

  return null;
}

function slugify(value: string) {
  const s = (value || '').trim().toLowerCase();
  if (!s) {
    return '';
  }
  let out = '';
  let lastDash = false;
  for (const ch of s) {
    const isAZ = ch >= 'a' && ch <= 'z';
    const is09 = ch >= '0' && ch <= '9';
    const isSafe = ch === '.' || ch === '_' || ch === '-';
    if (isAZ || is09) {
      out += ch;
      lastDash = false;
      continue;
    }
    if (isSafe) {
      if (lastDash && ch === '-') {
        continue;
      }
      out += ch;
      lastDash = ch === '-';
      continue;
    }
    if (!lastDash) {
      out += '-';
      lastDash = true;
    }
  }
  return out.replace(/^-+/, '').replace(/-+$/, '');
}

function createIdSuffix() {
  const ts = Date.now().toString(36);
  const rand = Math.random().toString(16).slice(2, 8);
  return `${ts}-${rand}`;
}

export function createCustomAgentIdFromDirName(dirName: string) {
  const base = slugify(dirName) || 'agent';
  return `${base}-${createIdSuffix()}`;
}

export function normalizeAgentNodeID(agentID: string | null | undefined) {
  const raw = (agentID || '').trim();
  const nodeID = raw.startsWith('@') ? raw.slice(1).trim() : raw;
  return nodeID.startsWith('agent-') ? nodeID : '';
}

export function buildReferenceAgentMarkdown(agentID: string) {
  const nodeID = normalizeAgentNodeID(agentID);
  if (!nodeID) {
    throw new Error('agent node id is required');
  }
  return `---\nbind: @${nodeID}\n---\n`;
}

export function buildCustomAgentTemplate(params: { dirName: string; agentID?: string }) {
  const id = (params.agentID || '').trim() || createCustomAgentIdFromDirName(params.dirName);
  const name = `${params.dirName || 'Custom'} Agent`;
  return `---\nid: ${id}\nname: ${name}\ndescription: Workspace-scoped custom agent.\ntags: user\n---\n\nYou are a helpful agent for this workspace. Be concise, safe, and practical.\n`;
}
