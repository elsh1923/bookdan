import { Ionicons } from '@expo/vector-icons';
import { useQueryClient } from '@tanstack/react-query';
import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import { useRef, useState } from 'react';
import { ActivityIndicator, KeyboardAvoidingView, Platform, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { newIdempotencyKey } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, EmptyState, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { useAuth } from '../src/auth';
import { usePrefs } from '../src/store';
import { Field } from '../src/Field';
import { FOOTER_CLEARANCE, StickyFooter } from '../src/StickyFooter';
import { price, shortDate } from '../src/format';
import { useQuote } from '../src/queries';

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

export default function Checkout() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const p = useLocalSearchParams<{ id: string; room: string; in: string; out: string; guests: string }>();
  const token = useAuth((s) => s.token);

  // The name is kept on the phone while typing, so a session that expires mid-checkout loses nothing.
  const name = usePrefs((s) => s.draftGuestName);
  const setName = usePrefs((s) => s.setDraftGuestName);
  const [nameError, setNameError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [holdError, setHoldError] = useState<string | null>(null);
  // Created once per checkout attempt and reused if the request is retried, so a retry can never create a second hold.
  const attemptKey = useRef<string | null>(null);

  const quote = useQuote(
    { property_id: p.id, room_type_id: p.room, check_in: p.in, check_out: p.out, guests: Number(p.guests) || 1 },
    !!token,
  );

  if (!token) {
    const next = `/checkout?id=${p.id}&room=${p.room}&in=${p.in}&out=${p.out}&guests=${p.guests ?? 1}`;
    return <Redirect href={{ pathname: '/signin', params: { next } }} />;
  }

  const months = i18n.t('months.short', { returnObjects: true }) as unknown as string[];
  const back = () => (router.canGoBack() ? router.back() : router.replace('/'));
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
    return (
      <SafeAreaView style={{ flex: 1 }}>
        {header}
        {soldOut
          ? <EmptyState icon="🛏️" title={t('checkout.sold_out_title')} body={t('checkout.sold_out_body')} actionLabel={t('checkout.back_to_property')} onAction={back} />
          : <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => quote.refetch()} />}
      </SafeAreaView>
    );
  }

  const q = quote.data;

  async function hold() {
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
          <T variant="display" style={{ paddingHorizontal: spacing.lg }}>{t('checkout.title')}</T>

          <T variant="label" color={colors.muted} style={{ paddingHorizontal: spacing.lg, marginTop: spacing.lg }}>{t('checkout.stay').toUpperCase()}</T>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.sm, paddingTop: spacing.sm }}>
            <StayTile
              icon="calendar-outline"
              title={t('search.check_in')}
              line1={`${shortDate(q.check_in, months)} – ${shortDate(q.check_out, months)}`}
              line2={t('common.night', { count: q.nights })}
            />
            <StayTile icon="people-outline" title={t('search.guests')} line1={t('common.guest', { count: q.guests })} />
            <StayTile icon="bed-outline" title={q.property_name} line1={q.room_name} />
          </ScrollView>

          <View style={{ paddingHorizontal: spacing.lg }}>
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

            <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('checkout.price').toUpperCase()}</T>
            <Row label={t('checkout.nightly', { price: price(q.nightly_santim, q.currency), nights: q.nights })} value={price(q.subtotal_santim, q.currency)} />
            <Row label={t('checkout.vat')} value={price(q.vat_santim, q.currency)} />
            <View style={{ height: 1, backgroundColor: colors.line, marginTop: spacing.md }} />
            <Row strong label={t('checkout.total')} value={price(q.total_santim, q.currency)} />

            <View style={{ flexDirection: 'row', gap: spacing.sm, alignItems: 'center', marginTop: spacing.lg }}>
              <Ionicons name="shield-checkmark-outline" size={18} color={colors.primaryDark} />
              <T variant="small" color={colors.inkSoft} style={{ flex: 1 }}>{t('checkout.hold_note')}</T>
            </View>
            {holdError ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {holdError}</T> : null}
          </View>
        </ScrollView>
      </KeyboardAvoidingView>

      <StickyFooter>
        <Button title={t('checkout.hold')} onPress={hold} loading={busy} />
      </StickyFooter>
    </SafeAreaView>
  );
}
