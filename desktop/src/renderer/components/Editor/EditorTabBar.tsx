import React, { useMemo } from 'react';
import { getEditorDocuments, useAppStore } from '../../store/appStore';
import { CloseButton, PinIcon, PlusIcon } from '../Icons';
import { IconButton } from '../IconButton';
import {
  ACTIVE_TAB_LABEL_CLASS,
  TAB_ITEM_FLEX_STYLE,
  TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS,
  getTabCloseButtonClassName,
  getTabShellClassName,
} from '../tabLayout';

export const EditorTabBar: React.FC = () => {
  const documents = useAppStore((state) => state.documents);
  const tabs = useMemo(() => getEditorDocuments(documents), [documents]);
  const activeTabId = useAppStore((state) => state.activeTabId);
  const pinnedTabId = useAppStore((state) => state.pinnedTabId);
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const closeTab = useAppStore((state) => state.closeTab);
  const openUntitledTab = useAppStore((state) => state.openUntitledTab);

  const handleDoubleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if ((e.target as HTMLElement).closest('[data-tab-item], [data-tabbar-action]')) {
      return;
    }
    openUntitledTab();
  };

  if (tabs.length === 0) {
    return (
      <div
        className="ui-tabbar editor-tabbar flex items-center gap-2 px-2 text-prime-text cursor-pointer"
        onDoubleClick={handleDoubleClick}
      >
        <IconButton
          data-tabbar-action
          className="shrink-0 text-secondary-text"
          onClick={openUntitledTab}
          title="New tab"
          aria-label="New tab"
        >
          <PlusIcon className="w-3.5 h-3.5" />
        </IconButton>
        <div className="flex-1 min-w-0" />
      </div>
    );
  }

  return (
    <div
      className="ui-tabbar editor-tabbar flex items-center gap-2 px-2"
      onDoubleClick={handleDoubleClick}
    >
      <div className="flex min-w-0 items-center overflow-hidden">
        <div className="flex min-w-0 items-center gap-1 overflow-x-auto">
          {tabs.map((tab) => {
            const isActive = tab.id === activeTabId;
            const isPinned = tab.id === pinnedTabId;
            const tabTitle = tab.missing ? `${tab.title} (deleted)` : tab.title;
            const tabTooltip = tab.missing
              ? `${tab.filePath || tab.title}\nFile was deleted on disk`
              : (tab.filePath || tab.title);
            return (
              <div
                key={tab.id}
                data-tab-item
                title={tabTooltip}
                className={getTabShellClassName(isActive, 'cursor-pointer')}
                style={TAB_ITEM_FLEX_STYLE}
                onClick={() => setActiveTab(tab.id)}
              >
                {isPinned && (
                  <PinIcon
                    className={`mr-1 h-3.5 w-3.5 flex-shrink-0 ${isActive ? ACTIVE_TAB_LABEL_CLASS : ''}`}
                  />
                )}
                <span
                  className={`ui-chrome-row-label flex-1 min-w-0 truncate cursor-pointer ${
                    isActive ? ACTIVE_TAB_LABEL_CLASS : 'text-secondary-text'
                  } ${tab.missing ? 'italic opacity-70' : ''}`}
                >
                  {tabTitle}
                </span>
                {tab.isDirty && (
                  <span className={`${isActive ? 'text-button-text' : 'text-secondary-text'} ml-1 flex-shrink-0`}>*</span>
                )}
                <CloseButton
                  className={getTabCloseButtonClassName(
                    'bg-secondary-bg',
                    TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS,
                  )}
                  onClick={(event) => {
                    event.stopPropagation();
                    closeTab(tab.id);
                  }}
                  variant="inline"
                />
              </div>
            );
          })}
        </div>
        <IconButton
          data-tabbar-action
          className="ml-1 shrink-0 text-secondary-text"
          onClick={openUntitledTab}
          title="New tab"
          aria-label="New tab"
        >
          <PlusIcon className="w-3.5 h-3.5" />
        </IconButton>
      </div>
      <div className="flex-1 min-w-0" />
    </div>
  );
};
