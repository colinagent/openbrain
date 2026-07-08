import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useAppStore } from '../../store/appStore';
import { buildAgentSwitchOptions } from '../FileExplorer/agentSwitchOptions';
import { PopupMenu } from '../PopupMenu';
import {
  getAddAgentPopupPosition,
  type AddAgentPopupAnchor,
  type AddAgentPopupPosition,
} from './addAgentPopupPosition';

export type { AddAgentPopupAnchor } from './addAgentPopupPosition';

interface AgentItem {
  dirPath: string | null;
  dirName: string;
  agentID: string;
}

type AddAgentPopupProps = {
  open: boolean;
  anchor: AddAgentPopupAnchor | null;
  onClose: () => void;
  onSelect: (agentID: string) => void;
  onCustomAgent?: () => void;
};

export const AddAgentPopup: React.FC<AddAgentPopupProps> = ({ open, anchor, onClose, onSelect, onCustomAgent }) => {
  const {
    connectionState,
    ensureDerivedDirs,
    refreshAgentNodes,
    agentNodes,
    agentsRootDir,
  } = useAppStore();
  const connected = connectionState === 'connected';

  const [hasRoots, setHasRoots] = useState(true);
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [position, setPosition] = useState<AddAgentPopupPosition | null>(null);
  const popupRef = useRef<HTMLDivElement | null>(null);

  const updatePosition = useCallback(() => {
    if (!open || !anchor || !popupRef.current) {
      setPosition((current) => (current ? null : current));
      return;
    }
    const rect = popupRef.current.getBoundingClientRect();
    const next = getAddAgentPopupPosition(
      anchor,
      { width: rect.width, height: rect.height },
      { width: window.innerWidth, height: window.innerHeight },
    );
    setPosition((current) => (
      current?.left === next.left && current.top === next.top ? current : next
    ));
  }, [anchor, open]);

  useEffect(() => {
    if (!open || !connected) {
      setHasRoots(true);
      setLoading(false);
      return;
    }

    let active = true;
    setLoading(true);

    const loadAgents = async () => {
      try {
        const derived = await ensureDerivedDirs();
        const agentsDir = (derived?.agentsDir || '').trim();
        if (!active) return;

        if (!agentsDir) {
          setHasRoots(false);
          return;
        }

        setHasRoots(true);
        await refreshAgentNodes({ force: true });
      } catch (e) {
        console.error('Failed to load agents:', e);
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    };

    loadAgents();

    return () => {
      active = false;
    };
  }, [open, connected, ensureDerivedDirs, refreshAgentNodes]);

  useLayoutEffect(() => {
    updatePosition();
  }, [updatePosition, hasRoots, loading, connected, onCustomAgent]);

  useEffect(() => {
    if (!open || !anchor) {
      setPosition(null);
      return;
    }

    const onResize = () => updatePosition();
    window.addEventListener('resize', onResize);

    const popup = popupRef.current;
    const observer = typeof ResizeObserver !== 'undefined' && popup
      ? new ResizeObserver(() => updatePosition())
      : null;
    if (popup) {
      observer?.observe(popup);
    }

    return () => {
      window.removeEventListener('resize', onResize);
      observer?.disconnect();
    };
  }, [open, anchor, updatePosition]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose();
      }
    };
    const onMouseDown = (event: MouseEvent) => {
      if (popupRef.current?.contains(event.target as Node)) {
        return;
      }
      onClose();
    };
    window.addEventListener('keydown', onKeyDown, true);
    window.addEventListener('mousedown', onMouseDown, true);
    return () => {
      window.removeEventListener('keydown', onKeyDown, true);
      window.removeEventListener('mousedown', onMouseDown, true);
    };
  }, [open, onClose]);

  const agents = useMemo<AgentItem[]>(() => (
    buildAgentSwitchOptions({
      agentNodes,
      agentsRootDir,
      currentAgentID: null,
    }).map((option) => ({
      dirPath: option.path,
      dirName: option.name,
      agentID: option.id,
    }))
  ), [agentNodes, agentsRootDir]);

  const filtered = useMemo(() => {
    const value = query.trim().toLowerCase();
    if (!value) {
      return agents;
    }
    return agents.filter((agent) => (
      agent.dirName.toLowerCase().includes(value)
      || (agent.dirPath || '').toLowerCase().includes(value)
      || agent.agentID.toLowerCase().includes(value)
    ));
  }, [agents, query]);

  if (!open || !anchor) {
    return null;
  }

  return createPortal(
    <PopupMenu
      ref={popupRef}
      className="fixed z-50 w-[360px] max-w-[calc(100vw-16px)] max-h-[70vh] overflow-hidden p-0"
      style={{
        left: position?.left ?? 0,
        top: position?.top ?? 0,
        visibility: position ? 'visible' : 'hidden',
      }}
      onMouseDown={(event) => event.stopPropagation()}
      onContextMenu={(event) => event.preventDefault()}
    >
      <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-titlebar-bg">
        <span className="text-sm font-semibold text-sidebar-fg">Add Agent</span>
        <button className="dialog-text-btn" onClick={onClose}>
          Close
        </button>
      </div>

      <div className="p-3 border-b border-border">
        <input
          className="w-full bg-editor-bg border border-border rounded px-2 py-1 text-sm text-prime-text placeholder:text-prime-text hover:text-link-text-hover focus:text-link-text-hover"
          placeholder="Search agent..."
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          autoFocus
        />
      </div>

      <div className="max-h-[50vh] overflow-auto">
        {!connected ? (
          <div className="p-4 text-sm text-secondary-text" />
        ) : !hasRoots ? (
          <div className="p-4 text-sm text-secondary-text">No agents directory available</div>
        ) : (
          <>
            {onCustomAgent ? (
              <button
                type="button"
                className="w-full text-left px-4 py-2 border-b border-border text-sm text-secondary-text hover:text-link-text-hover"
                onClick={() => {
                  onCustomAgent();
                  onClose();
                }}
              >
                Add Custom Agent
              </button>
            ) : null}
            {loading ? (
              <div className="p-4 text-sm text-secondary-text">Loading...</div>
            ) : filtered.length === 0 ? (
              <div className="p-4 text-sm text-secondary-text">No agents found</div>
            ) : (
              filtered.map((agent) => (
                <button
                  key={`${agent.agentID}:${agent.dirPath || ''}`}
                  className="group w-full text-left px-4 py-2 border-b border-border text-secondary-text hover:text-link-text-hover"
                  onClick={() => onSelect(agent.agentID)}
                >
                  <div className="text-sm text-secondary-text group-hover:text-link-text-hover">{agent.dirName}</div>
                  <div className="text-xs text-secondary-text group-hover:text-link-text-hover">{agent.dirPath || agent.agentID}</div>
                </button>
              ))
            )}
          </>
        )}
      </div>
    </PopupMenu>,
    document.body,
  );
};
