interface LogoProps {
  className?: string;
  width?: number | string;
  height?: number | string;
}

export function Logo({ className = '', width = 24, height = 24 }: LogoProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 48 48"
      className={className}
      width={width}
      height={height}
    >
      <rect fill="currentColor" x="6" y="6" width="36" height="8" rx="2" />
      <rect fill="currentColor" x="6" y="20" width="15" height="8" rx="2" />
      <rect fill="currentColor" x="27" y="20" width="15" height="8" rx="2" />
      <rect fill="currentColor" x="6" y="34" width="36" height="8" rx="2" />
    </svg>
  );
}
