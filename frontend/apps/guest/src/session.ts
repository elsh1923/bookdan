import { api, queryClient } from './api';
import { useAuth } from './auth';
import { clearAllVouchers } from './voucherStore';

/** Sign out and remove everything that belongs to the guest: token, cached data, saved vouchers. */
export async function signOutAndWipe(): Promise<void> {
  await endServerSession();
  useAuth.getState().signOut();
  queryClient.clear();
  await clearAllVouchers();
}

/** Tells the server this sign-in is over, waiting at most 4 seconds. With no connection the guest is still signed out on the phone. */
async function endServerSession(): Promise<void> {
  const refreshToken = useAuth.getState().refreshToken;
  if (!refreshToken) return;
  try {
    await Promise.race([api.logout(refreshToken), new Promise((resolve) => setTimeout(resolve, 4000))]);
  } catch {
    // offline or the server is down: sign out locally regardless
  }
}
