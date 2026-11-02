import { useRouter } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { FlatList, RefreshControl, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import type { Booking } from '@bookdan/domain';
import { todayAddis } from '@bookdan/domain';
import { splitBookings } from '../../src/bookingHistory';
import { useTranslation } from '@bookdan/i18n';
import { Chip, EmptyState, PressableScale, Reveal, Skeleton, T, colors, radius, shadow, spacing } from '@bookdan/ui';
import { useAuth } from '../../src/auth';
import { priceExact as price, shortDate } from '../../src/format';
import { useBookings } from '../../src/queries';
import { StatusBadge } from '../../src/StatusBadge';
import { pruneVouchers } from '../../src/voucherStore';
import { useDisplay } from '../../src/display';

type Tab = 'upcoming' | 'past' | 'cancelled' | 'failed';

// The guest's bookings in four tabs: upcoming, completed, cancelled and failed.
export default function Bookings() {
  useDisplay((st) => st.version); // redraw when the display currency changes
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const token = useAuth((s) => s.token);
  const query = useBookings(!!token);
  const months = i18n.t('months.short', { returnObjects: true }) as unknown as string[];
  // Vouchers are kept until the stay ends, then removed.
  useEffect(() => {
    if (query.data) pruneVouchers(query.data).catch(() => undefined);
  }, [query.data]);

  // One tab at a time: upcoming, completed, cancelled, failed.
  const [tab, setTab] = useState<Tab>('upcoming');
  const groups = useMemo(() => splitBookings(query.data ?? [], todayAddis()), [query.data]);
  const shown = groups[tab];
  const tabs: { key: Tab; label: string; count: number }[] = [
    { key: 'upcoming', label: t('bookings.upcoming'), count: groups.upcoming.length },
    { key: 'past', label: t('bookings.completed'), count: groups.past.length },
    { key: 'cancelled', label: t('bookings.cancelled'), count: groups.cancelled.length },
    { key: 'failed', label: t('bookings.failed'), count: groups.failed.length },
  ];
  const emptyText: Record<Tab, { title: string; body: string }> = {
    upcoming: { title: t('bookings.empty_title'), body: t('bookings.empty_body') },
    past: { title: t('bookings.empty_completed'), body: t('bookings.empty_completed_body') },
    cancelled: { title: t('bookings.empty_cancelled'), body: t('bookings.empty_cancelled_body') },
    failed: { title: t('bookings.empty_failed'), body: t('bookings.empty_failed_body') },
  };

  const title = <T variant="h1" style={{ padding: spacing.lg }}>{t('bookings.title')}</T>;

  if (!token) {
    return (
      <SafeAreaView style={{ flex: 1 }} edges={['top']}>
        {title}
        <EmptyState
          icon="🎫"
          title={t('bookings.sign_in_title')}
          body={t('bookings.sign_in_body')}
          actionLabel={t('bookings.sign_in')}
          onAction={() => router.push('/signin')}
        />
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={{ flex: 1 }} edges={['top']}>
      {title}
      {query.isPending ? (
        <View style={{ padding: spacing.lg, paddingTop: 0, gap: spacing.md }}>
          {[0, 1, 2].map((i) => <Skeleton key={i} height={118} radius={radius.lg} />)}
        </View>
      ) : query.isError ? (
        <EmptyState icon="⚠️" title={t('bookings.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => query.refetch()} />
      ) : (
        <FlatList
          data={shown}
          keyExtractor={(b) => b.id}
          contentContainerStyle={{ padding: spacing.lg, paddingTop: 0, flexGrow: 1 }}
          refreshControl={<RefreshControl refreshing={query.isRefetching} onRefresh={() => query.refetch()} tintColor={colors.primary} />}
          ListHeaderComponent={
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: spacing.sm, paddingBottom: spacing.md }}>
              {tabs.map((x) => (
                <Chip key={x.key} label={`${x.label} (${x.count})`} selected={tab === x.key} onPress={() => setTab(x.key)} />
              ))}
            </ScrollView>
          }
          ListEmptyComponent={
            <EmptyState icon="🎫" title={emptyText[tab].title} body={emptyText[tab].body} actionLabel={tab === 'upcoming' ? t('bookings.find') : undefined} onAction={tab === 'upcoming' ? () => router.navigate('/') : undefined} />
          }
          renderItem={({ item: b, index }) => (
            <Reveal index={index}>
            <PressableScale
              accessibilityRole="button"
              onPress={() => router.push({ pathname: '/booking/[id]', params: { id: b.id } })}
              scaleTo={0.98}
              style={[{ backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.lg, marginBottom: spacing.md }, shadow.card]}
            >
              <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'flex-start', gap: spacing.sm }}>
                <View style={{ flex: 1 }}>
                  <T variant="h3">{b.property_name}</T>
                  <T variant="small" color={colors.inkSoft}>{b.room_name}</T>
                </View>
                <StatusBadge status={b.status} />
              </View>
              <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>
                {shortDate(b.check_in, months)} – {shortDate(b.check_out, months)} · {t('common.night', { count: b.nights })}
              </T>
              <View style={{ flexDirection: 'row', justifyContent: 'space-between', marginTop: spacing.xs }}>
                <T variant="label" color={colors.muted}>{b.reference}</T>
                <T variant="h3" color={colors.primaryInk}>{price(b.total_santim, b.currency)}</T>
              </View>
            </PressableScale>
            </Reveal>
          )}
        />
      )}
    </SafeAreaView>
  );
}
