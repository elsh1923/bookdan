import { useEffect, useState } from 'react';
import { View } from 'react-native';
import { ApiError } from '@bookdan/api-client';
import { formatCountdown, type Booking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { useRouter } from 'expo-router';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from './api';
import { useDisplay } from './display';
import { priceExact as price, shortDate } from './format';
import { openGateway, savePendingPayment } from './payment';

/**
 * Shown while a change of dates waits for the payment of the difference. The booking is still on its old dates; the new nights are held for a few minutes.
 * The guest can pay (the same page if it is still open, a new one after a failure) or keep the old dates. The booking screen polls, so it moves by itself
 * as soon as the payment arrives.
 */
export function PendingChangeCard({ booking, months, onChanged }: { booking: Booking; months: string[]; onChanged: (b: Booking) => void }) {
  useDisplay((st) => st.version); // redraw when the display currency changes
  const { t } = useTranslation();
  const router = useRouter();
  const pc = booking.pending_date_change;
  const [busy, setBusy] = useState<'pay' | 'drop' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  if (!pc) return null;
  const left = Math.max(0, Date.parse(pc.expires_at) - now);

  async function pay() {
    if (!pc) return;
    setBusy('pay');
    setError(null);
    try {
      let current = booking;
      if (!pc.payment || pc.payment.status !== 'pending') {
        current = await api.payDateChange(booking.id, 'chapa');
        onChanged(current);
      }
      const p = current.pending_date_change?.payment;
      if (p?.payment_url) {
        await savePendingPayment(booking.id);
        await openGateway({ id: p.id, booking_id: booking.id, method: 'chapa', payment_url: p.payment_url }, router);
      }
    } catch (e) {
      setError(e instanceof ApiError && e.code === 'NETWORK' ? t('results.offline_body') : t('change.pending_failed'));
    } finally {
      setBusy(null);
    }
  }

  async function drop() {
    setBusy('drop');
    setError(null);
    try {
      onChanged(await api.cancelDateChange(booking.id));
    } catch (e) {
      setError(e instanceof ApiError && e.code === 'NETWORK' ? t('results.offline_body') : t('change.pending_failed'));
    } finally {
      setBusy(null);
    }
  }

  return (
    <View style={{ marginTop: spacing.md, padding: spacing.lg, borderRadius: radius.lg, borderWidth: 1.5, borderColor: colors.warning, backgroundColor: colors.warningSoft }}>
      <T variant="h3">{t('change.pending_title')}</T>
      <T style={{ marginTop: spacing.xs }}>
        {t('change.pending_body', { amount: price(pc.pay_now_santim, booking.currency), from: shortDate(pc.check_in, months), to: shortDate(pc.check_out, months) })}
      </T>
      <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>
        {left > 0 ? t('change.pending_time', { time: formatCountdown(left) }) : t('change.pending_over')}
      </T>
      {error ? <T color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {error}</T> : null}
      <Button title={t('change.pending_pay', { amount: price(pc.pay_now_santim, booking.currency) })} loading={busy === 'pay'} disabled={left <= 0} onPress={pay} style={{ marginTop: spacing.md }} />
      <Button variant="ghost" title={t('change.pending_keep')} loading={busy === 'drop'} onPress={drop} style={{ marginTop: spacing.xs }} />
    </View>
  );
}
