import { Ionicons } from '@expo/vector-icons';
import { RefreshControl, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import type { EarningLine, HotelEarnings } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Badge, EmptyState, Reveal, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { price, shortDate } from '../../src/format';
import { useEarnings } from '../../src/queries';

function Tile({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <View style={{ flex: 1, padding: spacing.md, borderRadius: radius.md, backgroundColor: strong ? colors.primarySoft : colors.surface, borderWidth: strong ? 0 : 1, borderColor: colors.line }}>
      <T variant="label" color={colors.muted}>{label.toUpperCase()}</T>
      <T variant="h3" color={strong ? colors.primaryInk : colors.ink} style={{ marginTop: 2 }}>{value}</T>
    </View>
  );
}

function Split({ label, value }: { label: string; value: string }) {
  return (
    <View style={{ flexDirection: 'row', justifyContent: 'space-between', gap: spacing.md }}>
      <T variant="small" color={colors.inkSoft}>{label}</T>
      <T variant="small">{value}</T>
    </View>
  );
}

function Line({ l, months }: { l: EarningLine; months: string[] }) {
  const { t } = useTranslation();
  return (
    <View style={{ padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface, marginBottom: spacing.sm }}>
      <View style={{ flexDirection: 'row', justifyContent: 'space-between', gap: spacing.sm }}>
        <T variant="h3" style={{ flex: 1 }} numberOfLines={1}>{l.guest_name}</T>
        <Badge tone={l.ready ? 'success' : 'neutral'} icon={l.ready ? '✓' : '…'} label={t(l.ready ? 'partner.earnings_state_ready' : 'partner.earnings_state_upcoming')} />
      </View>
      <T variant="small" color={colors.muted}>{l.reference} · {shortDate(l.check_in, months)} – {shortDate(l.check_out, months)}</T>
      <View style={{ marginTop: spacing.sm, gap: 2 }}>
        <Split label={t('partner.earnings_gross')} value={price(l.gross_santim)} />
        <Split label={t('partner.earnings_commission')} value={`− ${price(l.commission_santim)}`} />
        <Split label={t('partner.earnings_vat')} value={`− ${price(l.vat_santim)}`} />
        <View style={{ height: 1, backgroundColor: colors.line, marginVertical: 4 }} />
        <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
          <T>{t('partner.earnings_net')}</T>
          <T variant="h3">{price(l.net_santim)}</T>
        </View>
      </View>
    </View>
  );
}

function Hotel({ h, showName, months }: { h: HotelEarnings; showName: boolean; months: string[] }) {
  const { t } = useTranslation();
  return (
    <View style={{ marginTop: spacing.lg }}>
      {showName ? <T variant="h2" style={{ marginBottom: spacing.sm }}>{h.name}</T> : null}
      <Tile strong label={t('partner.earnings_to_pay')} value={price(h.to_pay_santim)} />
      <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
        <Tile label={t('partner.earnings_upcoming')} value={price(h.upcoming_santim)} />
        <Tile label={t('partner.earnings_paid')} value={price(h.paid_santim)} />
      </View>
      <T variant="small" color={colors.muted} style={{ marginTop: spacing.sm }}>{t('partner.earnings_note')}</T>

      <T variant="h3" style={{ marginTop: spacing.xl, marginBottom: spacing.sm }}>{t('partner.earnings_payouts')}</T>
      {h.payouts.length === 0 ? (
        <T color={colors.inkSoft}>{t('partner.earnings_no_payouts')}</T>
      ) : (
        h.payouts.map((p) => (
          <View key={p.id} style={{ flexDirection: 'row', justifyContent: 'space-between', paddingVertical: spacing.sm, borderBottomWidth: 1, borderBottomColor: colors.line }}>
            <View style={{ flex: 1 }}>
              <T>{price(p.amount_santim)}</T>
              <T variant="small" color={colors.muted}>{p.reference}</T>
            </View>
            <T variant="small" color={colors.inkSoft}>{p.paid_at.slice(0, 10)}</T>
          </View>
        ))
      )}

      <T variant="h3" style={{ marginTop: spacing.xl, marginBottom: spacing.sm }}>{t('partner.earnings_bookings')}</T>
      {h.items.length === 0 ? <T color={colors.inkSoft}>{t('partner.earnings_none')}</T> : h.items.map((l) => <Line key={l.booking_id} l={l} months={months} />)}
    </View>
  );
}

/** What the owner has earned, what is waiting to be paid to them, and what was already paid. Owners only. */
export default function Earnings() {
  const { t, i18n } = useTranslation();
  const earnings = useEarnings();
  const months = i18n.t('months.short', { returnObjects: true }) as unknown as string[];
  const hotels = earnings.data?.hotels ?? [];

  return (
    <SafeAreaView style={{ flex: 1 }} edges={['top']}>
      <ScrollView
        contentContainerStyle={{ padding: spacing.lg, paddingBottom: spacing.xxl }}
        refreshControl={<RefreshControl refreshing={earnings.isRefetching} onRefresh={() => earnings.refetch()} tintColor={colors.primary} />}
      >
        <T variant="display">{t('partner.earnings_title')}</T>
        {earnings.isPending ? (
          <View style={{ marginTop: spacing.lg, gap: spacing.md }}>
            <Skeleton height={80} radius={radius.md} />
            <Skeleton height={140} radius={radius.md} />
          </View>
        ) : earnings.isError ? (
          <EmptyState
            icon={earnings.error instanceof ApiError && earnings.error.code === 'FORBIDDEN' ? '🔒' : '⚠️'}
            title={t(earnings.error instanceof ApiError && earnings.error.code === 'FORBIDDEN' ? 'partner.earnings_owner_only' : earnings.error instanceof ApiError && earnings.error.code === 'NETWORK' ? 'results.offline_title' : 'results.error_title')}
            body={t(earnings.error instanceof ApiError && earnings.error.code === 'NETWORK' ? 'results.offline_body' : 'results.error_body')}
            actionLabel={t('common.retry')}
            onAction={() => earnings.refetch()}
          />
        ) : (
          <Reveal>
            {hotels.map((h) => <Hotel key={h.id} h={h} showName={hotels.length > 1} months={months} />)}
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.xl }}>
              <Ionicons name="information-circle-outline" size={18} color={colors.muted} />
              <T variant="small" color={colors.muted} style={{ flex: 1 }}>{t('partner.earnings_pay_at_hotel')}</T>
            </View>
          </Reveal>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}
