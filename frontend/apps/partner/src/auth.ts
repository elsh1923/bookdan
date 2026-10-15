import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { Tokens } from '@bookdan/api-client';
import type { Identity } from '@bookdan/domain';
import { api, bindSession, queryClient } from './api';
import { secureStorage } from './secureStorage';

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
