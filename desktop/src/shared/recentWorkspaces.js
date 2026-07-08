"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.createEmptyRecentWorkspaces = createEmptyRecentWorkspaces;
exports.normalizeRecentWorkspaces = normalizeRecentWorkspaces;
exports.upsertLocalRecent = upsertLocalRecent;
exports.upsertRemoteRecent = upsertRemoteRecent;
exports.getSortedRemoteBuckets = getSortedRemoteBuckets;
exports.getRemoteBucketByInstanceID = getRemoteBucketByInstanceID;
const MAX_RECENT = 10;
function createEmptyRecentWorkspaces() {
    return {
        local: [],
        remote: {},
    };
}
function isRecord(value) {
    return !!value && typeof value === 'object' && !Array.isArray(value);
}
function normalizeTimestamp(value) {
    return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}
function normalizePath(value) {
    if (typeof value !== 'string') {
        return null;
    }
    const path = value.trim();
    return path ? path : null;
}
function normalizeHost(value) {
    if (!isRecord(value) || typeof value.alias !== 'string' || !value.alias.trim()) {
        return null;
    }
    return {
        alias: value.alias.trim(),
        hostname: typeof value.hostname === 'string' && value.hostname.trim() ? value.hostname.trim() : undefined,
        user: typeof value.user === 'string' && value.user.trim() ? value.user.trim() : undefined,
        port: typeof value.port === 'string' && value.port.trim() ? value.port.trim() : undefined,
        identityFile: typeof value.identityFile === 'string' && value.identityFile.trim() ? value.identityFile.trim() : undefined,
    };
}
function dedupeLocal(entries) {
    const seen = new Set();
    const next = [];
    for (const entry of entries) {
        const path = normalizePath(entry.path);
        if (!path || seen.has(path)) {
            continue;
        }
        seen.add(path);
        next.push({
            path,
            lastOpenedAt: normalizeTimestamp(entry.lastOpenedAt),
        });
        if (next.length >= MAX_RECENT) {
            break;
        }
    }
    return next;
}
function dedupeRemoteDirectories(entries) {
    const seen = new Set();
    const next = [];
    for (const entry of entries) {
        const path = normalizePath(entry.path);
        if (!path || seen.has(path)) {
            continue;
        }
        seen.add(path);
        next.push({
            path,
            lastOpenedAt: normalizeTimestamp(entry.lastOpenedAt),
        });
        if (next.length >= MAX_RECENT) {
            break;
        }
    }
    return next;
}
function normalizeRecentWorkspaces(value) {
    if (!isRecord(value)) {
        return createEmptyRecentWorkspaces();
    }
    const local = Array.isArray(value.local)
        ? dedupeLocal(value.local
            .filter(isRecord)
            .map((entry) => ({
            path: typeof entry.path === 'string' ? entry.path : '',
            lastOpenedAt: normalizeTimestamp(entry.lastOpenedAt),
        })))
        : [];
    const remoteSource = isRecord(value.remote) ? value.remote : {};
    const remote = Object.entries(remoteSource).reduce((acc, [key, raw]) => {
        if (!isRecord(raw)) {
            return acc;
        }
        const instanceID = normalizePath(raw.instanceID) || key.trim();
        const host = normalizeHost(raw.host);
        if (!instanceID || !host) {
            return acc;
        }
        const directories = Array.isArray(raw.directories)
            ? dedupeRemoteDirectories(raw.directories
                .filter(isRecord)
                .map((entry) => ({
                path: typeof entry.path === 'string' ? entry.path : '',
                lastOpenedAt: normalizeTimestamp(entry.lastOpenedAt),
            })))
            : [];
        acc[instanceID] = {
            instanceID,
            host,
            label: typeof raw.label === 'string' && raw.label.trim() ? raw.label.trim() : undefined,
            lastOpenedAt: normalizeTimestamp(raw.lastOpenedAt),
            directories,
        };
        return acc;
    }, {});
    return { local, remote };
}
function upsertLocalRecent(recent, entry) {
    const path = normalizePath(entry.path);
    if (!path) {
        return recent;
    }
    return {
        local: dedupeLocal([{ path, lastOpenedAt: normalizeTimestamp(entry.lastOpenedAt) }, ...recent.local]),
        remote: recent.remote,
    };
}
function upsertRemoteRecent(recent, entry) {
    const instanceID = normalizePath(entry.instanceID);
    if (!instanceID) {
        return recent;
    }
    const currentBucket = recent.remote[instanceID];
    const nextBucket = {
        instanceID,
        host: entry.host,
        label: entry.label,
        lastOpenedAt: normalizeTimestamp(entry.lastOpenedAt),
        directories: currentBucket?.directories || [],
    };
    const path = normalizePath(entry.path);
    if (path) {
        nextBucket.directories = dedupeRemoteDirectories([
            { path, lastOpenedAt: normalizeTimestamp(entry.lastOpenedAt) },
            ...nextBucket.directories,
        ]);
    }
    return {
        local: recent.local,
        remote: {
            ...recent.remote,
            [instanceID]: nextBucket,
        },
    };
}
function getSortedRemoteBuckets(recent) {
    return Object.values(recent.remote).sort((a, b) => b.lastOpenedAt - a.lastOpenedAt);
}
function getRemoteBucketByInstanceID(recent, instanceID) {
    const key = typeof instanceID === 'string' ? instanceID.trim() : '';
    if (!key) {
        return null;
    }
    return recent.remote[key] || null;
}
