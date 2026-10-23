import { useQuery } from '@tanstack/react-query';
import { ApiError } from '@bookdan/api-client';
import { api } from './api';

/**
 * The hotel's bookings. Polled every 15 seconds while the app is open; the real backend also sends a push
 * alert for each new booking, because a phone in a pocket does not poll.
 */
export function useInbox() {
  return useQuery({ queryKey: ['inbox'], queryFn: () => api.listBookings(), refetchInterval: 15_000 });
}

/** What the owner's hotels earned. A receptionist gets FORBIDDEN, which hides the tab; it is not retried. */
export function useEarnings() {
  return useQuery({
    queryKey: ['earnings'],
    queryFn: () => api.getEarnings(),
    retry: (count, e) => !(e instanceof ApiError && e.code === 'FORBIDDEN') && count < 1,
    refetchInterval: 60_000,
  });
}

/** Translation key for why an answer (accept or decline) failed. */
export function answerErrorKey(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.code === 'NETWORK') return 'partner.offline_answer'; // an answer cannot be queued: the deadline is short
    if (e.code === 'DEADLINE_PASSED') return 'partner.deadline_passed';
    if (e.code === 'INVALID_STATE') return 'partner.already_answered';
  }
  return 'partner.answer_failed';
}
