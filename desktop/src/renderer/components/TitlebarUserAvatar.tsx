import React, { useEffect, useMemo, useState } from 'react';
import { UserIcon } from './Icons';
import { buildInitials, initialsBackgroundColor } from './avatarInitials';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_EDITOR,
  OP_SG_CAPSULE_ON_TITLEBAR,
} from './staticGlassCapsule';

type UserProfileLike = {
  name?: string;
  username?: string;
  email?: string;
  avatar?: string;
};

type TitlebarUserAvatarProps = {
  loggedIn: boolean;
  profile?: UserProfileLike;
  email?: string;
  uid?: string;
  size?: 'titlebar' | 'menu' | 'profile';
};

export function resolveUserAvatarSrc(profile?: UserProfileLike): string | null {
  return (profile?.avatar || '').trim() || null;
}

function resolveDisplayName(
  profile?: UserProfileLike,
  email?: string,
  uid?: string,
): string {
  return (profile?.name || profile?.username || profile?.email || email || uid || '').trim();
}

export function TitlebarUserAvatar({
  loggedIn,
  profile,
  email,
  uid,
  size = 'titlebar',
}: TitlebarUserAvatarProps) {
  const [imageFailed, setImageFailed] = useState(false);
  const avatarSrc = useMemo(() => resolveUserAvatarSrc(profile), [profile]);
  const displayName = useMemo(
    () => resolveDisplayName(profile, email, uid),
    [profile, email, uid],
  );

  useEffect(() => {
    setImageFailed(false);
  }, [avatarSrc]);

  const surfaceClass = size === 'profile'
    ? OP_SG_CAPSULE_ON_EDITOR
    : OP_SG_CAPSULE_ON_TITLEBAR;

  const capsuleClass = [
    OP_SG_CAPSULE,
    surfaceClass,
    'titlebar-user-avatar',
    size === 'menu' ? 'titlebar-user-avatar--menu' : '',
    size === 'profile' ? 'titlebar-user-avatar--profile' : '',
  ]
    .filter(Boolean)
    .join(' ');

  if (!loggedIn) {
    return (
      <span className={capsuleClass} aria-hidden="true">
        <UserIcon className="titlebar-user-avatar__icon" />
      </span>
    );
  }

  if (avatarSrc && !imageFailed) {
    return (
      <span className={capsuleClass} aria-hidden="true">
        <img
          src={avatarSrc}
          alt=""
          className="titlebar-user-avatar__img"
          onError={() => setImageFailed(true)}
        />
      </span>
    );
  }

  const initials = buildInitials(displayName || 'User');
  const backgroundColor = initialsBackgroundColor(displayName || 'User');

  return (
    <span className={capsuleClass} aria-hidden="true">
      <span
        className="titlebar-user-avatar__initials"
        style={{ backgroundColor }}
      >
        {initials}
      </span>
    </span>
  );
}
