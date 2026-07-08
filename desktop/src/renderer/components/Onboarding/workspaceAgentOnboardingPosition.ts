export type OnboardingRect = {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
};

export type OnboardingSize = {
  width: number;
  height: number;
};

export type OnboardingViewport = {
  width: number;
  height: number;
};

export type OnboardingPlacement = 'right' | 'left' | 'bottom' | 'top';

export type OnboardingPosition = {
  left: number;
  top: number;
  placement: OnboardingPlacement;
};

const EDGE_GAP = 8;
const TARGET_GAP = 12;

function clamp(value: number, min: number, max: number): number {
  if (max < min) {
    return min;
  }
  return Math.min(max, Math.max(min, value));
}

export function getWorkspaceAgentOnboardingPosition(
  target: OnboardingRect,
  card: OnboardingSize,
  viewport: OnboardingViewport,
): OnboardingPosition {
  const maxLeft = viewport.width - card.width - EDGE_GAP;
  const maxTop = viewport.height - card.height - EDGE_GAP;
  const centeredTop = target.top + target.height / 2 - card.height / 2;
  const centeredLeft = target.left + target.width / 2 - card.width / 2;

  if (viewport.width - target.right >= card.width + TARGET_GAP + EDGE_GAP) {
    return {
      left: target.right + TARGET_GAP,
      top: clamp(centeredTop, EDGE_GAP, maxTop),
      placement: 'right',
    };
  }

  if (target.left >= card.width + TARGET_GAP + EDGE_GAP) {
    return {
      left: target.left - card.width - TARGET_GAP,
      top: clamp(centeredTop, EDGE_GAP, maxTop),
      placement: 'left',
    };
  }

  if (viewport.height - target.bottom >= card.height + TARGET_GAP + EDGE_GAP) {
    return {
      left: clamp(centeredLeft, EDGE_GAP, maxLeft),
      top: target.bottom + TARGET_GAP,
      placement: 'bottom',
    };
  }

  if (target.top >= card.height + TARGET_GAP + EDGE_GAP) {
    return {
      left: clamp(centeredLeft, EDGE_GAP, maxLeft),
      top: target.top - card.height - TARGET_GAP,
      placement: 'top',
    };
  }

  return {
    left: clamp(target.right + TARGET_GAP, EDGE_GAP, maxLeft),
    top: clamp(centeredTop, EDGE_GAP, maxTop),
    placement: 'right',
  };
}
