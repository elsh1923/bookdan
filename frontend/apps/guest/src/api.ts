import AsyncStorage from '@react-native-async-storage/async-storage';
import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister';
import { QueryClient } from '@tanstack/react-query';
import { getDeviceId } from './device';
import Constants from 'expo-constants';
import { Platform } from 'react-native';
import { createHttpApi, createMockApi, createSessionManager, type Api, type SessionHooks } from '@bookdan/api-client';

// Rule 1: the Go backend is the source of truth. With EXPO_PUBLIC_API_MODE=http (and EXPO_PUBLIC_API_URL) the app talks to it;
// otherwise a mock that honours the same contract stands in, so the app still runs with no server.
// EXPO_PUBLIC_* values are read when the app is bundled (set them in eas.json for builds and updates).
const extra = Constants.expoConfig?.extra as { apiMode?: string; apiUrl?: string } | undefined;
const mode = process.env.EXPO_PUBLIC_API_MODE ?? extra?.apiMode ?? 'mock';
const apiUrl = process.env.EXPO_PUBLIC_API_URL ?? extra?.apiUrl ?? (Platform.OS === 'web' ? 'http://localhost:8080' : '');
export const isMockApi = mode !== 'http';
if (!isMockApi && !apiUrl) console.warn('EXPO_PUBLIC_API_MODE=http but EXPO_PUBLIC_API_URL is not set: every request will fail.');

// The mock keeps its bookings on the phone so a demo survives the app being killed (acceptance test M3).
const raw: Api = isMockApi
  ? createMockApi({
      store: {
        load: () => AsyncStorage.getItem('mock-server-state'),
        save: (json) => AsyncStorage.setItem('mock-server-state', json),
      },
    })
  : createHttpApi({ baseUrl: apiUrl, app: 'guest', deviceId: getDeviceId, platform: Platform.OS === 'ios' ? 'ios' : Platform.OS === 'android' ? 'android' : 'web' });

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
  registerPushToken: withRefresh(raw.registerPushToken),
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
