import AsyncStorage from '@react-native-async-storage/async-storage';
import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister';
import { QueryClient } from '@tanstack/react-query';
import Constants from 'expo-constants';
import { createMockApi, createSessionManager, type Api, type SessionHooks } from '@bookdan/api-client';

// Rule 1: the Go backend is the source of truth. Until it exists, a mock that
// honours the same contract stands in. Flip extra.apiMode to "http" later.
const mode = (Constants.expoConfig?.extra?.apiMode as string | undefined) ?? 'mock';
export const isMockApi = mode === 'mock';

// The mock keeps its bookings on the phone so a demo survives the app being killed (acceptance test M3).
const raw: Api = createMockApi({
  store: {
    load: () => AsyncStorage.getItem('mock-server-state'),
    save: (json) => AsyncStorage.setItem('mock-server-state', json),
  },
});

// Silent session refresh is shared with the partner app.
const session = createSessionManager(raw);
export const bindSession = session.bindSession;
const withRefresh = session.withRefresh;
export type { SessionHooks };

export const api: Api = {
  ...raw,
  getProfile: withRefresh(raw.getProfile),
  saveProfile: withRefresh(raw.saveProfile),
  deleteProfile: withRefresh(raw.deleteProfile),
  createQuote: withRefresh(raw.createQuote),
  createHold: withRefresh(raw.createHold),
  startPayment: withRefresh(raw.startPayment),
  getBooking: withRefresh(raw.getBooking),
  listBookings: withRefresh(raw.listBookings),
  getVoucher: withRefresh(raw.getVoucher),
  // Demo controls exist only on the mock and must never ship in a production build.
  demo: isMockApi ? raw.demo : undefined,
};

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 2,
      retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 8000), // growing delay, capped
      staleTime: 5 * 60_000, // search results: 5 minutes
      gcTime: 24 * 3_600_000,
    },
  },
});

// Only public catalogue data is cached to disk, so search and property pages open offline.
// Nothing about the guest (bookings, tokens, vouchers) goes here.
export const queryPersister = createAsyncStoragePersister({ storage: AsyncStorage, key: 'bookdan-query-cache', throttleTime: 1000 });
export const PERSISTED_QUERY_KEYS = new Set(['search', 'property']);
