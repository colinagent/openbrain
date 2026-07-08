export type AddAgentPopupAnchor =
  | { kind: 'point'; x: number; y: number }
  | { kind: 'rect'; rect: Pick<DOMRect, 'left' | 'top' | 'right' | 'bottom'> };

export type AddAgentPopupSize = {
  width: number;
  height: number;
};

export type AddAgentPopupViewport = {
  width: number;
  height: number;
};

export type AddAgentPopupPosition = {
  left: number;
  top: number;
};

const VIEWPORT_MARGIN = 8;
const POINT_OFFSET_X = 8;
const POINT_OFFSET_Y = 8;
const RECT_OFFSET_X = 4;

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(min, value), Math.max(min, max));
}

function placeHorizontal(primaryLeft: number, alternateLeft: number, popupWidth: number, viewportWidth: number) {
  const maxLeft = viewportWidth - popupWidth - VIEWPORT_MARGIN;
  if (primaryLeft + popupWidth <= viewportWidth - VIEWPORT_MARGIN) {
    return clamp(primaryLeft, VIEWPORT_MARGIN, maxLeft);
  }
  if (alternateLeft >= VIEWPORT_MARGIN) {
    return clamp(alternateLeft, VIEWPORT_MARGIN, maxLeft);
  }
  return clamp(primaryLeft, VIEWPORT_MARGIN, maxLeft);
}

function placeVertical(primaryTop: number, alternateTop: number, popupHeight: number, viewportHeight: number) {
  const maxTop = viewportHeight - popupHeight - VIEWPORT_MARGIN;
  if (primaryTop + popupHeight <= viewportHeight - VIEWPORT_MARGIN) {
    return clamp(primaryTop, VIEWPORT_MARGIN, maxTop);
  }
  if (alternateTop >= VIEWPORT_MARGIN) {
    return clamp(alternateTop, VIEWPORT_MARGIN, maxTop);
  }
  return clamp(primaryTop, VIEWPORT_MARGIN, maxTop);
}

export function getAddAgentPopupPosition(
  anchor: AddAgentPopupAnchor,
  popupSize: AddAgentPopupSize,
  viewport: AddAgentPopupViewport,
): AddAgentPopupPosition {
  if (anchor.kind === 'point') {
    return {
      left: placeHorizontal(
        anchor.x + POINT_OFFSET_X,
        anchor.x - popupSize.width - POINT_OFFSET_X,
        popupSize.width,
        viewport.width,
      ),
      top: placeVertical(
        anchor.y,
        anchor.y - popupSize.height - POINT_OFFSET_Y,
        popupSize.height,
        viewport.height,
      ),
    };
  }

  return {
    left: placeHorizontal(
      anchor.rect.right + RECT_OFFSET_X,
      anchor.rect.left - popupSize.width - RECT_OFFSET_X,
      popupSize.width,
      viewport.width,
    ),
    top: placeVertical(
      anchor.rect.top,
      anchor.rect.bottom - popupSize.height,
      popupSize.height,
      viewport.height,
    ),
  };
}
