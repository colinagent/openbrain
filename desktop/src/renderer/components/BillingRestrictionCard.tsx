import React from 'react';

import { useAppStore } from '../store/appStore';
import {
  BILLING_URL,
  type ActivityErrorInfo,
} from './Chat/activityErrorState';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_EDITOR,
  OP_SG_CAPSULE_ON_SIDEBAR,
  UI_PILL_BTN_FIT,
} from './staticGlassCapsule';

export type BillingRestrictionCardSurface = 'editor' | 'sidebar' | 'none';

export type BillingRestrictionCardProps = {
  info: ActivityErrorInfo;
  /**
   * Compact layout for narrow surfaces (cron sidebar rows, sync popup menus).
   * Renders a single one-line row — title on the left, primary action on the
   * right — so it survives a ~240px column without wrapping into a vertical
   * lump. The title alone carries the message; the long body paragraph is
   * dropped because it is redundant with the title + action button.
   */
  compact?: boolean;
  /**
   * Static-glass surface modifier for the primary button. The Activity Panel
   * and Cron task editor live on editor surfaces; cron sidebar rows live on a
   * sidebar surface; sync popup menus already provide their own frost surface
   * so no extra capsule class is needed there.
   */
  surface?: BillingRestrictionCardSurface;
};

function primaryButtonClass(surface: BillingRestrictionCardSurface, compact: boolean): string {
  const surfaceClass = surface === 'sidebar'
    ? `${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_SIDEBAR}`
    : surface === 'editor'
      ? `${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_EDITOR}`
      : '';
  if (compact) {
    return `ui-pill-btn-primary ${surfaceClass} ${UI_PILL_BTN_FIT} px-2.5 py-1 text-xs`;
  }
  return `ui-pill-btn-primary ${surfaceClass} px-3 py-1.5 text-sm`;
}

export function BillingRestrictionCard({
  info,
  compact = false,
  surface = 'editor',
}: BillingRestrictionCardProps) {
  const openModelsTab = useAppStore((state) => state.openModelsTab);
  const handleOpenBilling = () => {
    window.open(BILLING_URL, '_blank', 'noopener,noreferrer');
  };
  const handleOpenModels = () => {
    openModelsTab();
  };
  const canOpenModels = info.kind === 'bundled-token-required';

  if (compact) {
    return (
      <div className="flex items-center gap-2 rounded border border-accent/35 bg-accent/10 px-2.5 py-1.5">
        <div className="min-w-0 flex-1 truncate text-xs font-medium text-prime-text">
          {info.title}
        </div>
        <button
          type="button"
          className={`${primaryButtonClass(surface, true)} shrink-0`}
          onClick={handleOpenBilling}
        >
          {info.actionLabel}
        </button>
      </div>
    );
  }

  return (
    <div className="rounded-md border border-accent/30 bg-accent/5 px-3 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-xs uppercase tracking-wide text-accent">Billing</div>
          <div className="mt-1 text-sm font-medium text-prime-text">{info.title}</div>
          <p className="mt-1 text-sm leading-6 text-secondary-text">
            {info.message}
          </p>
        </div>
        <div className="flex shrink-0 flex-col gap-2">
          {canOpenModels && info.secondaryActionLabel ? (
            <button
              type="button"
              className="ui-pill-btn-secondary px-3 py-1.5 text-sm font-medium"
              onClick={handleOpenModels}
            >
              {info.secondaryActionLabel}
            </button>
          ) : null}
          <button
            type="button"
            className={primaryButtonClass(surface, false)}
            onClick={handleOpenBilling}
          >
            {info.actionLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
