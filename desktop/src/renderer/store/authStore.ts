import { create } from 'zustand';
import { clearBillingAccessCache } from '../services/billingAccess';

type DeviceCodeState = {
  userCode: string;
  verificationUri: string;
  expiresAt: number;
} | null;

type UserProfile = {
  uid: string;
  name: string;
  username: string;
  email?: string;
  avatar?: string;
  provider?: string;
};

type LoginOptions = {
  gateway?: string;
  orgSlug?: string;
};

type AuthState = {
  loggedIn: boolean;
  authRevision: number;
  uid: string | undefined;
  email: string | undefined;
  activeOrgID: string | undefined;
  activeOrgName: string | undefined;
  profile: UserProfile | undefined;
  initialized: boolean;
  initializing: boolean;
  deviceCode: DeviceCodeState;
  deviceCodeError: string | null;
  init: () => Promise<void>;
  startLogin: (options?: LoginOptions) => Promise<{ success: boolean; mode?: 'device_code'; loginUrl?: string } | undefined>;
  setActiveOrg: (orgID?: string | null, orgName?: string | null) => Promise<void>;
  logout: () => Promise<void>;
  clearDeviceCode: () => void;
  refreshProfile: () => Promise<void>;
};

const prefetchedAvatarUrls = new Set<string>();
const inflightAvatarImgs = new Map<string, HTMLImageElement>();

function resolveProfileAvatarUrl(profile?: Pick<UserProfile, 'avatar'> | null): string | null {
  const raw = (profile?.avatar || '').trim();
  if (!raw) {
    return null;
  }
  return raw;
}

function warmProfileAvatar(profile?: Pick<UserProfile, 'avatar'> | null): void {
  warmAvatar(resolveProfileAvatarUrl(profile));
}

function warmAvatar(url: string | null | undefined): void {
  const next = (url || '').trim();
  if (!next || prefetchedAvatarUrls.has(next) || inflightAvatarImgs.has(next)) {
    return;
  }
  try {
    const img = new Image();
    img.decoding = 'async';
    img.referrerPolicy = 'no-referrer';
    inflightAvatarImgs.set(next, img);
    const done = (ok: boolean) => {
      inflightAvatarImgs.delete(next);
      if (ok) {
        prefetchedAvatarUrls.add(next);
      }
      // Dev-only: help verify prefetch actually happened.
      if (window.electronAPI?.isDev) {
        // eslint-disable-next-line no-console
        console.debug('[authStore] avatar prefetch', ok ? 'ok' : 'failed', next);
      }
    };
    img.onload = () => done(true);
    img.onerror = () => done(false);
    img.src = next;
    if (typeof img.decode === 'function') {
      void img.decode()
        .then(() => done(true))
        .catch(() => {});
    }
  } catch {
    // ignore prefetch failures
  }
}

export const useAuthStore = create<AuthState>((set, get) => ({
  loggedIn: false,
  authRevision: 0,
  uid: undefined,
  email: undefined,
  activeOrgID: undefined,
  activeOrgName: undefined,
  profile: undefined,
  initialized: false,
  initializing: false,
  deviceCode: null,
  deviceCodeError: null,

  init: async () => {
    if (get().initialized || get().initializing) return;
    set({ initializing: true });

    // Load initial auth state from main process
    if (window.electronAPI?.auth) {
      const authApi = window.electronAPI.auth;
      // Subscribe to auth changes (e.g., from deep link callback)
      authApi.onChanged((payload) => {
        clearBillingAccessCache();
        set((state) => ({
          loggedIn: payload.loggedIn,
          authRevision: state.authRevision + 1,
          uid: payload.uid,
          email: payload.email,
          activeOrgID: payload.activeOrgID,
          activeOrgName: payload.activeOrgName,
          profile: payload.profile,
          deviceCode: null, // Clear device code on auth change
          deviceCodeError: null,
        }));
        warmProfileAvatar(payload.profile);
      });

      // Subscribe to device code events (dev mode)
      window.electronAPI.auth.onDeviceCode?.((payload) => {
        set({
          deviceCode: {
            userCode: payload.userCode,
            verificationUri: payload.verificationUri,
            expiresAt: payload.expiresAt,
          },
          deviceCodeError: null,
        });
      });

      window.electronAPI.auth.onDeviceCodeComplete?.((payload) => {
        set({
          deviceCode: null,
          deviceCodeError: payload.success ? null : payload.error || 'Sign in failed.',
        });
        if (payload.success) {
          void authApi.get()
            .then((auth) => {
              if (!auth) {
                return;
              }
              clearBillingAccessCache();
              set((state) => ({
                loggedIn: true,
                authRevision: state.authRevision + 1,
                uid: auth.uid,
                email: auth.email,
                activeOrgID: auth.activeOrgID,
                activeOrgName: auth.activeOrgName,
                profile: auth.profile,
              }));
              warmProfileAvatar(auth.profile);
            })
            .catch(() => {});
        }
      });

      try {
        const auth = await authApi.get();
        if (auth) {
          set({
            loggedIn: true,
            authRevision: get().authRevision + 1,
            uid: auth.uid,
            email: auth.email,
            activeOrgID: auth.activeOrgID,
            activeOrgName: auth.activeOrgName,
            profile: auth.profile,
          });
          warmProfileAvatar(auth.profile);

          // Ensure profile is available on app startup without requiring opening Profiles.
          // Prefer local disk profile first; only hit network if missing.
          if (!auth.profile && window.electronAPI?.profile?.get) {
            try {
              const diskProfile = await window.electronAPI.profile.get();
              if (diskProfile) {
                set({ profile: diskProfile });
                warmProfileAvatar(diskProfile);
              }
            } catch {
              // Verification below will also refresh the profile.
            }
          }
          // Local auth files are only a cache. Verify the token in the
          // background so expired sessions fall back to signed-out state.
          void get().refreshProfile();
        }
      } catch {
        // ignore auth load errors, we can still log in later
      }
      set({ initialized: true, initializing: false });
    } else {
      set({ initialized: true, initializing: false });
    }
  },

  startLogin: async (options?: LoginOptions) => {
    set({ deviceCodeError: null });
    if (window.electronAPI?.auth) {
      return window.electronAPI.auth.startLogin(options);
    }
    return undefined;
  },

  logout: async () => {
    if (window.electronAPI?.auth) {
      await window.electronAPI.auth.logout();
    }
    clearBillingAccessCache();
    set((state) => ({ loggedIn: false, authRevision: state.authRevision + 1, uid: undefined, email: undefined, activeOrgID: undefined, activeOrgName: undefined, profile: undefined, deviceCode: null, deviceCodeError: null }));
    prefetchedAvatarUrls.clear();
    inflightAvatarImgs.clear();
  },

  setActiveOrg: async (orgID?: string | null, orgName?: string | null) => {
    if (!window.electronAPI?.auth?.setActiveOrg) {
      return;
    }
    const result = await window.electronAPI.auth.setActiveOrg(orgID, orgName);
    if (!result?.success) {
      throw new Error(result?.error || 'Failed to set organization');
    }
    set({
      activeOrgID: result.activeOrgID,
      activeOrgName: result.activeOrgName,
    });
  },

  clearDeviceCode: () => {
    set({ deviceCode: null });
  },

  refreshProfile: async () => {
    if (window.electronAPI?.profile) {
      try {
        const result = await window.electronAPI.profile.refresh();
        if (result.authInvalid) {
          clearBillingAccessCache();
          set((state) => ({ loggedIn: false, authRevision: state.authRevision + 1, uid: undefined, email: undefined, activeOrgID: undefined, activeOrgName: undefined, profile: undefined, deviceCode: null, deviceCodeError: null }));
          return;
        }
        if (result.success && result.profile) {
          set((state) => ({
            authRevision: state.authRevision + 1,
            profile: result.profile,
            activeOrgID: result.activeOrgID,
            activeOrgName: result.activeOrgName,
          }));
          warmProfileAvatar(result.profile);
        }
      } catch {
        // ignore profile refresh errors
      }
    }
  },
}));
