import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { Tokens } from '@bookdan/api-client';
import type { Identity } from '@bookdan/domain';
import { api, bindSession, queryClient } from './api';
import { useOffline } from './offline';
import { secureStorage } from './secureStorage';
import { usePrefs } from './store';

interface AuthState {
  token: string | null;
  refreshToken: string | null;
  identity: Identity | null; // how the staff member signed in: phone or email
  signIn: (tokens: Tokens, identity: Identity) => void;
  setTokens: (tokens: Tokens) => void;
  signOut: () => void;
}

export const useAuth = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      refreshToken: null,
      identity: null,
      signIn: (tokens, identity) => {
        api.setToken(tokens.token);
        set({ token: tokens.token, refreshToken: tokens.refresh_token, identity });
      },
      setTokens: (tokens) => {
        api.setToken(tokens.token);
        set({ token: tokens.token, refreshToken: tokens.refresh_token });
      },
      signOut: () => {
        api.setToken(null);
        set({ token: null, refreshToken: null, identity: null });
        queryClient.clear(); // nothing of the hotel's stays on a shared front-desk phone
        usePrefs.getState().setOwner(null); // the next person to sign in may have another role
        // Check-ins waiting to be sent are kept (they would be lost); the rest of the offline cache goes with the session.
        if (useOffline.getState().queue.length === 0) useOffline.getState().wipe();
      },
    }),
    {
      name: 'partner-auth',
      version: 1,
      storage: createJSONStorage(() => secureStorage), // tokens live in the phone's secure store
      onRehydrateStorage: () => (state) => api.setToken(state?.token ?? null),
    },
  ),
);

bindSession({
  getRefreshToken: () => useAuth.getState().refreshToken,
  setTokens: (tokens) => useAuth.getState().setTokens(tokens),
  // The session ended for good: sign out, and the navigator shows the login screen.
  expire: () => useAuth.getState().signOut(),
});
