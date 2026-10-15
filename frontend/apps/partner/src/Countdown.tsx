import { useEffect, useState } from 'react';
import { holdRemainingMs } from '@bookdan/domain';

/**
 * Milliseconds left to answer, from the SERVER's clock: deadline minus the server time in the response,
 * minus the time since that response arrived. The phone's own clock is never compared with the deadline,
 * so a wrong phone date cannot hide or invent time. Null when the booking has no deadline.
 */
export function useRemaining(deadline: string | null, serverTime: string, receivedAt: number): number | null {
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);
  if (!deadline) return null;
  return holdRemainingMs(deadline, serverTime, Date.now() - receivedAt);
}

/** Below this the countdown turns red. */
export const URGENT_MS = 3 * 60_000;
