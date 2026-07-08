import React from 'react';
import { useTranslation } from 'react-i18next';
import { BrandMark } from '../Brand/BrandMark';
import { AgentCloudIcon, FolderIcon } from '../Icons';

type NewWindowLandingProps = {
  openingFolder?: boolean;
  onOpenFolder: () => void;
  onConnectRemote: () => void;
};

export const NewWindowLanding: React.FC<NewWindowLandingProps> = ({
  openingFolder = false,
  onOpenFolder,
  onConnectRemote,
}) => {
  const { t } = useTranslation('shell');
  const LANDING_SHIFT_PX = 104;

  return (
    <div className="h-full w-full flex items-center justify-center bg-editor-bg text-editor-fg">
      <div
        className="flex flex-col items-center gap-7"
        style={{ transform: `translateY(-${LANDING_SHIFT_PX}px)` }}
      >
        <BrandMark />
        <div className="flex items-center gap-4">
          <button
            type="button"
            className="inline-flex items-center justify-center gap-2 min-w-[190px] h-12 rounded border border-border px-5 text-base text-secondary-text hover:text-link-text-hover disabled:opacity-50 disabled:cursor-not-allowed"
            onClick={onOpenFolder}
            disabled={openingFolder}
          >
            <FolderIcon className="w-5 h-5" />
            <span>{t('landing.openFolder')}</span>
          </button>
          <button
            type="button"
            className="inline-flex items-center justify-center gap-2 min-w-[190px] h-12 rounded border border-border px-5 text-base text-secondary-text hover:text-link-text-hover"
            onClick={onConnectRemote}
          >
            <AgentCloudIcon className="w-5 h-5" />
            <span>{t('landing.connectRemote')}</span>
          </button>
        </div>
      </div>
    </div>
  );
};
