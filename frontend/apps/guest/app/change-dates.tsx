import { Ionicons } from '@expo/vector-icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { nightsBetween, todayAddis, type DateChangeQuote } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { useAuth } from '../src/auth';
import { DateRangePicker } from '../src/DateRangePicker';
import { useDisplay } from '../src/display';
import { priceExact as price, shortDate } from '../src/format';
import { openGateway, savePendingPayment } from '../src/payment';

/**
 * Move a confirmed booking to other dates. The guest picks the dates, sees what they would cost (the same, less, more, or settled at the hotel), and confirms
 * the new total. When the new dates cost more, the booking moves after the difference is paid: the server holds the new nights and the booking screen
 * waits for the payment. The server decides everything; this screen only shows its answer.
 */
export default function ChangeDates() {
  useDisplay((st) => st.version); // redraw when the display currency changes
  const { t } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const token = useAuth((s) => s.token);
  const { id } = useLocalSearchParams<{ id: string }>();
  const booking = useQuery({ queryKey: ['booking', id], queryFn: () => api.getBooking(id), enabled: !!token && !!id });
  const [s, setS] = useState<string | null>(null);
  const [e, setE] = useState<string | null>(null);
  const [long, setLong] = useState(false);
  const [busy, setBusy] = useState(false);
  const [quote, setQuote] = useState<DateChangeQuote | null>(null);
  const [error, setError] = useState<string | null>(null);
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
  if (!token) return <Redirect href="/signin" />;

  const close = () => (router.canGoBack() ? router.back() : router.replace('/bookings'));
  const b = booking.data;
  const picked = s && e;

  function fail(err: unknown) {
    const code = err instanceof ApiError ? err.code : '';
    setError(
      code === 'PRICE_DIFFERS' ? t('change.price_changed')
        : code === 'SOLD_OUT' ? t('change.sold_out')
        : code === 'NEW_DATES_TOO_SOON' ? t('change.too_soon')
        : code === 'CHANGE_TOO_LATE' ? t('change.too_late')
        : code === 'CHANGE_LIMIT' ? t('change.limit')
        : code === 'CHANGE_PENDING' ? t('change.pending_exists')
        : code === 'NON_REFUNDABLE' || code === 'COUPON_NO_CHANGE' ? t('change.not_allowed')
        : code === 'NETWORK' ? t('results.offline_body')
        : t('change.failed'),
    );
    setBusy(false);
  }

  async function seePrice() {
    if (!s || !e || !b) return;
    setBusy(true);
    setError(null);
    try {
      setQuote(await api.quoteDateChange(b.id, { check_in: s, check_out: e }));
    } catch (err) {
      fail(err);
    } finally {
      setBusy(false);
    }
  }

  async function confirm() {
    if (!quote || !b) return;
    setBusy(true);
    setError(null);
    try {
      const updated = await api.changeBookingDates(b.id, {
        check_in: quote.check_in, check_out: quote.check_out, method: 'chapa',
        confirm_total_santim: quote.kind === 'same_price' ? undefined : quote.new_total_santim,
      });
      qc.setQueryData(['booking', b.id], updated);
      await qc.invalidateQueries({ queryKey: ['bookings'] });
      const pay = updated.pending_date_change?.payment;
      if (pay?.payment_url) {
        // The new nights are held while the difference is paid: open the payment page, then wait for it on the booking screen.
        await savePendingPayment(b.id);
        await openGateway({ id: pay.id, booking_id: b.id, method: 'chapa', payment_url: pay.payment_url }, router);
        router.replace({ pathname: '/booking/[id]', params: { id: b.id } });
        return;
      }
      close();
    } catch (err) {
      fail(err);
    }
  }

  const confirmTitle = !quote ? '' : quote.kind === 'pay' ? t('change.confirm_pay', { amount: price(quote.pay_now_santim, quote.currency) })
    : quote.kind === 'refund' ? t('change.confirm_refund', { amount: price(quote.refund_now_santim, quote.currency) })
    : t('change.confirm');

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }}>
      <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.lg }}>
        <T variant="h2">{t('change.title')}</T>
        <Pressable accessibilityLabel={t('common.cancel')} onPress={close} style={{ width: 44, height: 44, alignItems: 'center', justifyContent: 'center' }}>
          <Ionicons name="close" size={24} color={colors.ink} />
        </Pressable>
      </View>
      {!b ? (
        <ActivityIndicator style={{ marginTop: spacing.xxl }} color={colors.primary} />
      ) : quote ? (
        <View style={{ flex: 1, padding: spacing.lg }}>
          <T variant="small" color={colors.muted}>{b.property_name}</T>
          <T variant="h3" style={{ marginTop: spacing.xs }}>{shortDate(quote.check_in, months)} – {shortDate(quote.check_out, months)} · {t('common.night', { count: quote.nights })}</T>
          <View style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.lg, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface, gap: spacing.sm }}>
            <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
              <T color={colors.inkSoft}>{t('change.old_total')}</T>
              <T>{price(quote.old_total_santim, quote.currency)}</T>
            </View>
            <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
              <T variant="h3">{t('change.new_total')}</T>
              <T variant="h3">{price(quote.new_total_santim, quote.currency)}</T>
            </View>
          </View>
          <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.lg }}>
            <Ionicons name={quote.kind === 'pay' ? 'card-outline' : quote.kind === 'refund' ? 'return-down-back-outline' : 'checkmark-circle-outline'} size={22} color={quote.kind === 'pay' ? colors.warning : colors.success} />
            <T style={{ flex: 1 }}>
              {quote.kind === 'same_price' ? t('change.same_note')
                : quote.kind === 'pay' ? t('change.pay_note', { amount: price(quote.pay_now_santim, quote.currency) })
                : quote.kind === 'refund' ? t('change.refund_note', { amount: price(quote.refund_now_santim, quote.currency) })
                : quote.difference_santim > 0 ? t('change.hotel_more', { amount: price(quote.difference_santim, quote.currency) })
                : t('change.hotel_less', { amount: price(-quote.difference_santim, quote.currency) })}
            </T>
          </View>
          <View style={{ flex: 1 }} />
          {error ? <T color={colors.danger} style={{ marginBottom: spacing.sm }}>✕ {error}</T> : null}
          <Button title={confirmTitle} loading={busy} onPress={confirm} />
          <Button variant="ghost" title={t('change.pick_other')} onPress={() => { setQuote(null); setError(null); }} style={{ marginTop: spacing.sm }} />
        </View>
      ) : (
        <>
          <T variant="small" color={long ? colors.danger : colors.inkSoft} style={{ paddingHorizontal: spacing.lg }}>{long ? t('search.date_max') : t('change.hint')}</T>
          <T variant="small" color={colors.muted} style={{ paddingHorizontal: spacing.lg, marginTop: spacing.xs }}>
            {b.property_name} · {shortDate(b.check_in, months)} – {shortDate(b.check_out, months)}
          </T>
          <View style={{ flex: 1, marginTop: spacing.sm }}>
            <DateRangePicker start={s} end={e} today={todayAddis()} onChange={(a, z) => { setS(a); setE(z); setLong(false); setError(null); }} onTooLong={() => setLong(true)} />
          </View>
          <View style={{ padding: spacing.lg, borderTopWidth: 1, borderTopColor: colors.line }}>
            {error ? <T color={colors.danger} style={{ marginBottom: spacing.sm }}>✕ {error}</T> : null}
            <Button
              title={picked ? `${t('change.see_price')} · ${shortDate(s, months)} – ${shortDate(e, months)} · ${t('common.night', { count: nightsBetween(s, e) })}` : t('change.see_price')}
              disabled={!picked}
              loading={busy}
              onPress={seePrice}
            />
          </View>
        </>
      )}
    </SafeAreaView>
  );
}
