import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import { useAppStore } from '../../store/appStore';
import { useUiStore } from '../../store/uiStore';
import { CloseIcon, SparkleIcon } from '../Icons';
import { IconButton } from '../IconButton';
import {
  getWorkspaceAgentOnboardingPosition,
  type OnboardingRect,
  type OnboardingSize,
  type OnboardingViewport,
} from './workspaceAgentOnboardingPosition';
import {
  findWorkspaceAgentOnboardingTarget,
  getPaddedOnboardingRect,
  readWorkspaceAgentOnboardingSeen,
  toOnboardingRect,
  type OnboardingSettingsSnapshot,
} from './workspaceAgentOnboardingDom';

const CARD_FALLBACK_SIZE: OnboardingSize = { width: 288, height: 138 };

export function WorkspaceAgentOnboarding() {
  const connected = useAppStore((state) => state.connectionState === 'connected');
  const currentDir = useAppStore((state) => state.currentDir);
  const agentBindingCount = useAppStore((state) => state.agentBindingByCwd.size);
  const nodeCount = useAppStore((state) => state.nodesByID.size);
  const hasBlockingModal = useUiStore((state) => state.hasBlockingModal);

  const [seen, setSeen] = useState<boolean | null>(null);
  const [target, setTarget] = useState<HTMLElement | null>(null);
  const [targetRect, setTargetRect] = useState<OnboardingRect | null>(null);
  const [cardSize, setCardSize] = useState<OnboardingSize>(CARD_FALLBACK_SIZE);
  const cardRef = useRef<HTMLDivElement | null>(null);
  const dismissingRef = useRef(false);
  const active = seen === false && connected && Boolean(currentDir) && !hasBlockingModal;

  const dismiss = useCallback(() => {
    if (dismissingRef.current) {
      return;
    }
    dismissingRef.current = true;
    setSeen(true);
    setTarget(null);
    setTargetRect(null);
    const saveSeen = window.electronAPI?.settings?.set?.({
      ui: { workspaceAgentOnboardingSeen: true },
    });
    void saveSeen?.catch(() => {});
  }, []);

  useEffect(() => {
    let disposed = false;
    const settingsApi = window.electronAPI?.settings;
    if (!settingsApi?.get) {
      setSeen(false);
      return undefined;
    }

    settingsApi.get()
      .then((settings) => {
        if (!disposed) {
          setSeen(readWorkspaceAgentOnboardingSeen(settings as OnboardingSettingsSnapshot));
        }
      })
      .catch(() => {
        if (!disposed) {
          setSeen(false);
        }
      });

    const disposeSettingsChanged = settingsApi.onChanged?.((settings) => {
      if (!disposed) {
        setSeen(readWorkspaceAgentOnboardingSeen(settings as OnboardingSettingsSnapshot));
      }
    });

    return () => {
      disposed = true;
      disposeSettingsChanged?.();
    };
  }, []);

  useLayoutEffect(() => {
    if (!active) {
      setTarget(null);
      setTargetRect(null);
      return undefined;
    }

    let frame = 0;
    const syncTarget = () => {
      const nextTarget = findWorkspaceAgentOnboardingTarget();
      setTarget(nextTarget);
      setTargetRect(nextTarget ? toOnboardingRect(nextTarget.getBoundingClientRect()) : null);
    };
    const scheduleSync = () => {
      if (frame) {
        cancelAnimationFrame(frame);
      }
      frame = requestAnimationFrame(syncTarget);
    };

    scheduleSync();
    window.addEventListener('resize', scheduleSync);
    window.addEventListener('scroll', scheduleSync, true);

    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(scheduleSync);
    if (observer) {
      observer.observe(document.body);
    }

    return () => {
      if (frame) {
        cancelAnimationFrame(frame);
      }
      window.removeEventListener('resize', scheduleSync);
      window.removeEventListener('scroll', scheduleSync, true);
      observer?.disconnect();
    };
  }, [active, agentBindingCount, currentDir, nodeCount]);

  useLayoutEffect(() => {
    if (!target || typeof ResizeObserver === 'undefined') {
      return undefined;
    }
    const observer = new ResizeObserver(() => {
      setTargetRect(toOnboardingRect(target.getBoundingClientRect()));
    });
    observer.observe(target);
    return () => observer.disconnect();
  }, [target]);

  useLayoutEffect(() => {
    const card = cardRef.current;
    if (!card || typeof ResizeObserver === 'undefined') {
      return undefined;
    }
    const observer = new ResizeObserver(() => {
      const rect = card.getBoundingClientRect();
      setCardSize({ width: rect.width, height: rect.height });
    });
    observer.observe(card);
    const rect = card.getBoundingClientRect();
    if (rect.width > 0 && rect.height > 0) {
      setCardSize({ width: rect.width, height: rect.height });
    }
    return () => observer.disconnect();
  }, [targetRect]);

  useEffect(() => {
    if (!targetRect) {
      document.body.classList.remove('workspace-agent-onboarding-active');
      return undefined;
    }
    document.body.classList.add('workspace-agent-onboarding-active');
    return () => {
      document.body.classList.remove('workspace-agent-onboarding-active');
    };
  }, [targetRect]);

  useEffect(() => {
    if (!target || !targetRect) {
      return undefined;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        dismiss();
      }
    };
    const handlePointerDown = (event: PointerEvent) => {
      if (target.contains(event.target as Node | null)) {
        window.setTimeout(dismiss, 0);
      }
    };
    document.addEventListener('keydown', handleKeyDown, true);
    document.addEventListener('pointerdown', handlePointerDown, true);
    return () => {
      document.removeEventListener('keydown', handleKeyDown, true);
      document.removeEventListener('pointerdown', handlePointerDown, true);
    };
  }, [dismiss, target, targetRect]);

  const viewport = useMemo<OnboardingViewport>(() => ({
    width: window.innerWidth,
    height: window.innerHeight,
  }), [targetRect]);

  if (!targetRect) {
    return null;
  }

  const paddedRect = getPaddedOnboardingRect(targetRect, viewport);
  const cardPosition = getWorkspaceAgentOnboardingPosition(paddedRect, cardSize, viewport);

  return createPortal(
    <div className="workspace-agent-onboarding" aria-live="polite">
      <div className="workspace-agent-onboarding-scrim top" style={{ height: `${paddedRect.top}px` }} />
      <div
        className="workspace-agent-onboarding-scrim left"
        style={{
          top: `${paddedRect.top}px`,
          width: `${paddedRect.left}px`,
          height: `${paddedRect.height}px`,
        }}
      />
      <div
        className="workspace-agent-onboarding-scrim right"
        style={{
          top: `${paddedRect.top}px`,
          left: `${paddedRect.right}px`,
          width: `${Math.max(0, viewport.width - paddedRect.right)}px`,
          height: `${paddedRect.height}px`,
        }}
      />
      <div
        className="workspace-agent-onboarding-scrim bottom"
        style={{
          top: `${paddedRect.bottom}px`,
          height: `${Math.max(0, viewport.height - paddedRect.bottom)}px`,
        }}
      />
      <div
        className="workspace-agent-onboarding-ring"
        style={{
          left: `${paddedRect.left}px`,
          top: `${paddedRect.top}px`,
          width: `${paddedRect.width}px`,
          height: `${paddedRect.height}px`,
        }}
      />
      <div
        ref={cardRef}
        className={`workspace-agent-onboarding-card placement-${cardPosition.placement}`}
        style={{ left: `${cardPosition.left}px`, top: `${cardPosition.top}px` }}
        role="dialog"
        aria-label="Workspace Agent guide"
      >
        <div className="flex items-start gap-2">
          <div className="workspace-agent-onboarding-icon">
            <SparkleIcon className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold text-prime-text">Start with this Agent</div>
            <div className="mt-1 text-xs leading-5 text-secondary-text">
              Click the workspace bubble to open chat here. Type your message in the conversation bar.
            </div>
          </div>
          <IconButton
            variant="inline"
            size={22}
            className="workspace-agent-onboarding-close text-secondary-text"
            onClick={dismiss}
            title="Close guide"
            aria-label="Close guide"
          >
            <CloseIcon className="h-3.5 w-3.5" />
          </IconButton>
        </div>
      </div>
    </div>,
    document.body,
  );
}
