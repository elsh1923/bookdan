import AsyncStorage from '@react-native-async-storage/async-storage';
import { QueryClient } from '@tanstack/react-query';
import { getDeviceId } from './device';
import Constants from 'expo-constants';
import { Platform } from 'react-native';
import { createHttpPartnerApi, createMockPartnerApi, createSessionManager, type PartnerApi, type SessionHooks } from '@bookdan/api-client';

// With EXPO_PUBLIC_API_MODE=http (and EXPO_PUBLIC_API_URL) the app talks to the Go backend; otherwise a mock that honours
// the same contract stands in. EXPO_PUBLIC_* values are read when the app is bundled (set them in eas.json for builds and updates).
const extra = Constants.expoConfig?.extra as { apiMode?: string; apiUrl?: string } | undefined;
const mode = process.env.EXPO_PUBLIC_API_MODE ?? extra?.apiMode ?? 'mock';
export const apiUrl = process.env.EXPO_PUBLIC_API_URL ?? extra?.apiUrl ?? (Platform.OS === 'web' ? 'http://localhost:8080' : '');
export const isMockApi = mode !== 'http';
if (!isMockApi && !apiUrl) console.warn('EXPO_PUBLIC_API_MODE=http but EXPO_PUBLIC_API_URL is not set: every request will fail.');

// The mock keeps its demo bookings on the phone so a demo survives the app being killed.
const raw: PartnerApi = isMockApi
  ? createMockPartnerApi({
      store: {
        load: () => AsyncStorage.getItem('mock-partner-state'),
        save: (json) => AsyncStorage.setItem('mock-partner-state', json),
      },
    })
  : createHttpPartnerApi({ baseUrl: apiUrl, app: 'partner', deviceId: getDeviceId, platform: Platform.OS === 'ios' ? 'ios' : Platform.OS === 'android' ? 'android' : 'web' });

// Silent session refresh is shared with the guest app.
const session = createSessionManager(raw);
export const bindSession = session.bindSession;
const withRefresh = session.withRefresh;
export type { SessionHooks };

export const api: PartnerApi = {
  ...raw,
  registerPushToken: withRefresh(raw.registerPushToken),
  revealGuestId: withRefresh(raw.revealGuestId),
  listBookings: withRefresh(raw.listBookings),
  acceptBooking: withRefresh(raw.acceptBooking),
  declineBooking: withRefresh(raw.declineBooking),
  cancelBooking: withRefresh(raw.cancelBooking),
  relocateBooking: withRefresh(raw.relocateBooking),
  getDispute: withRefresh(raw.getDispute),
  sendDisputeStatement: withRefresh(raw.sendDisputeStatement),
  getStatements: withRefresh(raw.getStatements),
  syncCheckins: withRefresh(raw.syncCheckins),
  listReviews: withRefresh(raw.listReviews),
  replyToReview: withRefresh(raw.replyToReview),
  removeReviewReply: withRefresh(raw.removeReviewReply),
  listSupportTickets: withRefresh(raw.listSupportTickets),
  getSupportTicket: withRefresh(raw.getSupportTicket),
  createSupportTicket: withRefresh(raw.createSupportTicket),
  replySupportTicket: withRefresh(raw.replySupportTicket),
  closeSupportTicket: withRefresh(raw.closeSupportTicket),
  lookupCheckin: withRefresh(raw.lookupCheckin),
  checkInGuest: withRefresh(raw.checkInGuest),
  getEarnings: withRefresh(raw.getEarnings),
  getRoomsChart: withRefresh(raw.getRoomsChart),
  listRoomBlocks: withRefresh(raw.listRoomBlocks),
  createRoomBlock: withRefresh(raw.createRoomBlock),
  releaseRoomBlock: withRefresh(raw.releaseRoomBlock),
  markNoShow: withRefresh(raw.markNoShow),
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
