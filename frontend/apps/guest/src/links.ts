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

/** Opens the dialer. Returns false on devices with no phone app (tablets, emulators). */
export function callPhone(phone: string): Promise<boolean> {
  return tryOpen([`tel:${phone.replace(/[^\d+]/g, '')}`]);
}
