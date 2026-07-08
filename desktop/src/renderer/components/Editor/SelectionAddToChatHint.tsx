import React from 'react';
import { createPortal } from 'react-dom';

type SelectionAddToChatHintProps = {
  position: { left: number; top: number } | null;
  shortcutLabel: string;
  onClick: () => void;
};

export const SelectionAddToChatHint: React.FC<SelectionAddToChatHintProps> = ({
  position,
  shortcutLabel,
  onClick,
}) => {
  if (!position || typeof document === 'undefined') {
    return null;
  }

  return createPortal(
    <button
      type="button"
      className="op-selection-add-chat-hint"
      style={{ left: `${position.left}px`, top: `${position.top}px` }}
      onMouseDown={(event) => {
        event.preventDefault();
        event.stopPropagation();
      }}
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onClick();
      }}
    >
      <span>Add to Chat</span>
      <span className="op-selection-add-chat-hint-key">{shortcutLabel}</span>
    </button>,
    document.body,
  );
};
