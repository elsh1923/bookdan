import { api } from './api';
import { useAuth } from './auth';

/**
 * The staff member tapped "Sign out": end the session on the server (so this phone stops getting booking alerts and the login
 * cannot be reused), then sign out on the phone. The server call waits at most 4 seconds and never blocks the sign-out.
 */
export async function endSession(): Promise<void> {
  const refreshToken = useAuth.getState().refreshToken;
  if (refreshToken) {
    try {
      await Promise.race([api.logout(refreshToken), new Promise((resolve) => setTimeout(resolve, 4000))]);
    } catch {
      // offline or the server is down: sign out locally regardless
    }
  }
  useAuth.getState().signOut();
}
