import { queryClient } from './api';
import { useAuth } from './auth';
import { clearAllVouchers } from './voucherStore';

/** Sign out and remove everything that belongs to the guest: token, cached data, saved vouchers. */
export async function signOutAndWipe(): Promise<void> {
  useAuth.getState().signOut();
  queryClient.clear();
  await clearAllVouchers();
}
