import Constants, { ExecutionEnvironment } from 'expo-constants';
import * as Device from 'expo-device';
import * as Notifications from 'expo-notifications';
import { useRootNavigationState, useRouter } from 'expo-router';
import { useEffect, useRef } from 'react';
import { Platform } from 'react-native';
import { parseBookingUpdate } from '@bookdan/domain';
import { api } from './api';
import { useAuth } from './auth';
import { getDeviceId } from './device';

/**
 * Booking updates ("the hotel confirmed your room") are push messages. They need a real Android phone running our own build;
 * a browser has none, and since SDK 53 Expo Go cannot receive them. iPhone is not built yet. Everywhere else the app still
 * works: the booking screen refreshes itself while it is open.
 */
export const updatesSupported =
  Platform.OS === 'android' && Device.isDevice && Constants.executionEnvironment !== ExecutionEnvironment.StoreClient;

// Chosen once at load, so the hook order never changes.
const useLastResponse: () => Notifications.NotificationResponse | null | undefined = updatesSupported
  ? Notifications.useLastNotificationResponse
  : () => null;

/** The Android channel booking updates use. The server names it in each message (data.channel). */
export const UPDATES_CHANNEL = 'booking_updates';

if (updatesSupported) {
  // While the app is open the message still shows.
  Notifications.setNotificationHandler({
    handleNotification: async () => ({ shouldShowBanner: true, shouldShowList: true, shouldPlaySound: true, shouldSetBadge: false }),
  });
}

async function ensureChannel(): Promise<void> {
  if (!updatesSupported) return;
  await Notifications.setNotificationChannelAsync(UPDATES_CHANNEL, {
    name: 'Booking updates',
    description: 'When a hotel confirms or declines your room',
    importance: Notifications.AndroidImportance.HIGH,
    lightColor: '#2563EB',
  });
}

/** Sends the phone's FCM token to the server. Safe to repeat: the server keeps one token per install. */
async function sendToken(token: string): Promise<void> {
  await api.registerPushToken({
    token,
    token_type: 'fcm',
    device_id: await getDeviceId(),
    platform: 'android',
    app_version: Constants.expoConfig?.version ?? '0',
  });
}

export type NotifyStatus = 'on' | 'off' | 'denied' | 'unsupported';

/** Whether this phone can and will receive booking updates right now. */
export async function readNotifyStatus(): Promise<NotifyStatus> {
  if (!updatesSupported) return 'unsupported';
  const { status } = await Notifications.getPermissionsAsync();
  return status === 'granted' ? 'on' : status === 'denied' ? 'denied' : 'off';
}

/**
 * Turns booking updates on: asks permission (the app asks only when the guest has something to wait for), gets the token
 * and registers it. Returns the resulting status, so the screen can say what happened.
 */
export async function enableBookingUpdates(): Promise<NotifyStatus> {
  if (!updatesSupported) return 'unsupported';
  await ensureChannel();
  let { status } = await Notifications.getPermissionsAsync();
  if (status !== 'granted') status = (await Notifications.requestPermissionsAsync()).status;
  if (status !== 'granted') return status === 'denied' ? 'denied' : 'off';
  try {
    await sendToken(String((await Notifications.getDevicePushTokenAsync()).data));
    return 'on';
  } catch {
    // No Firebase file in this build, no Google services on the phone, or no connection.
    return 'off';
  }
}

/**
 * Mounted once near the root. While the guest is signed in and has already allowed notifications it keeps the token
 * registered (FCM can rotate it) and opens the booking when a message is tapped, including when the tap started the app.
 */
export function usePushUpdates(): void {
  const router = useRouter();
  const navigation = useRootNavigationState();
  const signedIn = useAuth((s) => !!s.token);
  const lastResponse = useLastResponse();
  const opened = useRef<string | null>(null);

  useEffect(() => {
    if (!updatesSupported || !signedIn) return;
    // Only refresh the registration if the guest already said yes: the permission question belongs to the booking screen.
    readNotifyStatus().then((s) => (s === 'on' ? enableBookingUpdates() : undefined)).catch(() => undefined);
    const sub = Notifications.addPushTokenListener((t) => {
      sendToken(String(t.data)).catch(() => undefined);
    });
    return () => sub.remove();
  }, [signedIn]);

  useEffect(() => {
    if (!updatesSupported || !signedIn || !navigation?.key || !lastResponse) return;
    const id = lastResponse.notification.request.identifier;
    if (opened.current === id) return; // each tap opens its booking once
    const update = parseBookingUpdate(lastResponse.notification.request.content.data);
    if (!update) return;
    opened.current = id;
    router.push({ pathname: '/booking/[id]', params: { id: update.booking_id } });
  }, [lastResponse, signedIn, navigation?.key, router]);
}
