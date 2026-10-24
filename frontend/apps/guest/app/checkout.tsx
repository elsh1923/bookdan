import { Ionicons } from '@expo/vector-icons';
import { useQueryClient } from '@tanstack/react-query';
import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import { useEffect, useRef, useState } from 'react';
import { ActivityIndicator, KeyboardAvoidingView, Platform, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { maskId, newIdempotencyKey } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Animated, Button, EmptyState, FadeIn, Pop, ProgressFill, StepTransition, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { useAuth } from '../src/auth';
import { usePrefs } from '../src/store';
import { CouponBox } from '../src/CouponBox';
import { Field } from '../src/Field';
import { PolicyCard } from '../src/RoomOptions';
import { FOOTER_CLEARANCE, StickyFooter } from '../src/StickyFooter';
import { price, shortDate } from '../src/format';
import { useProfile, useQuote } from '../src/queries';

function Row({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline', marginTop: spacing.md }}>
      <T variant={strong ? 'displaySm' : 'body'} color={strong ? colors.ink : colors.inkSoft} style={{ flex: 1, marginRight: spacing.md }}>{label}</T>
      <T variant={strong ? 'displaySm' : 'body'} color={colors.ink}>{value}</T>
    </View>
  );
}

function StayTile({ icon, title, line1, line2 }: { icon: keyof typeof Ionicons.glyphMap; title: string; line1: string; line2?: string }) {
  return (
    <View style={{ width: 164, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
      <Ionicons name={icon} size={20} color={colors.primaryDark} />
      <T variant="label" color={colors.muted} style={{ marginTop: spacing.sm }}>{title.toUpperCase()}</T>
      <T variant="h3" numberOfLines={2}>{line1}</T>
      {line2 ? <T variant="small" color={colors.inkSoft} numberOfLines={2}>{line2}</T> : null}
    </View>
  );
}

/** MakeMyTrip-style progress: three numbered steps, the finished ones ticked. */
function Stepper({ step, labels }: { step: number; labels: string[] }) {
  return (
    <View style={{ flexDirection: 'row', paddingHorizontal: spacing.lg, marginTop: spacing.sm }} accessibilityLabel={`${step + 1} / ${labels.length}`}>
      {labels.map((label, i) => {
        const done = i < step;
        const active = i === step;
        return (
          <View key={label} style={{ flex: 1, alignItems: 'center' }}>
            {i < labels.length - 1 ? <View style={{ position: 'absolute', top: 13, left: '50%', right: '-50%' }}><ProgressFill progress={done ? 1 : 0} /></View> : null}
            <View style={{ width: 28, height: 28, borderRadius: 14, alignItems: 'center', justifyContent: 'center', backgroundColor: done || active ? colors.primary : colors.fill }}>
              {done ? <Pop><Ionicons name="checkmark" size={16} color="#fff" /></Pop> : <T variant="small" color={active ? '#fff' : colors.muted}>{i + 1}</T>}
            </View>
            <T variant="small" color={active ? colors.ink : colors.muted} numberOfLines={1} style={{ marginTop: 4 }}>{label}</T>
          </View>
        );
      })}
    </View>
  );
}

export default function Checkout() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const p = useLocalSearchParams<{ id: string; room: string; in: string; out: string; guests: string; rooms?: string; meal?: string; refundable?: string }>();
  // The room option chosen on the hotel page (room only and refundable unless the guest picked otherwise).
  const meal: 'room_only' | 'breakfast' = p.meal === 'breakfast' ? 'breakfast' : 'room_only';
  const refundable = p.refundable !== 'false';
  const token = useAuth((s) => s.token);

  // The name is kept on the phone while typing, so a session that expires mid-checkout loses nothing.
  const name = usePrefs((s) => s.draftGuestName);
  const setName = usePrefs((s) => s.setDraftGuestName);
  const [nameError, setNameError] = useState<string | null>(null);
  const [rooms, setRooms] = useState(Math.min(9, Math.max(1, Number(p.rooms) || 1)));
  const [step, setStep] = useState(0); // 0 review, 1 guest details, 2 pay
  const [busy, setBusy] = useState(false);
  const [holdError, setHoldError] = useState<string | null>(null);
  const [coupon, setCoupon] = useState<string | null>(null); // the code the server accepted
  const [couponNote, setCouponNote] = useState<string | null>(null);
  // Created once per checkout attempt and reused if the request is retried, so a retry can never create a second hold.
  const attemptKey = useRef<string | null>(null);

  const quote = useQuote(
    { property_id: p.id, room_type_id: p.room, check_in: p.in, check_out: p.out, guests: Math.max(Number(p.guests) || 1, rooms), rooms, meal, refundable, coupon_code: coupon ?? undefined },
    !!token,
  );
  // A coupon that stops working while the guest is on this screen (it ran out, or expired) is dropped and the guest is told.
  useEffect(() => {
    if (coupon && quote.error instanceof ApiError && quote.error.code === 'COUPON_INVALID') {
      setCoupon(null);
      setCouponNote(quote.error.message);
    }
  }, [coupon, quote.error]);

  // The hotel receives the guest's name and ID with the request, so the guest must have saved them.
  const profile = useProfile(!!token);
  useEffect(() => {
    if (profile.data && !name) setName(profile.data.full_name);
  }, [profile.data]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!token) {
    const next = `/checkout?id=${p.id}&room=${p.room}&in=${p.in}&out=${p.out}&guests=${p.guests ?? 1}&rooms=${rooms}&meal=${meal}&refundable=${refundable}`;
    return <Redirect href={{ pathname: '/signin', params: { next } }} />;
  }

  const months = i18n.t('months.short', { returnObjects: true }) as unknown as string[];
  const leave = () => (router.canGoBack() ? router.back() : router.replace('/'));
  const back = () => (step > 0 ? setStep(step - 1) : leave());
  const header = (
    <View style={{ paddingHorizontal: spacing.lg }}>
      <Pressable accessibilityLabel={t('common.back')} onPress={back} style={{ width: 44, height: 44, justifyContent: 'center' }}>
        <Ionicons name="arrow-back" size={24} color={colors.ink} />
      </Pressable>
    </View>
  );

  if (quote.isPending) {
    return <SafeAreaView style={{ flex: 1 }}>{header}<View style={{ flex: 1, justifyContent: 'center' }}><ActivityIndicator size="large" color={colors.primary} /></View></SafeAreaView>;
  }
  if (quote.isError) {
    const soldOut = quote.error instanceof ApiError && quote.error.code === 'SOLD_OUT';
    const optionOff = quote.error instanceof ApiError && quote.error.code === 'OPTION_UNAVAILABLE';
    if (coupon && quote.error instanceof ApiError && quote.error.code === 'COUPON_INVALID') {
      return <SafeAreaView style={{ flex: 1 }}>{header}<View style={{ flex: 1, justifyContent: 'center' }}><ActivityIndicator size="large" color={colors.primary} /></View></SafeAreaView>;
    }
    return (
      <SafeAreaView style={{ flex: 1 }}>
        {header}
        {optionOff
          ? <EmptyState icon="🛏️" title={t('checkout.option_unavailable')} body="" actionLabel={t('checkout.back_to_property')} onAction={leave} />
          : soldOut
          ? <EmptyState icon="🛏️" title={t('checkout.sold_out_title')} body={t('checkout.sold_out_body')} actionLabel={t('checkout.back_to_property')} onAction={leave} />
          : <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => quote.refetch()} />}
      </SafeAreaView>
    );
  }

  const q = quote.data;
  const switching = quote.isPlaceholderData; // a new price for a different number of rooms is on its way
  const changeRooms = (n: number) => {
    attemptKey.current = null; // a different request must never reuse the old retry key
    setRooms(n);
  };
  const needsProfile = profile.isSuccess && !profile.data;

  // The server decides whether a code works (dates, uses, hotel, minimum): ask it for a price with the code and keep the code if it answers.
  async function applyCoupon(code: string): Promise<string | null> {
    try {
      await api.createQuote({ property_id: p.id, room_type_id: p.room, check_in: p.in, check_out: p.out, guests: Math.max(Number(p.guests) || 1, rooms), rooms, meal, refundable, coupon_code: code });
      attemptKey.current = null; // a different price must never reuse the old retry key
      setCouponNote(null);
      setCoupon(code);
      return null;
    } catch (e) {
      return e instanceof ApiError && e.code === 'COUPON_INVALID' ? e.message : t('checkout.coupon_failed');
    }
  }
  const removeCoupon = () => {
    attemptKey.current = null;
    setCoupon(null);
  };
  // What the coupon took off the room charges (before VAT), for the price lines.
  const listBase = q.nightly_santim * q.nights * q.rooms;
  const roomDiscount = Math.max(0, listBase - q.subtotal_santim);

  function next() {
    if (step === 1) {
      if (needsProfile) return router.push('/guest-details');
      if (name.trim().length < 2) return setNameError(t('checkout.name_required'));
      setNameError(null);
    }
    setStep(step + 1);
  }

  async function hold() {
    if (switching) return;
    if (needsProfile) return router.push('/guest-details');
    if (name.trim().length < 2) return setNameError(t('checkout.name_required'));
    setNameError(null);
    setHoldError(null);
    setBusy(true);
    try {
      attemptKey.current ??= newIdempotencyKey();
      const b = await api.createHold({ quote_id: q.id, guest_name: name.trim(), idempotency_key: attemptKey.current });
      attemptKey.current = null;
      setName('');
      qc.invalidateQueries({ queryKey: ['bookings'] });
      router.replace({ pathname: '/booking/[id]', params: { id: b.id } });
    } catch (e) {
      if (e instanceof ApiError && e.code === 'SOLD_OUT') setHoldError(t('checkout.sold_out_title'));
      else if (e instanceof ApiError && e.code === 'PROFILE_INCOMPLETE') router.push('/guest-details');
      else if (e instanceof ApiError && e.code === 'COUPON_INVALID') { attemptKey.current = null; setCoupon(null); setCouponNote(e.message); }
      else if (e instanceof ApiError && e.code === 'QUOTE_EXPIRED') { attemptKey.current = null; setHoldError(t('checkout.expired')); quote.refetch(); }
      else setHoldError(t('results.error_body'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        {header}
        <ScrollView contentContainerStyle={{ paddingBottom: FOOTER_CLEARANCE + spacing.lg }} keyboardShouldPersistTaps="handled">
          <StepTransition stepKey={step}>
          <T variant="display" style={{ paddingHorizontal: spacing.lg }}>{t(step === 0 ? 'checkout.step_review_title' : step === 1 ? 'checkout.step_guest_title' : 'checkout.step_pay_title')}</T>
          <Stepper step={step} labels={[t('checkout.step_review'), t('checkout.step_guest'), t('checkout.step_pay')]} />

          {step !== 1 ? (
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.sm, paddingTop: spacing.lg }}>
              <StayTile icon="bed-outline" title={q.property_name} line1={q.room_name} />
              <StayTile
                icon="calendar-outline"
                title={t('search.check_in')}
                line1={`${shortDate(q.check_in, months)} – ${shortDate(q.check_out, months)}`}
                line2={t('common.night', { count: q.nights })}
              />
              <StayTile icon="people-outline" title={t('search.guests')} line1={t('common.guest', { count: q.guests })} line2={t('common.room', { count: q.rooms })} />
            </ScrollView>
          ) : null}

          {step === 0 ? (
            <View style={{ marginHorizontal: spacing.lg, marginTop: spacing.lg, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line }}>
              <View style={{ flex: 1 }}>
                <T variant="h3">{t('checkout.rooms_label')}</T>
                {q.rooms_available <= 3 ? <T variant="small" color={colors.warning}>{t('property.rooms_left', { count: q.rooms_available })}</T> : null}
              </View>
              <Pressable accessibilityLabel={`${t('checkout.rooms_label')} -`} disabled={rooms <= 1} onPress={() => changeRooms(rooms - 1)} style={{ width: 44, height: 44, alignItems: 'center', justifyContent: 'center', opacity: rooms <= 1 ? 0.3 : 1 }}>
                <T variant="h2" color={colors.primaryInk}>−</T>
              </Pressable>
              <T variant="h3" style={{ minWidth: 28, textAlign: 'center' }}>{rooms}</T>
              <Pressable accessibilityLabel={`${t('checkout.rooms_label')} +`} disabled={rooms >= Math.min(9, q.rooms_available)} onPress={() => changeRooms(rooms + 1)} style={{ width: 44, height: 44, alignItems: 'center', justifyContent: 'center', opacity: rooms >= Math.min(9, q.rooms_available) ? 0.3 : 1 }}>
                <T variant="h2" color={colors.primaryInk}>+</T>
              </Pressable>
            </View>
          ) : null}

          {step === 0 ? (
            <View style={{ paddingHorizontal: spacing.lg, marginTop: spacing.lg, gap: spacing.md }}>
              <PolicyCard meal={q.meal} refundable={q.refundable} />
              <CouponBox applied={q.coupon_code} saving={q.discount_santim > 0 ? price(q.discount_santim, q.currency) : null} onApply={applyCoupon} onRemove={removeCoupon} />
              {couponNote ? <T variant="small" color={colors.danger}>✕ {couponNote}</T> : null}
            </View>
          ) : null}

          <View style={{ paddingHorizontal: spacing.lg }}>
            {step === 1 ? (
              <>
                {needsProfile ? (
                  <View style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1.5, borderColor: colors.primary, backgroundColor: colors.primarySoft }}>
                    <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                      <Ionicons name="id-card-outline" size={22} color={colors.primaryInk} />
                      <T variant="h3" color={colors.primaryInk} style={{ flex: 1 }}>{t('checkout.need_id_title')}</T>
                    </View>
                    <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('checkout.need_id_body')}</T>
                    <Button icon="add-circle-outline" title={t('checkout.add_id')} onPress={() => router.push('/guest-details')} style={{ marginTop: spacing.md }} />
                  </View>
                ) : null}
                <Field
                  label={t('checkout.guest_name')}
                  hint={t('checkout.guest_name_hint')}
                  value={name}
                  onChangeText={setName}
                  autoComplete="name"
                  textContentType="name"
                  autoCapitalize="words"
                  error={nameError}
                />
                {profile.data ? (
                  <View style={{ flexDirection: 'row', gap: spacing.sm, alignItems: 'center', marginTop: spacing.md }}>
                    <Ionicons name="id-card-outline" size={18} color={colors.inkSoft} />
                    <T variant="small" color={colors.inkSoft} style={{ flex: 1 }}>
                      {t('checkout.profile_shared', { type: t(`idType.${profile.data.id_type}`), masked: maskId(profile.data.id_last4) })}
                    </T>
                    <Pressable onPress={() => router.push('/guest-details')} hitSlop={8}><T variant="small" color={colors.primaryInk}>{t('checkout.edit_id')}</T></Pressable>
                  </View>
                ) : null}
              </>
            ) : (
              <>
                {step === 2 ? (
                  <View style={{ marginTop: spacing.lg, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line }}>
                    <T variant="label" color={colors.muted}>{t('checkout.guest_name').toUpperCase()}</T>
                    <T variant="h3">{name.trim()}</T>
                    {profile.data ? <T variant="small" color={colors.inkSoft}>{t(`idType.${profile.data.id_type}`)} {maskId(profile.data.id_last4)}</T> : null}
                  </View>
                ) : null}

                <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('checkout.price').toUpperCase()}</T>
                <Row label={q.rooms > 1 ? t('checkout.nightly_rooms', { price: price(q.nightly_santim, q.currency), rooms: t('common.room', { count: q.rooms }), nights: t('common.night', { count: q.nights }) }) : t('checkout.nightly', { price: price(q.nightly_santim, q.currency), nights: q.nights })} value={price(roomDiscount > 0 ? listBase : q.subtotal_santim, q.currency)} />
                {roomDiscount > 0 && q.coupon_code ? (
                  <View style={{ flexDirection: 'row', justifyContent: 'space-between', marginTop: spacing.md }}>
                    <T color={colors.success} style={{ flex: 1, marginRight: spacing.md }}>{t('checkout.coupon_line', { code: q.coupon_code })}</T>
                    <T color={colors.success}>− {price(roomDiscount, q.currency)}</T>
                  </View>
                ) : null}
                <Row label={t('checkout.vat')} value={price(q.vat_santim, q.currency)} />
                <View style={{ height: 1, backgroundColor: colors.line, marginTop: spacing.md }} />
                <Row strong label={t('checkout.total')} value={price(q.total_santim, q.currency)} />
                {q.discount_santim > 0 ? <T variant="small" color={colors.success} style={{ textAlign: 'right', marginTop: spacing.xs }}>{t('checkout.coupon_saved', { amount: price(q.discount_santim, q.currency) })}</T> : null}
                {step === 2 ? <View style={{ marginTop: spacing.lg }}><PolicyCard meal={q.meal} refundable={q.refundable} /></View> : null}

                <View style={{ flexDirection: 'row', gap: spacing.sm, alignItems: 'center', marginTop: spacing.lg }}>
                  <Ionicons name="shield-checkmark-outline" size={18} color={colors.primaryDark} />
                  <T variant="small" color={colors.inkSoft} style={{ flex: 1 }}>{t('checkout.hold_note')}</T>
                </View>
              </>
            )}
            {holdError ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {holdError}</T> : null}
          </View>
          </StepTransition>
        </ScrollView>
      </KeyboardAvoidingView>

      <StickyFooter>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md }}>
          <View>
            <T variant="small" color={colors.muted}>{t('checkout.total')}</T>
            <Animated.View key={q.total_santim} entering={FadeIn.duration(260)}><T variant="h3">{price(q.total_santim, q.currency)}</T></Animated.View>
          </View>
          <Button
            style={{ flex: 1 }}
            title={step === 2 ? t('checkout.hold') : step === 1 && needsProfile ? t('checkout.add_id') : t('checkout.continue')}
            onPress={step === 2 ? hold : next}
            loading={busy || switching}
          />
        </View>
      </StickyFooter>
    </SafeAreaView>
  );
}
