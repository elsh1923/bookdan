import Constants, { ExecutionEnvironment } from 'expo-constants';
import * as Device from 'expo-device';
import * as Notifications from 'expo-notifications';
import { useRootNavigationState, useRouter } from 'expo-router';
import { useEffect, useRef } from 'react';
import { Platform } from 'react-native';
import { parseBookingAlert, type AlertFacts } from '@bookdan/domain';
import { api } from './api';
import { useAuth } from './auth';
import { getDeviceId } from './device';

/**
 * Push alerts need a real Android phone running our own build. They do not exist in a browser, and since
 * SDK 53 Expo Go cannot receive them either. iPhone is not planned for the partner app.
 */
export const alertsSupported =
  Platform.OS === 'android' && Device.isDevice && Constants.executionEnvironment !== ExecutionEnvironment.StoreClient;

// Chosen once at load, so the hook order never changes. In a browser or Expo Go there is nothing to read.
const useLastResponse: () => Notifications.NotificationResponse | null | undefined = alertsSupported
  ? Notifications.useLastNotificationResponse
  : () => null;

/** The Android channel new-booking alerts use: highest importance, its own sound (spec section 13). */
export const ALERT_CHANNEL = 'booking_alerts';
export const ALERT_SOUND = 'booking_alert.wav'; // bundled by the expo-notifications plugin in app.json

if (alertsSupported) {
  // While the app is open the alert still shows and still plays its sound.
  Notifications.setNotificationHandler({
    handleNotification: async () => ({ shouldShowBanner: true, shouldShowList: true, shouldPlaySound: true, shouldSetBadge: false }),
  });
}

export async function ensureAlertChannel(): Promise<void> {
  if (!alertsSupported) return;
  await Notifications.setNotificationChannelAsync(ALERT_CHANNEL, {
    name: 'New bookings',
    description: 'A guest has paid and the hotel must answer within 15 minutes',
    importance: Notifications.AndroidImportance.MAX,
    sound: ALERT_SOUND,
    vibrationPattern: [0, 500, 250, 500, 250, 500],
    lightColor: '#2563EB',
    lockscreenVisibility: Notifications.AndroidNotificationVisibility.PUBLIC,
    enableVibrate: true,
  });
}

/** Reads the FCM token and sends it to the server. Safe to call again: the server replaces the old one. */
async function sendToken(token: string): Promise<void> {
  await api.registerPushToken({
    token,
    token_type: 'fcm',
    device_id: await getDeviceId(),
    platform: 'android',
    app_version: Constants.expoConfig?.version ?? '0',
  });
}

export type RegisterResult = 'ok' | 'unsupported' | 'denied' | 'no_token';

/**
 * Set this phone up for alerts: create the channel, ask for permission (the one moment the spec allows
 * it, at partner setup), get the FCM token and register it. Returns why it could not finish, so the
 * settings checklist can say so.
 */
export async function registerForAlerts(): Promise<RegisterResult> {
  if (!alertsSupported) return 'unsupported';
  await ensureAlertChannel();
  let { status } = await Notifications.getPermissionsAsync();
  if (status !== 'granted') status = (await Notifications.requestPermissionsAsync()).status;
  if (status !== 'granted') return 'denied';
  try {
    const t = await Notifications.getDevicePushTokenAsync();
    await sendToken(String(t.data));
    return 'ok';
  } catch {
    // No Firebase config in this build, no Google services on the phone, or no connection.
    return 'no_token';
  }
}

/** What the readiness checklist needs to know right now. */
export async function readAlertFacts(): Promise<AlertFacts> {
  if (!alertsSupported) return { supported: false, permission: 'undetermined', channel: null, pushToken: null };
  const perm = await Notifications.getPermissionsAsync();
  const channel = await Notifications.getNotificationChannelAsync(ALERT_CHANNEL);
  let pushToken: string | null = null;
  try {
    pushToken = String((await Notifications.getDevicePushTokenAsync()).data) || null;
  } catch {
    pushToken = null;
  }
  return {
    supported: true,
    permission: perm.status === 'granted' ? 'granted' : perm.status === 'denied' ? 'denied' : 'undetermined',
    channel: channel ? { importance: channel.importance, hasSound: !!channel.sound } : { importance: 0, hasSound: false },
    pushToken,
  };
}

/**
 * A local alert in 5 seconds, on the same channel and with the same sound as a real one, so staff can lock
 * the phone and hear what a new booking sounds like. It needs no server or Firebase.
 */
export async function sendTestAlert(bookingId: string): Promise<void> {
  await ensureAlertChannel();
  await Notifications.scheduleNotificationAsync({
    content: {
      title: 'New booking',
      body: 'A guest has paid. Answer within 15 minutes.',
      sound: ALERT_SOUND,
      data: { type: 'new_booking', booking_id: bookingId },
    },
    trigger: { type: Notifications.SchedulableTriggerInputTypes.TIME_INTERVAL, seconds: 5, channelId: ALERT_CHANNEL },
  });
}

/**
 * Mounted once, near the root. While signed in it registers the phone for alerts, keeps the token fresh,
 * and opens the right booking when an alert is tapped (including when the tap started the app).
 */
export function usePushAlerts(): void {
  const router = useRouter();
  const navigation = useRootNavigationState();
  const signedIn = useAuth((s) => !!s.token);
  const lastResponse = useLastResponse();
  const opened = useRef<string | null>(null);

  useEffect(() => {
    if (!alertsSupported || !signedIn) return;
    registerForAlerts();
    // FCM can rotate the token at any time: tell the server the new one.
    const sub = Notifications.addPushTokenListener((t) => {
      sendToken(String(t.data)).catch(() => undefined);
    });
    return () => sub.remove();
  }, [signedIn]);

  useEffect(() => {
    if (!alertsSupported || !signedIn || !navigation?.key || !lastResponse) return;
    const id = lastResponse.notification.request.identifier;
    if (opened.current === id) return; // each tap opens its booking once
    const alert = parseBookingAlert(lastResponse.notification.request.content.data);
    if (!alert) return;
    opened.current = id;
    router.push({ pathname: '/booking/[id]', params: { id: alert.booking_id } });
  }, [lastResponse, signedIn, navigation?.key, router]);
}
