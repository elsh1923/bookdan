import { Linking, Platform } from 'react-native';

async function tryOpen(urls: string[]): Promise<boolean> {
  for (const url of urls) {
    try {
      await Linking.openURL(url);
      return true;
    } catch {
      // try the next option
    }
  }
  return false;
}

/**
 * Opens the phone's own maps app at a point, falling back to a web map.
 * Returns false when nothing could open it, so the caller can show the address instead.
 */
export function openDirections(place: { lat: number; lng: number; label: string }): Promise<boolean> {
  const { lat, lng } = place;
  const label = encodeURIComponent(place.label);
  const web = `https://www.google.com/maps/search/?api=1&query=${lat},${lng}`;
  const urls = Platform.select<string[]>({
    ios: [`maps://?ll=${lat},${lng}&q=${label}`, `https://maps.apple.com/?ll=${lat},${lng}&q=${label}`, web],
    android: [`geo:${lat},${lng}?q=${lat},${lng}(${label})`, web],
    default: [web],
  })!;
  return tryOpen(urls);
}

/**
 * Opens a "new event" screen for a stay (Google Calendar's own link, which works with no calendar permission).
 * Times are the hotel's check-in and check-out in Addis Ababa. Returns false when nothing could open it.
 */
export function addStayToCalendar(stay: { title: string; details: string; location: string; checkIn: string; checkOut: string; inTime: string; outTime: string }): Promise<boolean> {
  const stamp = (date: string, time: string) => `${date.replace(/-/g, '')}T${time.replace(':', '')}00`;
  const q = new URLSearchParams({
    action: 'TEMPLATE',
    text: stay.title,
    details: stay.details,
    location: stay.location,
    dates: `${stamp(stay.checkIn, stay.inTime)}/${stamp(stay.checkOut, stay.outTime)}`,
    ctz: 'Africa/Addis_Ababa',
  });
  return tryOpen([`https://calendar.google.com/calendar/render?${q.toString()}`]);
}

/** Opens the dialer. Returns false on devices with no phone app (tablets, emulators). */
export function callPhone(phone: string): Promise<boolean> {
  return tryOpen([`tel:${phone.replace(/[^\d+]/g, '')}`]);
}
