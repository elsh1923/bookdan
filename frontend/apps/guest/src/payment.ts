import AsyncStorage from '@react-native-async-storage/async-storage';
import * as Linking from 'expo-linking';
import { useRootNavigationState, useRouter } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { useEffect, useRef } from 'react';
import { ApiError } from '@bookdan/api-client';
import { shouldResumeOnLaunch, type PaymentSession } from '@bookdan/domain';
import { api } from './api';
import { useAuth } from './auth';

// Remembers which booking is mid-payment, so a killed app can take the guest straight back to it.
// It holds only a booking id, nothing sensitive.
const PENDING_KEY = 'pending-payment';

export const savePendingPayment = (bookingId: string) =>
  AsyncStorage.setItem(PENDING_KEY, JSON.stringify({ bookingId, at: Date.now() }));

export async function loadPendingPayment(): Promise<string | null> {
  try {
    const raw = await AsyncStorage.getItem(PENDING_KEY);
    const id = raw ? JSON.parse(raw)?.bookingId : null;
    return typeof id === 'string' ? id : null;
  } catch {
    return null;
  }
}

export const clearPendingPayment = () => AsyncStorage.removeItem(PENDING_KEY);

type AppRouter = ReturnType<typeof useRouter>;

/**
 * The one function that knows how a payment page is shown, so the screens never change when the real
 * Telebirr hand-off arrives. A real gateway opens in an in-app browser that returns by deep link
 * (hotelapp://payment-return). The result is never read from the browser: the booking screen polls.
 */
export async function openGateway(session: PaymentSession, router: AppRouter): Promise<void> {
  if (session.payment_url.startsWith('mock://')) {
    router.push({ pathname: '/mock-gateway/[session]', params: { session: session.id, booking: session.booking_id } });
    return;
  }
  await WebBrowser.openAuthSessionAsync(session.payment_url, Linking.createURL('payment-return'));
}

/** On launch, if a payment was left unfinished, take the guest back to that booking once. */
export function usePaymentRecovery() {
  const router = useRouter();
  const navigation = useRootNavigationState();
  const token = useAuth((s) => s.token);
  const done = useRef(false);

  useEffect(() => {
    if (done.current || !navigation?.key || !token) return;
    done.current = true;
    (async () => {
      const bookingId = await loadPendingPayment();
      if (!bookingId) return;
      try {
        const booking = await api.getBooking(bookingId);
        if (shouldResumeOnLaunch(booking)) router.push({ pathname: '/booking/[id]', params: { id: bookingId } });
        else await clearPendingPayment();
      } catch (e) {
        if (e instanceof ApiError && e.code === 'NOT_FOUND') await clearPendingPayment();
        // Offline or a server error: keep it and try on the next launch.
      }
    })();
  }, [navigation?.key, token, router]);
}
