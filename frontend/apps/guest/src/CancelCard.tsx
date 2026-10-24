import { useState } from 'react';
import { View } from 'react-native';
import { ApiError } from '@bookdan/api-client';
import type { Booking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from './api';
import { price, shortDate } from './format';

/** "2026-11-18T09:00:00Z" as the Addis Ababa date and time the guest will understand. */
function addisMoment(iso: string, months: string[]): string {
  const local = new Date(Date.parse(iso) + 3 * 3_600_000).toISOString(); // Addis Ababa is UTC+3, no daylight saving
  return `${shortDate(local.slice(0, 10), months)}, ${local.slice(11, 16)}`;
}

/**
 * Cancel a booking. The server decides whether it is allowed and what comes back (the app only shows it), so the guest sees the
 * exact outcome before confirming: a full refund, a refund until a deadline, or no refund.
 */
export function CancelCard({ booking, months, onChanged }: { booking: Booking; months: string[]; onChanged: (b: Booking) => void }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refund = booking.refund_if_cancelled_santim;
  const message =
    refund > 0
      ? t('cancel.refund', { amount: price(refund, booking.currency) })
      : booking.payment_method === 'pay_at_hotel'
        ? t('cancel.nothing_paid')
        : t('cancel.no_refund');

  async function confirm() {
    setBusy(true);
    setError(null);
    try {
      onChanged(await api.cancelBooking(booking.id));
    } catch (e) {
      setError(
        e instanceof ApiError && e.code === 'NETWORK' ? t('results.offline_body')
          : e instanceof ApiError && e.code === 'CANCEL_TOO_LATE' ? t('cancel.too_late')
          : t('cancel.failed'),
      );
      setBusy(false);
    }
  }

  if (!open) {
    return <Button variant="ghost" icon="close-circle-outline" title={t('cancel.button')} onPress={() => setOpen(true)} style={{ marginTop: spacing.md }} />;
  }
  return (
    <View style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1.5, borderColor: colors.danger, backgroundColor: colors.dangerSoft }}>
      <T variant="h3" color={colors.danger}>{t('cancel.title')}</T>
      <T style={{ marginTop: spacing.xs }}>{message}</T>
      {!booking.refundable ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.xs }}>{t('cancel.nonrefundable_note')}</T> : null}
      {refund > 0 && booking.full_refund_until ? (
        <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>
          {t('cancel.free_until', { when: addisMoment(booking.full_refund_until, months) })}
        </T>
      ) : null}
      {refund === 0 && booking.payment_method !== 'pay_at_hotel' ? (
        <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('cancel.hotel_keeps')}</T>
      ) : null}
      {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {error}</T> : null}
      <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md }}>
        <Button variant="secondary" title={t('cancel.keep')} onPress={() => setOpen(false)} disabled={busy} style={{ flex: 1 }} />
        <Button title={t('cancel.confirm')} onPress={confirm} loading={busy} style={{ flex: 1 }} />
      </View>
    </View>
  );
}
