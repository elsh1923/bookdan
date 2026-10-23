// Will this phone actually ring when a booking arrives? Budget Android phones silence background apps to
// save battery, so the partner app shows a checklist. This is the pure decision; the app gathers the facts.

/** Android notification importance: 5 is the highest ("urgent", makes a sound and pops up). */
export const ANDROID_IMPORTANCE_MAX = 5;

export interface AlertFacts {
  /** False in a browser or Expo Go: push alerts need a real phone running a development or store build. */
  supported: boolean;
  permission: 'granted' | 'denied' | 'undetermined';
  /** The Android "booking_alerts" channel, or null where channels do not exist (iPhone). */
  channel: { importance: number; hasSound: boolean } | null;
  /** The push token (FCM) registered with the server for this phone, or null. */
  pushToken: string | null;
}

export type AlertItemId = 'permission' | 'channel' | 'token' | 'battery';
/** ok = done, action = the staff member must do something, unknown = cannot be checked from inside the app. */
export type AlertItemStatus = 'ok' | 'action' | 'unknown';
export interface AlertItem {
  id: AlertItemId;
  status: AlertItemStatus;
}

export function alertReadiness(f: AlertFacts): { ready: boolean; items: AlertItem[] } {
  if (!f.supported) {
    return {
      ready: false,
      items: [
        { id: 'permission', status: 'action' },
        { id: 'channel', status: 'action' },
        { id: 'token', status: 'action' },
        { id: 'battery', status: 'unknown' },
      ],
    };
  }
  const permission: AlertItemStatus = f.permission === 'granted' ? 'ok' : 'action';
  const channel: AlertItemStatus =
    f.channel === null ? 'ok' : f.channel.importance >= ANDROID_IMPORTANCE_MAX && f.channel.hasSound ? 'ok' : 'action';
  const token: AlertItemStatus = f.pushToken ? 'ok' : 'action';
  return {
    // Battery optimisation cannot be read by an app, so it never counts as "done"; the checklist keeps asking.
    ready: permission === 'ok' && channel === 'ok' && token === 'ok',
    items: [
      { id: 'permission', status: permission },
      { id: 'channel', status: channel },
      { id: 'token', status: token },
      { id: 'battery', status: 'unknown' },
    ],
  };
}

/** What the server pushes when a booking needs an answer. The app opens that booking when it is tapped. */
export interface BookingAlertData {
  type: 'new_booking';
  booking_id: string;
}

/** A push the guest gets when the hotel answers (or does not). The server sends the booking and what happened to it. */
export interface BookingUpdateData {
  type: 'booking_update';
  booking_id: string;
  status: 'confirmed' | 'declined' | 'no_response';
}

export function parseBookingUpdate(data: unknown): BookingUpdateData | null {
  const d = data as { type?: unknown; booking_id?: unknown; status?: unknown } | null;
  if (
    d && d.type === 'booking_update' && typeof d.booking_id === 'string' && d.booking_id.length > 0 &&
    (d.status === 'confirmed' || d.status === 'declined' || d.status === 'no_response')
  ) {
    return { type: 'booking_update', booking_id: d.booking_id, status: d.status };
  }
  return null;
}

export function parseBookingAlert(data: unknown): BookingAlertData | null {
  const d = data as { type?: unknown; booking_id?: unknown } | null;
  if (d && d.type === 'new_booking' && typeof d.booking_id === 'string' && d.booking_id.length > 0) {
    return { type: 'new_booking', booking_id: d.booking_id };
  }
  return null;
}
