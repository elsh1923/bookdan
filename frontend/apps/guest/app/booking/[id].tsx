import { Ionicons } from '@expo/vector-icons';
import { useQueryClient } from '@tanstack/react-query';
import { useFocusEffect, useLocalSearchParams, useRouter } from 'expo-router';
import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { formatCountdown, holdRemainingMs, shouldResumeOnLaunch, type Booking, type PaymentMethod } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Badge, Button, EmptyState, Pop, Reveal, T, colors, fonts, radius, shadow, spacing } from '@bookdan/ui';
import { api, isMockApi } from '../../src/api';
import { CancelCard } from '../../src/CancelCard';
import { NotifyMeCard } from '../../src/NotifyMeCard';
import { ReviewCard } from '../../src/ReviewCard';
import { addStayToCalendar, callPhone } from '../../src/links';
import { clearPendingPayment, openGateway, savePendingPayment } from '../../src/payment';
import { price, shortDate } from '../../src/format';
import { useBooking } from '../../src/queries';
import { StatusBadge } from '../../src/StatusBadge';
import { TripTimeline } from '../../src/TripTimeline';
import { ensureVoucher } from '../../src/voucherStore';

/** Countdown from the server clock: remaining = expiry - server_time - time since that response. */
function useHoldRemaining(b: Booking | undefined, receivedAt: number) {
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);
  if (!b?.hold_expires_at) return null;
  return holdRemainingMs(b.hold_expires_at, b.server_time, Date.now() - receivedAt);
}

function Line({ label, value }: { label: string; value: string }) {
  return (
    <View style={{ flexDirection: 'row', justifyContent: 'space-between', paddingVertical: spacing.sm }}>
      <T color={colors.inkSoft} style={{ marginRight: spacing.md }}>{label}</T>
      <T style={{ flexShrink: 1, flex: 1, textAlign: 'right' }}>{value}</T>
    </View>
  );
}

function PaymentOption({ icon, title, note, selected, onPress }: { icon: keyof typeof Ionicons.glyphMap; title: string; note?: string; selected: boolean; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="radio"
      accessibilityState={{ selected }}
      onPress={onPress}
      style={{
        minHeight: 64, paddingVertical: spacing.md, paddingHorizontal: spacing.md, marginTop: spacing.sm, borderRadius: radius.md, borderWidth: 1.5,
        flexDirection: 'row', alignItems: 'center', gap: spacing.md,
        borderColor: selected ? colors.primary : colors.line,
        backgroundColor: selected ? colors.primarySoft : colors.surface,
      }}
    >
      <Ionicons name={icon} size={22} color={selected ? colors.primaryInk : colors.inkSoft} />
      <View style={{ flex: 1 }}>
        <T variant="h3" color={selected ? colors.primaryInk : colors.ink}>{title}</T>
        {note ? <T variant="small" color={colors.inkSoft}>{note}</T> : null}
      </View>
      <Ionicons name={selected ? 'radio-button-on' : 'radio-button-off'} size={22} color={selected ? colors.primary : colors.muted} />
    </Pressable>
  );
}

export default function BookingScreen() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const { id } = useLocalSearchParams<{ id: string }>();
  const query = useBooking(id);
  const remaining = useHoldRemaining(query.data, query.dataUpdatedAt);
  // Coming back from the payment page: ask the server right away instead of waiting for the next poll.
  const { refetch } = query;
  useFocusEffect(useCallback(() => { refetch(); }, [refetch]));
  const confirmed = query.data?.status === 'CONFIRMED' || query.data?.status === 'CHECKED_IN';
  const [method, setMethod] = useState<PaymentMethod>('chapa');
  const [paying, setPaying] = useState(false);
  const [payError, setPayError] = useState<string | null>(null);

  // Once confirmed, save the voucher on this phone so it opens without internet.
  useEffect(() => {
    if (!confirmed || !id) return;
    ensureVoucher(id).then(() => qc.invalidateQueries({ queryKey: ['voucher', id] })).catch(() => undefined);
  }, [confirmed, id, qc]);

  // Once the payment is settled one way or the other, stop reminding the guest on the next launch.
  useEffect(() => {
    if (query.data && !shouldResumeOnLaunch(query.data)) clearPendingPayment().catch(() => undefined);
  }, [query.data]);

  // After cancelling or reviewing: show the server's answer at once and refresh the list of bookings.
  const onBookingChanged = (updated: Booking) => {
    qc.setQueryData(['booking', id], updated);
    qc.invalidateQueries({ queryKey: ['bookings'] });
  };
  const goBookings = () => router.replace('/bookings');
  const header = (
    <View style={{ paddingHorizontal: spacing.lg }}>
      <Pressable accessibilityLabel={t('common.back')} onPress={goBookings} style={{ width: 44, height: 44, justifyContent: 'center' }}>
        <Ionicons name="arrow-back" size={24} color={colors.ink} />
      </Pressable>
    </View>
  );

  if (query.isPending) {
    return <SafeAreaView style={{ flex: 1 }}>{header}<View style={{ flex: 1, justifyContent: 'center' }}><ActivityIndicator size="large" color={colors.primary} /></View></SafeAreaView>;
  }
  if (query.isError) {
    return (
      <SafeAreaView style={{ flex: 1 }}>
        {header}
        <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => query.refetch()} />
      </SafeAreaView>
    );
  }

  const b = query.data;
  const months = i18n.t('months.short', { returnObjects: true }) as unknown as string[];
  // The server decides expiry; the countdown only tells the guest how long is left.
  const held = b.status === 'HELD' && (remaining === null || remaining > 0);
  const paymentPending = held && b.payment === 'pending';

  async function pay() {
    setPaying(true);
    setPayError(null);
    try {
      const { booking, session } = await api.startPayment({ booking_id: b.id, method });
      qc.setQueryData(['booking', b.id], booking);
      qc.invalidateQueries({ queryKey: ['bookings'] });
      // Pay-at-hotel is done at once. Otherwise open the payment page; the result comes back by polling.
      if (session) {
        await savePendingPayment(b.id);
        await openGateway(session, router);
      }
    } catch (e) {
      setPayError(e instanceof ApiError && e.code === 'HOLD_EXPIRED' ? t('booking.hold_expired_pay') : t('booking.pay_failed'));
      query.refetch();
    } finally {
      setPaying(false);
    }
  }

  const hero = ((): { icon: keyof typeof Ionicons.glyphMap; color: string; title: string; body: string } => {
    switch (b.status) {
      case 'HELD':
        return held
          ? { icon: 'timer-outline', color: colors.primary, title: t('booking.held_title'), body: t('booking.held_body') }
          : { icon: 'time-outline', color: colors.muted, title: t('booking.expired_title'), body: t('booking.expired_body') };
      case 'PAID_AWAITING_HOTEL': return { icon: 'hourglass-outline', color: colors.warning, title: t('booking.awaiting_title'), body: t('booking.awaiting_body') };
      case 'CONFIRMED':
      case 'CHECKED_IN':
      case 'COMPLETED': return { icon: 'checkmark-circle', color: colors.success, title: t('booking.confirmed_title'), body: t('booking.confirmed_body') };
      case 'EXPIRED': return { icon: 'time-outline', color: colors.muted, title: t('booking.expired_title'), body: t('booking.expired_body') };
      case 'DECLINED_REFUNDED': return { icon: 'arrow-undo-outline', color: colors.danger, title: t('booking.declined_title'), body: t('booking.declined_body') };
      case 'CANCELLED_REFUNDED': return { icon: 'arrow-undo-outline', color: colors.muted, title: t('booking.cancelled_title'), body: b.refund_santim ? t('booking.cancelled_refunded', { amount: price(b.refund_santim, b.currency) }) : b.payment_method === 'pay_at_hotel' ? t('booking.cancelled_nothing_paid') : t('booking.cancelled_no_refund') };
      default: return { icon: 'ellipse-outline', color: colors.muted, title: t(`status.${b.status}`), body: '' };
    }
  })();

  const expired = b.status === 'EXPIRED' || (b.status === 'HELD' && !held);
  const total = price(b.total_santim, b.currency);

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }}>
      {header}
      <ScrollView contentContainerStyle={{ padding: spacing.lg, paddingTop: 0, paddingBottom: spacing.xxl * 2 }}>
        <View style={{ alignItems: 'center', paddingVertical: spacing.lg }}>
          <Pop key={hero.icon + hero.title} style={{ width: 72, height: 72, borderRadius: 36, backgroundColor: confirmed ? colors.successSoft : colors.fill, alignItems: 'center', justifyContent: 'center' }}>
            <Ionicons name={hero.icon} size={38} color={hero.color} />
          </Pop>
          <T variant="displaySm" style={{ textAlign: 'center', marginTop: spacing.md }}>{hero.title}</T>
          {hero.body ? <T color={colors.inkSoft} style={{ textAlign: 'center', marginTop: spacing.xs }}>{hero.body}</T> : null}
          <View style={{ marginTop: spacing.md }}><StatusBadge status={expired ? 'EXPIRED' : b.status} /></View>
        </View>

        {held && remaining !== null ? (
          <View accessibilityLabel={t('booking.time_left')} style={{ alignSelf: 'center', flexDirection: 'row', alignItems: 'center', gap: spacing.xs, marginTop: spacing.sm, paddingHorizontal: spacing.md, paddingVertical: 6, borderRadius: radius.pill, backgroundColor: remaining < 120_000 ? colors.warningSoft : colors.primarySoft }}>
            <Ionicons name="timer-outline" size={16} color={remaining < 120_000 ? colors.warning : colors.primaryInk} />
            <T variant="small" color={remaining < 120_000 ? colors.warning : colors.primaryInk} style={{ fontFamily: fonts.bold, fontVariant: ['tabular-nums'] }}>
              {t('booking.pay_before')} {formatCountdown(remaining)}
            </T>
          </View>
        ) : null}

        {b.status === 'PAID_AWAITING_HOTEL' ? <NotifyMeCard /> : null}

        {paymentPending ? (
          <View style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1, borderColor: colors.primary, backgroundColor: colors.primarySoft }}>
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md }}>
              <ActivityIndicator color={colors.primary} />
              <T variant="h3" style={{ flex: 1 }}>{t('booking.payment_pending_title')}</T>
            </View>
            <T color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('booking.payment_pending_body')}</T>
            <Button variant="secondary" title={t('booking.open_payment_again')} loading={paying} onPress={pay} style={{ marginTop: spacing.md }} />
          </View>
        ) : null}

        {held && !paymentPending ? (
          <View style={{ marginTop: spacing.xl }}>
            {b.payment === 'failed' ? (
              <View style={{ flexDirection: 'row', gap: spacing.sm, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.dangerSoft, marginBottom: spacing.md }}>
                <Ionicons name="alert-circle-outline" size={20} color={colors.danger} />
                <T color={colors.danger} style={{ flex: 1 }}>{t('booking.payment_failed')}</T>
              </View>
            ) : null}
            <T variant="displaySm">{t('booking.choose_payment')}</T>
            <PaymentOption icon="card-outline" title={t('booking.chapa')} note={t('booking.chapa_note')} selected={method === 'chapa'} onPress={() => setMethod('chapa')} />
            {/* The ways the guest can pay on Chapa's page, as small badges (text only). */}
            <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.xs, marginTop: -spacing.xs }}>
              {['Telebirr', 'CBE Birr', 'M-Pesa', 'Bank', 'Visa / Mastercard'].map((name) => (
                <View key={name} style={{ paddingHorizontal: spacing.sm, paddingVertical: 3, borderRadius: radius.sm, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
                  <T variant="label" color={colors.inkSoft}>{name}</T>
                </View>
              ))}
            </View>
            {b.pay_at_hotel && b.discount_santim === 0 ? (
              <PaymentOption icon="business-outline" title={t('booking.pay_at_hotel')} note={t('booking.pay_at_hotel_note')} selected={method === 'pay_at_hotel'} onPress={() => setMethod('pay_at_hotel')} />
            ) : null}
            {b.pay_at_hotel && b.discount_santim > 0 ? <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('booking.coupon_online_only')}</T> : null}
            {isMockApi && method !== 'pay_at_hotel' ? (
              <View style={{ marginTop: spacing.md }}><Badge tone="warning" icon="⚠" label="SIMULATED PAYMENT · no money moves" /></View>
            ) : null}
            {payError ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {payError}</T> : null}
            <Button
              title={method === 'pay_at_hotel' ? t('booking.reserve') : t('booking.pay', { amount: total })}
              onPress={pay}
              loading={paying}
              style={{ marginTop: spacing.lg }}
            />
          </View>
        ) : null}

        <Reveal index={1} style={[{ backgroundColor: colors.surface, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, padding: spacing.lg, marginTop: spacing.xl }, shadow.card]}>
          <T variant="displaySm">{b.property_name}</T>
          <T color={colors.inkSoft} style={{ marginBottom: spacing.sm }}>{b.address}</T>
          <Line label={t('booking.reference')} value={b.reference} />
          <Line label={t('booking.room')} value={b.rooms > 1 ? `${b.rooms} × ${b.room_name}` : b.room_name} />
          {b.meal === 'breakfast' ? <Line label={t('booking.meal_line')} value={t('checkout.meal_breakfast')} /> : null}
          {!b.refundable ? <Line label={t('booking.policy_line')} value={t('property.option_nonrefundable')} /> : null}
          {b.coupon_code ? <Line label={t('booking.coupon_row', { code: b.coupon_code })} value={`− ${price(b.discount_santim, b.currency)}`} /> : null}
          <Line label={t('booking.check_in')} value={`${shortDate(b.check_in, months)} · ${b.check_in_time}`} />
          <Line label={t('booking.check_out')} value={`${shortDate(b.check_out, months)} · ${b.check_out_time}`} />
          <Line label={t('booking.guests')} value={b.children > 0 ? t('booking.adults_children', { adults: b.guests - b.children, children: b.children }) : String(b.guests)} />
          {b.booked_for_other ? <Line label={t('checkout.staying')} value={`${b.guest_name}${b.guest_phone ? ` · ${b.guest_phone}` : ''}`} /> : null}
          {b.special_requests ? <Line label={t('checkout.your_requests')} value={b.special_requests} /> : null}
          {b.payment_method ? <Line label={t('booking.paid_with')} value={t(`booking.${b.payment_method}`)} /> : null}
          <View style={{ height: 1, backgroundColor: colors.line, marginVertical: spacing.sm }} />
          <Line label={t('booking.total')} value={total} />
        </Reveal>

        <TripTimeline booking={b} />

        {b.can_cancel ? <CancelCard booking={b} months={months} onChanged={onBookingChanged} /> : null}
        <ReviewCard booking={b} onChanged={onBookingChanged} />

        {expired ? <Button title={t('booking.try_again')} onPress={() => router.replace('/')} style={{ marginTop: spacing.xl }} /> : null}
        {confirmed ? (
          <Button
            variant="secondary"
            icon="calendar-outline"
            title={t('booking.add_to_calendar')}
            onPress={async () => {
              const ok = await addStayToCalendar({
                title: b.property_name, details: `${t('booking.reference')}: ${b.reference}`, location: b.address,
                checkIn: b.check_in, checkOut: b.check_out, inTime: b.check_in_time, outTime: b.check_out_time,
              });
              if (!ok) Alert.alert(t('common.cannot_open_title'), t('common.cannot_open_body', { value: `${b.check_in} – ${b.check_out}` }));
            }}
            style={{ marginTop: spacing.xl }}
          />
        ) : null}
        {confirmed ? (
          <Button icon="qr-code-outline" title={t('booking.view_voucher')} onPress={() => router.push({ pathname: '/voucher/[id]', params: { id } })} style={{ marginTop: spacing.xl }} />
        ) : null}
        <Button
          variant="secondary"
          icon="call-outline"
          title={t('booking.call_hotel')}
          onPress={async () => {
            if (!(await callPhone(b.property_phone))) Alert.alert(t('common.cannot_open_title'), t('common.cannot_open_body', { value: b.property_phone }));
          }}
          style={{ marginTop: spacing.md }}
        />
        <Button variant="ghost" title={t('booking.my_bookings')} onPress={goBookings} style={{ marginTop: spacing.sm }} />
      </ScrollView>
    </SafeAreaView>
  );
}
