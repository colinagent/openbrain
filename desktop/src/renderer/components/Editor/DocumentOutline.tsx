import React, { useEffect, useMemo, useRef, useState } from 'react';
import type { Text } from '@codemirror/state';
import { PinIcon } from '../Icons';
import type { MarkdownEditorInstance } from './codemirror/setup';
import {
  buildOutlineTreeEntries,
  getLinesFromContent,
  parseOutlineEntries,
  type OutlineEntry,
  type OutlineTreeEntry,
  type ParsedLine,
} from './chatMarkdownStructure';

type DocumentOutlineProps = {
  content: string;
  editorRef: React.RefObject<MarkdownEditorInstance | null>;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  onEntriesChange?: (hasEntries: boolean) => void;
  pinEnabled?: boolean;
  pinned?: boolean;
  onPinToggle?: () => void;
  outlineToggleEnabled?: boolean;
};

function getLinesFromDoc(doc: Text): ParsedLine[] {
  const lines: ParsedLine[] = [];
  for (let number = 1; number <= doc.lines; number++) {
    const line = doc.line(number);
    lines.push({
      number,
      from: line.from,
      text: line.text,
    });
  }
  return lines;
}

function filterVisibleEntries(entries: OutlineTreeEntry[], expandedIds: Set<string>): OutlineTreeEntry[] {
  return entries.filter((entry) => entry.ancestorIds.every((ancestorId) => expandedIds.has(ancestorId)));
}

function normalizeExpandedIds(entries: OutlineTreeEntry[], expandedIds: Set<string>): Set<string> {
  const expandableIds = new Set(entries.filter((entry) => entry.hasChildren).map((entry) => entry.id));
  const nextExpandedIds = new Set<string>();
  for (const id of expandedIds) {
    if (expandableIds.has(id)) {
      nextExpandedIds.add(id);
    }
  }
  return nextExpandedIds;
}

function resolveExpandedIds(
  autoExpandedIds: Set<string>,
  manualExpandedIds: Set<string>,
  manualCollapsedIds: Set<string>
): Set<string> {
  const resolved = new Set<string>(autoExpandedIds);
  for (const id of manualExpandedIds) {
    resolved.add(id);
  }
  for (const id of manualCollapsedIds) {
    resolved.delete(id);
  }
  return resolved;
}

function getAutoExpandedIds(activeId: string | null, entryMap: Map<string, OutlineTreeEntry>): Set<string> {
  if (!activeId) {
    return new Set();
  }
  const activeEntry = entryMap.get(activeId);
  if (!activeEntry || activeEntry.type !== 'heading' || !activeEntry.topLevelId) {
    return new Set();
  }

  const nextExpandedIds = new Set<string>();
  if (activeEntry.level === 1 && activeEntry.hasChildren) {
    nextExpandedIds.add(activeEntry.id);
  }
  for (const ancestorId of activeEntry.ancestorIds) {
    const ancestor = entryMap.get(ancestorId);
    if (!ancestor?.hasChildren || ancestor.topLevelId !== activeEntry.topLevelId) {
      continue;
    }
    nextExpandedIds.add(ancestorId);
  }

  return nextExpandedIds;
}

function areSetsEqual(left: Set<string>, right: Set<string>): boolean {
  if (left.size !== right.size) {
    return false;
  }
  for (const value of left) {
    if (!right.has(value)) {
      return false;
    }
  }
  return true;
}

function findActiveIndex(entries: OutlineEntry[], pos: number): number {
  if (entries.length === 0) {
    return -1;
  }
  let low = 0;
  let high = entries.length - 1;
  let result = -1;
  while (low <= high) {
    const mid = Math.floor((low + high) / 2);
    if (entries[mid].spyPos <= pos) {
      result = mid;
      low = mid + 1;
    } else {
      high = mid - 1;
    }
  }
  return result;
}

export const DocumentOutline: React.FC<DocumentOutlineProps> = ({
  content,
  editorRef,
  expanded,
  onExpandedChange,
  onEntriesChange,
  pinEnabled = false,
  pinned = false,
  onPinToggle,
  outlineToggleEnabled = true,
}) => {
  const [manualExpandedIds, setManualExpandedIds] = useState<Set<string>>(() => new Set());
  const [manualCollapsedIds, setManualCollapsedIds] = useState<Set<string>>(() => new Set());
  const [autoExpandedIds, setAutoExpandedIds] = useState<Set<string>>(() => new Set());
  const [activeId, setActiveId] = useState<string | null>(null);
  const [viewReadySeq, setViewReadySeq] = useState(0);
  const entryRefs = useRef<Array<HTMLButtonElement | null>>([]);

  useEffect(() => {
    let rafId = 0;
    const waitForView = () => {
      if (editorRef.current?.getView()) {
        setViewReadySeq((prev) => prev + 1);
        return;
      }
      rafId = window.requestAnimationFrame(waitForView);
    };
    waitForView();
    return () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
    };
  }, [editorRef, content]);

  const entries = useMemo(() => {
    const view = editorRef.current?.getView();
    if (view) {
      return parseOutlineEntries(getLinesFromDoc(view.state.doc));
    }
    return parseOutlineEntries(getLinesFromContent(content));
  }, [content, editorRef, viewReadySeq]);

  const treeEntries = useMemo(() => buildOutlineTreeEntries(entries), [entries]);

  const entryMap = useMemo(() => {
    const map = new Map<string, OutlineTreeEntry>();
    for (const entry of treeEntries) {
      map.set(entry.id, entry);
    }
    return map;
  }, [treeEntries]);

  const expandedIds = useMemo(
    () => resolveExpandedIds(autoExpandedIds, manualExpandedIds, manualCollapsedIds),
    [autoExpandedIds, manualExpandedIds, manualCollapsedIds]
  );

  const visibleEntries = useMemo(
    () => filterVisibleEntries(treeEntries, expandedIds),
    [treeEntries, expandedIds]
  );

  const activeVisibleIndex = useMemo(
    () => visibleEntries.findIndex((entry) => entry.id === activeId),
    [visibleEntries, activeId]
  );

  useEffect(() => {
    entryRefs.current = entryRefs.current.slice(0, visibleEntries.length);
  }, [visibleEntries.length]);

  useEffect(() => {
    onEntriesChange?.(treeEntries.length > 0);
  }, [onEntriesChange, treeEntries.length]);

  useEffect(() => {
    setManualExpandedIds((prev) => {
      const next = normalizeExpandedIds(treeEntries, prev);
      return areSetsEqual(prev, next) ? prev : next;
    });
    setManualCollapsedIds((prev) => {
      const next = normalizeExpandedIds(treeEntries, prev);
      return areSetsEqual(prev, next) ? prev : next;
    });
    setAutoExpandedIds((prev) => {
      const next = normalizeExpandedIds(treeEntries, prev);
      return areSetsEqual(prev, next) ? prev : next;
    });
  }, [treeEntries]);

  useEffect(() => {
    if (treeEntries.length === 0) {
      setActiveId(null);
      return;
    }
    setActiveId((prev) => {
      if (prev && entryMap.has(prev)) {
        return prev;
      }
      return treeEntries[0].id;
    });
  }, [treeEntries, entryMap]);

  useEffect(() => {
    const view = editorRef.current?.getView();
    if (!view || treeEntries.length === 0) {
      return;
    }

    let rafId = 0;
    const syncActiveEntry = () => {
      rafId = 0;
      const visibleFrom = view.viewportLineBlocks[0]?.from ?? view.viewport.from;
      const nextIndex = findActiveIndex(treeEntries, visibleFrom);
      const nextActiveId = treeEntries[nextIndex]?.id ?? treeEntries[0]?.id ?? null;
      setActiveId((prev) => (prev === nextActiveId ? prev : nextActiveId));
    };

    const onScroll = () => {
      if (rafId) {
        return;
      }
      rafId = window.requestAnimationFrame(syncActiveEntry);
    };

    syncActiveEntry();
    view.scrollDOM.addEventListener('scroll', onScroll, { passive: true });
    return () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      view.scrollDOM.removeEventListener('scroll', onScroll);
    };
  }, [editorRef, treeEntries]);

  useEffect(() => {
    const nextAutoExpandedIds = getAutoExpandedIds(activeId, entryMap);
    setAutoExpandedIds((prev) => (areSetsEqual(prev, nextAutoExpandedIds) ? prev : nextAutoExpandedIds));
  }, [activeId, entryMap]);

  useEffect(() => {
    if (!expanded || activeVisibleIndex < 0) {
      return;
    }
    const activeEl = entryRefs.current[activeVisibleIndex];
    activeEl?.scrollIntoView({ block: 'nearest' });
  }, [activeVisibleIndex, expanded]);

  const hasEntries = treeEntries.length > 0;

  if (!hasEntries && !pinEnabled) {
    return null;
  }

  const jumpToEntry = (entry: OutlineTreeEntry) => {
    const editor = editorRef.current;
    if (!editor?.scrollToPos(entry.pos)) {
      return;
    }
    setActiveId((prev) => (prev === entry.id ? prev : entry.id));
  };

  return (
    <div className={`op-md-outline ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
      <div className="op-md-outline-toggle-wrap">
        {!expanded && pinEnabled && (
          <button
            type="button"
            className={`op-md-outline-pin icon-gutter-btn-sm icon-button-inline ${pinned ? 'is-pinned' : ''}`}
            onClick={(event) => {
              event.stopPropagation();
              onPinToggle?.();
            }}
            title={pinned ? 'Unpin file' : 'Pin file to right'}
            aria-label={pinned ? 'Unpin file' : 'Pin file to right'}
            aria-pressed={pinned}
          >
            <PinIcon className="h-3.5 w-3.5" />
          </button>
        )}
        {hasEntries && outlineToggleEnabled && (
          <button
            type="button"
            className={`op-md-outline-toggle icon-gutter-btn-sm icon-button-inline ${expanded ? 'is-active' : ''}`}
            onClick={() => onExpandedChange(!expanded)}
            title={expanded ? 'Collapse outline' : 'Expand outline'}
            aria-label={expanded ? 'Collapse outline' : 'Expand outline'}
            aria-pressed={expanded}
          >
            {expanded ? (
              <svg
                width="14"
                height="14"
                viewBox="0 0 16 16"
                fill="none"
                xmlns="http://www.w3.org/2000/svg"
                className="op-md-outline-collapse-icon"
                aria-hidden="true"
              >
                <path
                  d="M12.5 4L8.5 8L12.5 12"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
                <path
                  d="M7.5 4L3.5 8L7.5 12"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            ) : (
              <svg
                width="14"
                height="14"
                viewBox="0 0 16 16"
                fill="none"
                xmlns="http://www.w3.org/2000/svg"
                className="op-md-outline-hamburger-icon"
                aria-hidden="true"
              >
                <path
                  d="M3 4H13"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
                <path
                  d="M3 8H13"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
                <path
                  d="M3 12H9"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            )}
          </button>
        )}
      </div>
      {expanded && hasEntries && (
        <div className="op-md-outline-list">
          {visibleEntries.map((entry, index) => {
            const isActive = entry.id === activeId;
            const isBranch = entry.hasChildren;
            const isEntryExpanded = isBranch && expandedIds.has(entry.id);
            const paddingLeft = 10 + entry.depth * 12;

            const toggleEntry = () => {
              if (!isBranch) {
                return;
              }
              if (isEntryExpanded) {
                setManualExpandedIds((prev) => {
                  if (!prev.has(entry.id)) {
                    return prev;
                  }
                  const next = new Set(prev);
                  next.delete(entry.id);
                  return next;
                });
                setManualCollapsedIds((prev) => {
                  if (prev.has(entry.id)) {
                    return prev;
                  }
                  const next = new Set(prev);
                  next.add(entry.id);
                  return next;
                });
                return;
              }
              setManualCollapsedIds((prev) => {
                if (!prev.has(entry.id)) {
                  return prev;
                }
                const next = new Set(prev);
                next.delete(entry.id);
                return next;
              });
              setManualExpandedIds((prev) => {
                if (prev.has(entry.id)) {
                  return prev;
                }
                const next = new Set(prev);
                next.add(entry.id);
                return next;
              });
            };

            return (
              <div
                key={entry.id}
                className={`op-md-outline-item-row ${isActive ? 'is-active' : ''} ${isBranch ? 'is-branch' : 'is-leaf'} ${isEntryExpanded ? 'is-expanded' : ''}`}
                style={{ paddingLeft }}
              >
                <button
                  type="button"
                  className={`op-md-outline-item-chevron ${isBranch ? 'is-branch' : 'is-leaf'} ${isEntryExpanded ? 'is-expanded' : ''}`}
                  onMouseDown={(e) => {
                    e.preventDefault();
                  }}
                  onClick={(e) => {
                    e.stopPropagation();
                    toggleEntry();
                  }}
                  aria-label={isBranch
                    ? (isEntryExpanded ? `Collapse ${entry.text}` : `Expand ${entry.text}`)
                    : undefined}
                  aria-expanded={isBranch ? isEntryExpanded : undefined}
                  disabled={!isBranch}
                >
                  {isBranch ? (
                    <svg
                      className="op-md-outline-item-chevron-icon"
                      viewBox="0 0 16 16"
                      fill="none"
                      xmlns="http://www.w3.org/2000/svg"
                    >
                      <path
                        d="M5.5 3.75L10 8L5.5 12.25"
                        stroke="currentColor"
                        strokeWidth="2.25"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      />
                    </svg>
                  ) : null}
                </button>
                <button
                  ref={(el) => {
                    entryRefs.current[index] = el;
                  }}
                  type="button"
                  className={`op-md-outline-item ${isActive ? 'is-active' : ''}`}
                  onMouseDown={(e) => {
                    e.preventDefault();
                  }}
                  onClick={() => {
                    jumpToEntry(entry);
                  }}
                  title={entry.text}
                >
                  <span className="op-md-outline-item-text">
                    {entry.text}
                  </span>
                </button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
