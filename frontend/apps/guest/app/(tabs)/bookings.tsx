import { useRouter } from 'expo-router';
import { useEffect } from 'react';
import { ActivityIndicator, FlatList, Pressable, RefreshControl, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, T, colors, radius, shadow, spacing } from '@bookdan/ui';
import { useAuth } from '../../src/auth';
import { price, shortDate } from '../../src/format';
import { useBookings } from '../../src/queries';
import { StatusBadge } from '../../src/StatusBadge';
import { pruneVouchers } from '../../src/voucherStore';

// E3 adds the offline voucher on confirmed bookings.
export default function Bookings() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const token = useAuth((s) => s.token);
  const query = useBookings(!!token);
  const months = i18n.t('months.short', { returnObjects: true }) as unknown as string[];
  // Vouchers are kept until the stay ends, then removed.
  useEffect(() => {
    if (query.data) pruneVouchers(query.data).catch(() => undefined);
  }, [query.data]);

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
        <ActivityIndicator color={colors.primary} style={{ marginTop: spacing.xxl }} />
      ) : query.isError ? (
        <EmptyState icon="⚠️" title={t('bookings.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => query.refetch()} />
      ) : (
        <FlatList
          data={query.data}
          keyExtractor={(b) => b.id}
          contentContainerStyle={{ padding: spacing.lg, paddingTop: 0, flexGrow: 1 }}
          refreshControl={<RefreshControl refreshing={query.isRefetching} onRefresh={() => query.refetch()} tintColor={colors.primary} />}
          ListEmptyComponent={
            <EmptyState icon="🎫" title={t('bookings.empty_title')} body={t('bookings.empty_body')} actionLabel={t('bookings.find')} onAction={() => router.navigate('/')} />
          }
          renderItem={({ item: b }) => (
            <Pressable
              accessibilityRole="button"
              onPress={() => router.push({ pathname: '/booking/[id]', params: { id: b.id } })}
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
            </Pressable>
          )}
        />
      )}
    </SafeAreaView>
  );
}
