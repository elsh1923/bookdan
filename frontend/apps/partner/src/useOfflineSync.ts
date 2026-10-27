import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';
import type { SyncResult } from '@bookdan/domain';
import { api } from './api';
import { useAuth } from './auth';
import { useOffline } from './offline';

const EVERY_MS = 60_000;

/**
 * Keeps check-in working offline: while online it refreshes the voucher key and the hotel list, and it sends the check-ins made offline (now, when the app comes
 * back to the front, and every minute). A failed attempt changes nothing: the queue stays and is tried again.
 */
export function useOfflineSync() {
  const token = useAuth((s) => s.token);
  const busy = useRef(false);

  useEffect(() => {
    if (!token) return;
    const run = async () => {
      if (busy.current) return;
      busy.current = true;
      try {
        const st = useOffline.getState();
        try {
          const key = await api.getVoucherKey();
          if (key.public_key) st.setKey(key.public_key);
          const chart = await api.getRoomsChart();
          if (chart.hotels.length) st.setHotels(chart.hotels.map((h) => h.id));
        } catch {
          // offline: what is cached is used
        }
        const queued = useOffline.getState().queue;
        if (queued.length > 0) {
          const sent = queued.slice(0, 100);
          try {
            const results = await api.syncCheckins(sent.map((q) => ({ token: q.token, at: q.at })));
            useOffline.getState().settle(sent, results.map((r) => r.result as SyncResult));
          } catch {
            // still offline: try again later
          }
        }
      } finally {
        busy.current = false;
      }
    };
    run();
    const timer = setInterval(run, EVERY_MS);
    const sub = AppState.addEventListener('change', (s) => {
      if (s === 'active') run();
    });
    return () => {
      clearInterval(timer);
      sub.remove();
    };
  }, [token]);
}
