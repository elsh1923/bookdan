import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { Tokens } from '@bookdan/api-client';
import type { Identity } from '@bookdan/domain';
import { api, bindSession } from './api';
import { secureStorage } from './secureStorage';

interface AuthState {
  token: string | null;
  refreshToken: string | null;
  identity: Identity | null; // how the guest signed in: phone or email
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
      },
    }),
    {
      name: 'guest-auth',
      version: 2,
      storage: createJSONStorage(() => secureStorage),
      // v0 stored only a phone number, v1 an identity without a refresh token.
      migrate: (persisted: unknown) => {
        const old = (persisted ?? {}) as { token?: string | null; phone?: string | null; identity?: Identity | null };
        return {
          token: old.token ?? null,
          refreshToken: null,
          identity: old.identity ?? (old.phone ? { type: 'phone', value: old.phone } : null),
        } as AuthState;
      },
      onRehydrateStorage: () => (state) => api.setToken(state?.token ?? null),
    },
  ),
);

// Let the API client refresh an expired session without importing this store (which imports the API).
bindSession({
  getRefreshToken: () => useAuth.getState().refreshToken,
  setTokens: (tokens) => useAuth.getState().setTokens(tokens),
  // The guest is signed out for good, but nothing they typed or saved on the phone is touched here:
  // the checkout draft stays, and sign-in sends them back to where they were.
  expire: () => useAuth.getState().signOut(),
});
