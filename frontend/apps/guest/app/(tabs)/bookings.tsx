import { useRouter } from 'expo-router';
import { useEffect, useMemo } from 'react';
import { RefreshControl, SectionList, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import type { Booking } from '@bookdan/domain';
import { todayAddis } from '@bookdan/domain';
import { splitBookings } from '../../src/bookingHistory';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, PressableScale, Reveal, Skeleton, T, colors, radius, shadow, spacing } from '@bookdan/ui';
import { useAuth } from '../../src/auth';
import { priceExact as price, shortDate } from '../../src/format';
import { useBookings } from '../../src/queries';
import { StatusBadge } from '../../src/StatusBadge';
import { pruneVouchers } from '../../src/voucherStore';
import { useDisplay } from '../../src/display';

// The guest's bookings: the stays to come first, then the history of past stays.
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

  const sections = useMemo(() => {
    const { upcoming, past } = splitBookings(query.data ?? [], todayAddis());
    return [
      { key: 'upcoming', title: t('bookings.upcoming'), data: upcoming },
      { key: 'past', title: t('bookings.past'), data: past },
    ].filter((s) => s.data.length > 0);
  }, [query.data, t]);

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
        <SectionList
          sections={sections}
          keyExtractor={(b) => b.id}
          contentContainerStyle={{ padding: spacing.lg, paddingTop: 0, flexGrow: 1 }}
          stickySectionHeadersEnabled={false}
          refreshControl={<RefreshControl refreshing={query.isRefetching} onRefresh={() => query.refetch()} tintColor={colors.primary} />}
          ListEmptyComponent={
            <EmptyState icon="🎫" title={t('bookings.empty_title')} body={t('bookings.empty_body')} actionLabel={t('bookings.find')} onAction={() => router.navigate('/')} />
          }
          renderSectionHeader={({ section }) => (
            <T variant="h3" color={colors.inkSoft} style={{ marginTop: spacing.md, marginBottom: spacing.sm }}>{section.title}</T>
          )}
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
