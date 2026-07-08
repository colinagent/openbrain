import React, { useId, useMemo, useRef } from 'react';
import { useUiStore } from '../../store/uiStore';
import {
  buildOpenBrainLogoGradientStops,
  OPENBRAIN_LOGO_GRADIENT,
} from '../Brand/openBrainLogoGradient';
import { useLogoTailBg } from './useLogoTailBg';

type OpenBrainLogoProps = {
  className?: string;
  monochrome?: boolean;
  tailColor?: string;
  title?: string;
};

const LOGO_PATH =
  'm314.044 208.564 125.905-125.906c7.358 5.244 16.347 8.342 26.051 8.342 24.814 0 45-20.187 45-45s-20.186-45-45-45-45 20.187-45 45c0 9.704 3.098 18.692 8.342 26.051l-125.906 125.905c-21.252-17.4-49.181-21.149-73.053-12.447l-49.192-102.179c8.992-6.857 14.809-17.675 14.809-29.83 0-20.678-16.822-37.5-37.5-37.5s-37.5 16.822-37.5 37.5c0 24.326 22.972 42.35 46.686 36.358l49.193 102.182c-19.49 11.965-33.121 32.579-35.503 56.46h-75.92c-3.652-25.407-25.554-45-51.956-45-28.948 0-52.5 23.552-52.5 52.5s23.552 52.5 52.5 52.5c26.401 0 48.304-19.593 51.956-45h75.92c2.129 21.346 13.247 40.081 29.502 52.363l-65.772 108.326c-5.142-2.049-10.742-3.189-16.606-3.189-24.813 0-45 20.186-45 45s20.187 45 45 45 45-20.186 45-45c0-13.573-6.048-25.752-15.581-34.009l65.768-108.319c26.371 12.644 58.207 8.865 81.076-10.757l73.497 70.156c-6.135 8.591-9.759 19.092-9.759 30.43 0 28.948 23.552 52.5 52.5 52.5s52.5-23.552 52.5-52.5-23.553-52.501-52.501-52.501c-12.212 0-23.454 4.203-32.382 11.222l-73.497-70.157c8.487-10.868 14.061-24.113 15.503-38.565h68.631c3.484 17.096 18.635 30 36.745 30 20.678 0 37.5-16.822 37.5-37.5s-16.822-37.5-37.5-37.5c-18.11 0-33.261 12.904-36.745 30h-68.631c-1.502-15.062-7.481-28.823-16.58-39.936zM136 53.5c0-12.406 10.094-22.5 22.5-22.5s22.5 10.094 22.5 22.5-10.094 22.5-22.5 22.5-22.5-10.094-22.5-22.5zM53.5 293.5c-20.678 0-37.5-16.822-37.5-37.5s16.822-37.5 37.5-37.5 37.5 16.822 37.5 37.5-16.822 37.5-37.5 37.5zM458.5 413.5c0 20.678-16.822 37.5-37.5 37.5s-37.5-16.822-37.5-37.5 16.822-37.5 37.5-37.5 37.5 16.822 37.5 37.5zM436 233.5c12.406 0 22.5 10.094 22.5 22.5s-10.094 22.5-22.5 22.5-22.5-10.094-22.5-22.5 10.094-22.5 22.5-22.5z';

export function OpenBrainLogo({
  className = 'h-4 w-4',
  monochrome = false,
  tailColor,
  title,
}: OpenBrainLogoProps) {
  const gradientId = `${useId().replace(/:/g, '')}-openbrain-gradient`;
  const rootRef = useRef<SVGSVGElement>(null);
  const scheme = useUiStore((state) => state.theme.scheme);
  const direction = scheme === 'dark' ? 'darken' : 'lighten';
  const resolvedTailBg = useLogoTailBg(rootRef, direction, tailColor);
  const gradientStops = useMemo(
    () => buildOpenBrainLogoGradientStops(resolvedTailBg, direction),
    [direction, resolvedTailBg],
  );

  return (
    <svg
      ref={rootRef}
      className={className}
      viewBox="0 0 512 512"
      role={title ? 'img' : undefined}
      aria-hidden={title ? undefined : true}
    >
      {title ? <title>{title}</title> : null}
      {!monochrome ? (
        <defs>
          <linearGradient
            id={gradientId}
            x1={OPENBRAIN_LOGO_GRADIENT.x1}
            y1={OPENBRAIN_LOGO_GRADIENT.y1}
            x2={OPENBRAIN_LOGO_GRADIENT.x2}
            y2={OPENBRAIN_LOGO_GRADIENT.y2}
            gradientUnits="userSpaceOnUse"
          >
            {gradientStops.map((stop) => (
              <stop key={stop.offset} offset={stop.offset} stopColor={stop.color} />
            ))}
          </linearGradient>
        </defs>
      ) : null}
      <path
        fill={monochrome ? 'currentColor' : `url(#${gradientId})`}
        d={LOGO_PATH}
      />
    </svg>
  );
}
