import type { SupportTicket } from '@bookdan/domain';

/** The badge colour of a request: an answer waiting for the guest stands out; waiting for support and closed are calm. */
export function statusTone(status: SupportTicket['status']): 'success' | 'warning' | 'neutral' {
  return status === 'answered' ? 'success' : status === 'open' ? 'warning' : 'neutral';
}
