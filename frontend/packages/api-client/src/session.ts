import { ApiError, type AuthApi, type Tokens } from './types';

/** What the session needs from an app's account store, registered by the app (avoids an import cycle). */
export interface SessionHooks {
  getRefreshToken(): string | null;
  setTokens(tokens: Tokens): void;
  /** Refresh failed for good: clear the session so screens send the user back to sign-in. */
  expire(): void;
}

/**
 * Silent session refresh shared by the guest and partner apps. Wrap every call that needs a signed-in
 * user with withRefresh: on UNAUTHENTICATED it refreshes once (however many calls fail together)
 * and retries.
 */
export function createSessionManager(auth: Pick<AuthApi, 'refresh'>) {
  let hooks: SessionHooks | null = null;
  let refreshing: Promise<boolean> | null = null;

  function refreshSession(): Promise<boolean> {
    refreshing ??= (async () => {
      const refreshToken = hooks?.getRefreshToken();
      if (!hooks || !refreshToken) return false;
      try {
        hooks.setTokens(await auth.refresh(refreshToken));
        return true;
      } catch (e) {
        // No signal is not the same as a bad session: keep the user signed in and surface the network error.
        if (e instanceof ApiError && e.code === 'UNAUTHENTICATED') {
          hooks.expire();
          return false;
        }
        throw e;
      }
    })().finally(() => {
      refreshing = null;
    });
    return refreshing;
  }

  return {
    bindSession(h: SessionHooks) {
      hooks = h;
    },
    withRefresh<A extends unknown[], R>(fn: (...args: A) => Promise<R>) {
      return async (...args: A): Promise<R> => {
        try {
          return await fn(...args);
        } catch (e) {
          if (e instanceof ApiError && e.code === 'UNAUTHENTICATED' && (await refreshSession())) return fn(...args);
          throw e;
        }
      };
    },
  };
}
