import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useState } from 'react';
import { ApiError } from '@bookdan/api-client';
import type { PartnerBooking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { answerErrorKey } from './queries';

/**
 * ok = the server took the answer. offline = no connection, the hotel can try again. moved_on = the booking
 * was already answered or its time ran out, so there is nothing left to retry.
 */
export type AnswerOutcome = 'ok' | 'offline' | 'moved_on';

/**
 * Accepting or declining a booking, with its busy and error state. Used by the inbox and by the booking
 * screen an alert opens, so both behave the same. There is no offline queue: the time to answer is short,
 * so a missing connection is reported clearly instead of promising something the hotel cannot keep.
 */
export function useAnswers() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [busyId, setBusyId] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const refresh = useCallback(() => qc.invalidateQueries({ queryKey: ['inbox'] }), [qc]);

  async function answer(b: PartnerBooking, action: () => Promise<unknown>): Promise<AnswerOutcome> {
    setBusyId(b.id);
    setErrors((e) => ({ ...e, [b.id]: '' }));
    try {
      await action();
      return 'ok';
    } catch (e) {
      setErrors((prev) => ({ ...prev, [b.id]: t(answerErrorKey(e)) }));
      return e instanceof ApiError && e.code === 'NETWORK' ? 'offline' : 'moved_on';
    } finally {
      setBusyId(null);
      refresh();
    }
  }

  return { busyId, errors, answer, refresh };
}
