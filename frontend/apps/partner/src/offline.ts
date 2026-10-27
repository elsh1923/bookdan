import AsyncStorage from '@react-native-async-storage/async-storage';
import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import { enqueueCheckin, settleQueue, type QueuedCheckin, type SyncResult } from '@bookdan/domain';

/**
 * What the app keeps so a guest can still be checked in when the hotel has no internet: the server's voucher public key, the hotels this staff member works at,
 * the check-ins waiting to be sent, and the ones the server refused (shown until dismissed). It lives on the phone and survives a restart.
 */
export interface OfflineProblem {
  reference: string;
  result: SyncResult;
  at: string;
}

interface OfflineState {
  publicKey: string | null;
  hotelIds: string[];
  queue: QueuedCheckin[];
  problems: OfflineProblem[];
  setKey: (key: string) => void;
  setHotels: (ids: string[]) => void;
  enqueue: (item: QueuedCheckin) => void;
  settle: (sent: QueuedCheckin[], results: SyncResult[]) => void;
  dismiss: (reference: string) => void;
  wipe: () => void;
}

export const useOffline = create<OfflineState>()(
  persist(
    (set) => ({
      publicKey: null,
      hotelIds: [],
      queue: [],
      problems: [],
      setKey: (publicKey) => set({ publicKey }),
      setHotels: (hotelIds) => set({ hotelIds }),
      enqueue: (item) => set((s) => ({ queue: enqueueCheckin(s.queue, item) })),
      settle: (sent, results) =>
        set((s) => {
          const out = settleQueue(s.queue, sent, results);
          return { queue: out.queue, problems: [...s.problems, ...out.problems] };
        }),
      dismiss: (reference) => set((s) => ({ problems: s.problems.filter((p) => p.reference !== reference) })),
      wipe: () => set({ publicKey: null, hotelIds: [], queue: [], problems: [] }),
    }),
    { name: 'partner-offline', storage: createJSONStorage(() => AsyncStorage) },
  ),
);
