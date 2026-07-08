import React from 'react';

import { OP_POPUP_MENU } from './staticGlassCapsule';

type PopupMenuProps = {
  /** Plain overlay panel — no static-glass frost surface. */
  plain?: boolean;
} & React.HTMLAttributes<HTMLDivElement>;

type PopupMenuItemProps = {
  active?: boolean;
  highlighted?: boolean;
} & React.ButtonHTMLAttributes<HTMLButtonElement>;

export const PopupMenu = React.forwardRef<HTMLDivElement, PopupMenuProps>(
  ({ plain = false, className, children, ...props }, ref) => {
    const classes = [
      plain ? 'border border-border bg-overlay-bg' : OP_POPUP_MENU,
      'rounded-lg',
      'p-1',
      className,
    ]
      .filter(Boolean)
      .join(' ');

    return (
      <div ref={ref} className={classes} {...props}>
        {children}
      </div>
    );
  }
);

PopupMenu.displayName = 'PopupMenu';

export const PopupMenuItem = React.forwardRef<HTMLButtonElement, PopupMenuItemProps>(
  ({ active = false, highlighted = false, className, children, type, ...props }, ref) => {
    const stateClass = active
      ? 'text-highlight font-medium hover:bg-hover-bg'
      : highlighted
        ? 'bg-hover-bg text-prime-text'
        : 'text-secondary-text hover:bg-hover-bg hover:text-prime-text';

    const classes = [
      'w-full',
      'px-2',
      'py-1.5',
      'text-left',
      'rounded',
      'flex',
      'items-center',
      'gap-2',
      'text-sm',
      'transition-[color,background-color]',
      stateClass,
      'disabled:text-tertiary-text',
      'disabled:cursor-not-allowed',
      'disabled:hover:bg-transparent',
      className,
    ]
      .filter(Boolean)
      .join(' ');

    return (
      <button ref={ref} type={type ?? 'button'} className={classes} {...props}>
        {children}
      </button>
    );
  }
);

PopupMenuItem.displayName = 'PopupMenuItem';

export function PopupMenuSeparator() {
  return <div className="h-px bg-border my-1 mx-1" />;
}
