import { OpenBrainLogo } from '../Icons';

const PRODUCT_NAME = 'OPENBRAIN';
const PRODUCT_TAGLINE = 'A GUI and agent runtime for GBrain';

export function BrandMark() {
  return (
    <div className="flex flex-col items-center gap-5">
      <OpenBrainLogo
        className="op-brand-mark-fg block h-28 w-28 shrink-0"
        monochrome
        title="OpenBrain"
      />
      <div className="flex flex-col items-center gap-2">
        <h1 className="op-brand-mark-fg op-brand-mark-title m-0 text-center text-2xl">
          {PRODUCT_NAME}
        </h1>
        <p className="op-brand-mark-fg m-0 whitespace-nowrap text-center text-base leading-snug">
          {PRODUCT_TAGLINE}
        </p>
      </div>
    </div>
  );
}
