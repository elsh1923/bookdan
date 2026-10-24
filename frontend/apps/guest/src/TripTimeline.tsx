import { Ionicons } from '@expo/vector-icons';
import { View } from 'react-native';
import type { Booking, BookingStatus } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, spacing } from '@bookdan/ui';

type Step = 'requested' | 'confirmed' | 'checked_in' | 'completed' | 'declined' | 'cancelled' | 'expired';

const STEP_OF: Partial<Record<BookingStatus, Step>> = {
  PAID_AWAITING_HOTEL: 'requested',
  CONFIRMED: 'confirmed',
  CHECKED_IN: 'checked_in',
  COMPLETED: 'completed',
  DECLINED_REFUNDED: 'declined',
  CANCELLED_REFUNDED: 'cancelled',
  EXPIRED: 'expired',
};
const PATH: Step[] = ['requested', 'confirmed', 'checked_in', 'completed'];
const LAST_ON_PATH: Partial<Record<BookingStatus, number>> = { PAID_AWAITING_HOTEL: 0, CONFIRMED: 1, CHECKED_IN: 2, COMPLETED: 3 };

function when(iso: string): string {
  const d = new Date(iso);
  return `${d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })}, ${d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}`;
}

/**
 * What has happened to the booking and what comes next: requested, confirmed, checked in, completed (or why it ended).
 * Times come from the server's history; without one (the offline demo) the steps are drawn from the status alone.
 */
export function TripTimeline({ booking }: { booking: Booking }) {
  const { t } = useTranslation();
  if (booking.status === 'HELD') return null;

  const reached = new Map<Step, string | null>();
  for (const e of booking.timeline) {
    const step = STEP_OF[e.status];
    if (step && !reached.has(step)) reached.set(step, e.at);
  }
  // No history (demo mode, or an older server): show the steps up to the current status without times.
  const upTo = LAST_ON_PATH[booking.status];
  if (reached.size === 0) {
    if (upTo !== undefined) PATH.slice(0, upTo + 1).forEach((s) => reached.set(s, null));
    else { const s = STEP_OF[booking.status]; if (s) reached.set(s, null); }
  }
  // A booking that ended early shows the steps it did reach, then how it ended; an active one also shows what is still to come.
  const steps: { step: Step; at: string | null; done: boolean }[] = [];
  for (const s of PATH) if (reached.has(s)) steps.push({ step: s, at: reached.get(s) ?? null, done: true });
  for (const s of ['declined', 'cancelled', 'expired'] as Step[]) if (reached.has(s)) steps.push({ step: s, at: reached.get(s) ?? null, done: true });
  if (upTo !== undefined) for (const s of PATH.slice(upTo + 1)) steps.push({ step: s, at: null, done: false });

  return (
    <View style={{ marginTop: spacing.xl }}>
      <T variant="displaySm">{t('booking.timeline_title')}</T>
      <View style={{ marginTop: spacing.md }}>
        {steps.map((s, i) => {
          const bad = s.step === 'declined' || s.step === 'cancelled' || s.step === 'expired';
          return (
            <View key={s.step} style={{ flexDirection: 'row', minHeight: 48 }}>
              <View style={{ alignItems: 'center', width: 28 }}>
                <Ionicons name={s.done ? (bad ? 'close-circle' : 'checkmark-circle') : 'ellipse-outline'} size={22} color={s.done ? (bad ? colors.danger : colors.success) : colors.muted} />
                {i < steps.length - 1 ? <View style={{ flex: 1, width: 2, backgroundColor: s.done ? colors.success : colors.line, marginVertical: 2 }} /> : null}
              </View>
              <View style={{ flex: 1, paddingLeft: spacing.sm, paddingBottom: spacing.md }}>
                <T color={s.done ? colors.ink : colors.muted}>{t(`booking.step_${s.step}`)}</T>
                {s.at ? <T variant="label" color={colors.muted}>{when(s.at)}</T> : null}
              </View>
            </View>
          );
        })}
      </View>
    </View>
  );
}
