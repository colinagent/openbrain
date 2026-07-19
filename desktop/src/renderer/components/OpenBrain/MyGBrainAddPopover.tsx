import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';

import { useDismissOnOutsideInteraction } from '../../hooks/useDismissOnOutsideInteraction';
import { OPENBRAIN_GRAPH_CAPSULE } from '../staticGlassCapsule';
import type { PublicBrainDirectoryEntry } from '../../store/openBrainStore';

const POPOVER_GAP = 12;
const VIEWPORT_MARGIN = 12;
const POPOVER_WIDTH = 320;

type PopoverLayout = { top: number; left: number };

function computePopoverLayout(anchor: HTMLElement, panel: HTMLElement): PopoverLayout {
  const anchorRect = anchor.getBoundingClientRect();
  const height = panel.offsetHeight;
  const width = panel.offsetWidth || POPOVER_WIDTH;
  const left = Math.min(
    Math.max(VIEWPORT_MARGIN, anchorRect.left + anchorRect.width / 2 - width / 2),
    Math.max(VIEWPORT_MARGIN, window.innerWidth - width - VIEWPORT_MARGIN),
  );

  const spaceAbove = anchorRect.top - POPOVER_GAP - VIEWPORT_MARGIN;
  const spaceBelow = window.innerHeight - anchorRect.bottom - POPOVER_GAP - VIEWPORT_MARGIN;
  const placeAbove = spaceAbove >= height || (spaceAbove >= spaceBelow && spaceBelow < height);

  if (placeAbove) {
    return {
      top: Math.max(VIEWPORT_MARGIN, anchorRect.top - POPOVER_GAP - height),
      left,
    };
  }

  return {
    top: anchorRect.bottom + POPOVER_GAP,
    left,
  };
}

type MyGBrainAddPopoverProps = {
  anchorRef: React.RefObject<HTMLElement | null>;
  open: boolean;
  loggedIn: boolean;
  busy: boolean;
  onClose: () => void;
  onCreateSource: () => Promise<void>;
  onFollowPublicBrain: (ownerUID: string) => Promise<void>;
  onUnfollowPublicBrain: (ownerUID: string) => Promise<void>;
  listPublicBrainDirectory: (query: string) => Promise<PublicBrainDirectoryEntry[]>;
  onLogin: () => Promise<void>;
};

function PublicBrainDirectoryAvatar({ entry }: { entry: PublicBrainDirectoryEntry }) {
  const avatar = (entry.avatar || '').trim();
  const [imageFailed, setImageFailed] = useState(false);

  useEffect(() => {
    setImageFailed(false);
  }, [avatar]);

  return (
    <span className="grid h-7 w-7 shrink-0 place-items-center overflow-hidden rounded-full border border-white/55 bg-[#2f8f6b] text-xs font-black leading-none text-white">
      {avatar && !imageFailed ? (
        <img
          src={avatar}
          alt=""
          className="h-full w-full object-cover"
          draggable={false}
          referrerPolicy="no-referrer"
          onError={() => setImageFailed(true)}
        />
      ) : (
        entry.ownerInitial || entry.name.slice(0, 1).toUpperCase()
      )}
    </span>
  );
}

export const MyGBrainAddPopover: React.FC<MyGBrainAddPopoverProps> = ({
  anchorRef,
  open,
  loggedIn,
  busy,
  onClose,
  onCreateSource,
  onFollowPublicBrain,
  onUnfollowPublicBrain,
  listPublicBrainDirectory,
  onLogin,
}) => {
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [showPublicSearch, setShowPublicSearch] = useState(false);
  const [query, setQuery] = useState('');
  const [directory, setDirectory] = useState<PublicBrainDirectoryEntry[]>([]);
  const [directoryLoading, setDirectoryLoading] = useState(false);
  const [actionBusy, setActionBusy] = useState(false);
  const [layout, setLayout] = useState<PopoverLayout | null>(null);

  const syncLayout = () => {
    const anchor = anchorRef.current;
    const panel = panelRef.current;
    if (!anchor || !panel) {
      return false;
    }
    setLayout(computePopoverLayout(anchor, panel));
    return true;
  };

  useLayoutEffect(() => {
    if (!open) {
      setLayout(null);
      return;
    }
    syncLayout();
  }, [anchorRef, open, showPublicSearch]);

  useDismissOnOutsideInteraction({
    active: open,
    onDismiss: onClose,
    insideRefs: [panelRef],
  });

  useEffect(() => {
    if (!open) {
      return;
    }
    const update = () => {
      syncLayout();
    };
    window.addEventListener('resize', update);
    window.addEventListener('scroll', update, true);
    return () => {
      window.removeEventListener('resize', update);
      window.removeEventListener('scroll', update, true);
    };
  }, [anchorRef, open, showPublicSearch]);

  useEffect(() => {
    if (!open || !loggedIn || !showPublicSearch) {
      return;
    }
    let cancelled = false;
    setDirectoryLoading(true);
    void listPublicBrainDirectory(query).then((entries) => {
      if (!cancelled) {
        setDirectory(entries);
        setDirectoryLoading(false);
      }
    }).catch(() => {
      if (!cancelled) {
        setDirectory([]);
        setDirectoryLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [listPublicBrainDirectory, loggedIn, open, query, showPublicSearch]);

  const filteredDirectory = useMemo(() => directory, [directory]);

  if (!open) {
    return null;
  }

  const runAction = async (action: () => Promise<void>, options?: { closeBeforeAction?: boolean }) => {
    if (actionBusy || busy) {
      return;
    }
    if (options?.closeBeforeAction) {
      onClose();
      await action();
      return;
    }
    setActionBusy(true);
    try {
      await action();
      onClose();
    } finally {
      setActionBusy(false);
    }
  };

  return (
    <>
      <button
        type="button"
        className="openbrain-add-popover-scrim no-drag"
        aria-label="Close add menu"
        onClick={onClose}
      />
      <div
        ref={panelRef}
        className={`openbrain-add-popover no-drag ${OPENBRAIN_GRAPH_CAPSULE} openbrain-add-popover-panel${layout ? '' : ' openbrain-add-popover-panel--measuring'}`}
        style={layout ? { top: `${layout.top}px`, left: `${layout.left}px` } : undefined}
        role="dialog"
        aria-label="Add to MyGBrain"
      >
        {!loggedIn ? (
          <button
            type="button"
            className={`${OPENBRAIN_GRAPH_CAPSULE} openbrain-add-popover-btn w-full px-4 py-2 text-sm font-bold`}
            disabled={actionBusy || busy}
            onClick={() => void runAction(onLogin)}
          >
            Log in
          </button>
        ) : (
          <>
            <div className="mb-3 flex flex-col gap-2">
              <button
                type="button"
                className={`${OPENBRAIN_GRAPH_CAPSULE} openbrain-add-popover-btn w-full px-4 py-2 text-sm font-bold`}
                disabled={actionBusy || busy}
                onClick={() => void runAction(onCreateSource, { closeBeforeAction: true })}
              >
                Add source
              </button>
              <button
                type="button"
                className={`${OPENBRAIN_GRAPH_CAPSULE} openbrain-add-popover-btn w-full px-4 py-2 text-sm font-bold${showPublicSearch ? ' ring-1 ring-[#2f8f6b]/40' : ''}`}
                disabled={actionBusy || busy}
                onClick={() => setShowPublicSearch((value) => !value)}
              >
                Add public brain
              </button>
            </div>

            {showPublicSearch ? (
              <div className="space-y-2">
                <input
                  type="search"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Search public brains"
                  className={`${OPENBRAIN_GRAPH_CAPSULE} openbrain-add-popover-input w-full px-3 py-2 text-sm outline-none`}
                />
                <div className="max-h-52 space-y-1 overflow-auto">
                  {directoryLoading ? (
                    <div className="px-1 py-2 text-xs text-prime-text/60">Searching…</div>
                  ) : null}
                  {!directoryLoading && filteredDirectory.length === 0 ? (
                    <div className="px-1 py-2 text-xs text-prime-text/60">No other public brains found.</div>
                  ) : null}
                  {filteredDirectory.map((entry) => (
                    <div
                      key={entry.ownerUID}
                      className={`${OPENBRAIN_GRAPH_CAPSULE} openbrain-add-popover-row flex items-center justify-between gap-2 px-3 py-2`}
                    >
                      <div className="flex min-w-0 items-center gap-2">
                        <PublicBrainDirectoryAvatar entry={entry} />
                        <div className="min-w-0">
                          <div className="flex min-w-0 items-center gap-2">
                            <div className="truncate text-sm font-bold">{entry.name}</div>
                            {entry.owned ? (
                              <span className={`${OPENBRAIN_GRAPH_CAPSULE} shrink-0 px-2 py-0.5 text-[10px] font-bold text-prime-text/70`}>
                                You
                              </span>
                            ) : null}
                          </div>
                          <div className="truncate text-[11px] text-prime-text/60">
                            @{entry.username} · {entry.activeSourceCount} public source{entry.activeSourceCount === 1 ? '' : 's'}
                          </div>
                          {entry.description ? (
                            <div className="mt-1 max-h-8 overflow-hidden text-[11px] leading-4 text-prime-text/60">
                              {entry.description}
                            </div>
                          ) : null}
                        </div>
                      </div>
                      <button
                        type="button"
                        className={`${OPENBRAIN_GRAPH_CAPSULE} openbrain-add-popover-btn shrink-0 px-2 py-1 text-xs font-bold`}
                        disabled={actionBusy || busy || entry.owned}
                        onClick={() => void runAction(() => (
                          entry.owned
                            ? Promise.resolve()
                            : entry.followed
                            ? onUnfollowPublicBrain(entry.ownerUID)
                            : onFollowPublicBrain(entry.ownerUID)
                        ))}
                      >
                        {entry.owned ? 'You' : entry.followed ? 'Unfollow' : 'Follow'}
                      </button>
                    </div>
                  ))}
                </div>
                <p className="text-[11px] leading-4 text-prime-text/55">
                  Your public sources are already in MyGBrain. Add public brains from other users here.
                </p>
              </div>
            ) : null}
          </>
        )}
      </div>
    </>
  );
};
