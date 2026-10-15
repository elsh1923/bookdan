import AsyncStorage from '@react-native-async-storage/async-storage';
import { QueryClient } from '@tanstack/react-query';
import Constants from 'expo-constants';
import { createMockPartnerApi, createSessionManager, type PartnerApi, type SessionHooks } from '@bookdan/api-client';

// Until the Go backend exists, a mock that honours the same contract stands in.
// Flip extra.apiMode to "http" later.
const mode = (Constants.expoConfig?.extra?.apiMode as string | undefined) ?? 'mock';
export const isMockApi = mode === 'mock';

// The mock keeps its demo bookings on the phone so a demo survives the app being killed.
const raw: PartnerApi = createMockPartnerApi({
  store: {
    load: () => AsyncStorage.getItem('mock-partner-state'),
    save: (json) => AsyncStorage.setItem('mock-partner-state', json),
  },
});

// Silent session refresh is shared with the guest app.
const session = createSessionManager(raw);
export const bindSession = session.bindSession;
const withRefresh = session.withRefresh;
export type { SessionHooks };

export const api: PartnerApi = {
  ...raw,
  registerPushToken: withRefresh(raw.registerPushToken),
  listBookings: withRefresh(raw.listBookings),
  acceptBooking: withRefresh(raw.acceptBooking),
  declineBooking: withRefresh(raw.declineBooking),
  // Demo controls exist only on the mock and must never ship in a production build.
  demo: isMockApi ? raw.demo : undefined,
};

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 8000), // growing delay, capped
      staleTime: 0, // the inbox is time-critical: always ask the server
    },
  },
});
