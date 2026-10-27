import { Ionicons } from '@expo/vector-icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { nightsBetween, todayAddis } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { useAuth } from '../src/auth';
import { DateRangePicker } from '../src/DateRangePicker';
import { shortDate } from '../src/format';

/**
 * Move a confirmed booking to other dates. The server allows it only at exactly the same price, once, and while both check-ins are more
 * than 48 hours away; this screen only collects the dates and shows the server's answer.
 */
export default function ChangeDates() {
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
  const [error, setError] = useState<string | null>(null);
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
  if (!token) return <Redirect href="/signin" />;

  const close = () => (router.canGoBack() ? router.back() : router.replace('/bookings'));
  const b = booking.data;
  const picked = s && e;

  async function send() {
    if (!s || !e || !b) return;
    setBusy(true);
    setError(null);
    try {
      const updated = await api.changeBookingDates(b.id, { check_in: s, check_out: e });
      qc.setQueryData(['booking', b.id], updated);
      await qc.invalidateQueries({ queryKey: ['bookings'] });
      close();
    } catch (err) {
      const code = err instanceof ApiError ? err.code : '';
      setError(
        code === 'PRICE_DIFFERS' ? `${t('change.price_differs')}${err instanceof ApiError && err.message ? `\n\n${err.message}` : ''}`
          : code === 'SOLD_OUT' ? t('change.sold_out')
          : code === 'NEW_DATES_TOO_SOON' ? t('change.too_soon')
          : code === 'CHANGE_TOO_LATE' ? t('change.too_late')
          : code === 'CHANGE_LIMIT' ? t('change.limit')
          : code === 'NON_REFUNDABLE' || code === 'COUPON_NO_CHANGE' ? t('change.not_allowed')
          : code === 'NETWORK' ? t('results.offline_body')
          : t('change.failed'),
      );
      setBusy(false);
    }
  }

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
              title={picked ? `${t('change.confirm')} · ${shortDate(s, months)} – ${shortDate(e, months)} · ${t('common.night', { count: nightsBetween(s, e) })}` : t('change.confirm')}
              disabled={!picked}
              loading={busy}
              onPress={send}
            />
          </View>
        </>
      )}
    </SafeAreaView>
  );
}
