import { useEffect, useRef } from 'react';
import { api } from './api';
import { useAuth } from './auth';
import { usePrefs } from './store';

/**
 * Keeps the saved hotels (wishlist) on the server too, so they follow the guest to a new phone. The list on the phone stays the one the screens use (it
 * works offline); while the guest is signed in, what was saved or removed is sent a moment after the last change, and the server's list comes back and is
 * merged in. A failed attempt (no internet) changes nothing: the next change or the next launch sends it again.
 */
export function SavedSync() {
  const signedIn = useAuth((s) => !!s.token);
  const saved = usePrefs((s) => s.saved);
  const removed = usePrefs((s) => s.savedRemoved);
  const hydrated = usePrefs((s) => s.hydrated);
  const busy = useRef(false);

  useEffect(() => {
    if (!signedIn || !hydrated) return;
    const timer = setTimeout(async () => {
      if (busy.current) return;
      busy.current = true;
      const sent = { add: usePrefs.getState().saved, remove: usePrefs.getState().savedRemoved };
      try {
        const ids = await api.syncSaved(sent);
        usePrefs.getState().setSaved(ids, sent);
      } catch {
        // Offline or a server problem: it is sent again with the next change.
      } finally {
        busy.current = false;
      }
    }, 800);
    return () => clearTimeout(timer);
  }, [signedIn, hydrated, saved, removed]);

  return null;
}
