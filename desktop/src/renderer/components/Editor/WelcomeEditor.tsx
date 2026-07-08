import React from 'react';
import { BrandMark } from '../Brand/BrandMark';

type WelcomeEditorProps = {
  chatPanelBottomInset?: number;
  chatPanelOpen?: boolean;
};

export const WelcomeEditor: React.FC<WelcomeEditorProps> = ({
  chatPanelBottomInset = 0,
  chatPanelOpen = false,
}) => {
  const translateY = chatPanelBottomInset > 0
    ? (chatPanelOpen ? 0 : -chatPanelBottomInset / 2)
    : 0;

  return (
    <div className="flex-1 flex items-center justify-center">
      <div style={translateY !== 0 ? { transform: `translateY(${translateY}px)` } : undefined}>
        <BrandMark />
      </div>
    </div>
  );
};
