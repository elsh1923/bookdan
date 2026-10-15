import AsyncStorage from '@react-native-async-storage/async-storage';
import { newIdempotencyKey } from '@bookdan/domain';

const KEY = 'partner-device-id';
let cached: string | null = null;

/** A stable id for this install of the app (sent to the server as X-Device-Id and with the push token). */
export async function getDeviceId(): Promise<string> {
  if (cached) return cached;
  let id = await AsyncStorage.getItem(KEY);
  if (!id) {
    id = newIdempotencyKey(); // a random UUID
    await AsyncStorage.setItem(KEY, id);
  }
  cached = id;
  return id;
}
