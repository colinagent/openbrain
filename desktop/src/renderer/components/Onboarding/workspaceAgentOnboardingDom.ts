import type { OnboardingRect, OnboardingViewport } from './workspaceAgentOnboardingPosition';

const DIR_TARGET_SELECTOR = '[data-onboarding-target="workspace-dir-agent-pill"]';
const SPOTLIGHT_PADDING = 8;

export type OnboardingSettingsSnapshot = {
  ui?: {
    workspaceAgentOnboardingSeen?: unknown;
  };
};

export function readWorkspaceAgentOnboardingSeen(
  settings: OnboardingSettingsSnapshot | null | undefined,
): boolean {
  return settings?.ui?.workspaceAgentOnboardingSeen === true;
}

function isVisibleTarget(element: Element): element is HTMLElement {
  if (!(element instanceof HTMLElement)) {
    return false;
  }
  const rect = element.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) {
    return false;
  }
  if (rect.bottom < 0 || rect.top > window.innerHeight || rect.right < 0 || rect.left > window.innerWidth) {
    return false;
  }
  const style = window.getComputedStyle(element);
  return style.display !== 'none' && style.visibility !== 'hidden';
}

export function findWorkspaceAgentOnboardingTarget(): HTMLElement | null {
  for (const selector of [DIR_TARGET_SELECTOR]) {
    const targets = Array.from(document.querySelectorAll(selector));
    const target = targets.find(isVisibleTarget);
    if (target) {
      return target;
    }
  }
  return null;
}

export function toOnboardingRect(rect: DOMRect): OnboardingRect {
  return {
    left: rect.left,
    top: rect.top,
    right: rect.right,
    bottom: rect.bottom,
    width: rect.width,
    height: rect.height,
  };
}

export function getPaddedOnboardingRect(rect: OnboardingRect, viewport: OnboardingViewport): OnboardingRect {
  const left = Math.max(0, rect.left - SPOTLIGHT_PADDING);
  const top = Math.max(0, rect.top - SPOTLIGHT_PADDING);
  const right = Math.min(viewport.width, rect.right + SPOTLIGHT_PADDING);
  const bottom = Math.min(viewport.height, rect.bottom + SPOTLIGHT_PADDING);
  return {
    left,
    top,
    right,
    bottom,
    width: Math.max(0, right - left),
    height: Math.max(0, bottom - top),
  };
}
