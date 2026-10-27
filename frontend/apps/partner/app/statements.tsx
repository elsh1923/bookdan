import { Ionicons } from '@expo/vector-icons';
import { useQuery } from '@tanstack/react-query';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, Share, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { todayAddis, type Statement } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, EmptyState, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { price } from '../src/format';

/** The months to choose from: this one and the five before it, newest first. */
function lastMonths(): string[] {
  const [y, m] = todayAddis().slice(0, 7).split('-').map(Number);
  return Array.from({ length: 6 }, (_, i) => {
    const d = new Date(Date.UTC(y, m - 1 - i, 1));
    return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`;
  });
}

function Row({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <View style={{ flexDirection: 'row', justifyContent: 'space-between', gap: spacing.md, paddingVertical: 3 }}>
      <T variant={strong ? 'h3' : 'small'} color={strong ? colors.ink : colors.inkSoft} style={{ flex: 1 }}>{label}</T>
      <T variant={strong ? 'h3' : 'small'}>{value}</T>
    </View>
  );
}

function summaryText(s: Statement, t: (k: string, o?: Record<string, unknown>) => string): string {
  const lines = [
    `BookDan · ${s.hotel} · ${s.month}`,
    `${t('partner.st_opening')}: ${price(s.opening_santim)}`,
    `${t('partner.st_online_share')}: ${price(s.online_hotel_share_santim)}`,
    `${t('partner.st_online_commission')}: ${price(s.online_commission_santim)}`,
    `${t('partner.st_online_vat')}: ${price(s.online_vat_santim)}`,
    `${t('partner.st_pah_collected')}: ${price(s.pay_at_hotel_collected_santim)}`,
    `${t('partner.st_pah_commission')}: ${price(s.pay_at_hotel_commission_santim + s.pay_at_hotel_vat_santim)}`,
    `${t('partner.st_paid')}: ${price(s.paid_santim)}`,
    `${t('partner.st_received')}: ${price(s.received_santim)}`,
    `${t('partner.st_closing')}: ${price(Math.abs(s.closing_santim))} (${s.closing_santim >= 0 ? t('partner.st_owed_to_you') : t('partner.st_owed_to_bookdan')})`,
    '',
    ...s.lines.map((l) => `${l.date} · ${l.reference} · ${l.guest_name} · ${price(l.net_santim)}`),
  ];
  return lines.join('\n');
}

/** A month's statement for the owner's hotel(s): the balance, the split of online bookings, pay-at-hotel commission, and every stay. */
export default function Statements() {
  const { t } = useTranslation();
  const router = useRouter();
  const { property } = useLocalSearchParams<{ property?: string }>();
  const months = lastMonths();
  const [month, setMonth] = useState(months[0]);
  const q = useQuery({ queryKey: ['statements', month, property ?? ''], queryFn: () => api.getStatements(month, property || undefined), retry: false });
  const ownerOnly = q.error instanceof ApiError && q.error.code === 'FORBIDDEN';

  return (
    <SafeAreaView style={{ flex: 1 }}>
      <View style={{ flexDirection: 'row', alignItems: 'center', paddingHorizontal: spacing.lg, gap: spacing.sm }}>
        <Pressable accessibilityLabel={t('common.back')} onPress={() => (router.canGoBack() ? router.back() : router.replace('/earnings'))} style={{ width: 44, height: 44, justifyContent: 'center' }}>
          <Ionicons name="arrow-back" size={24} color={colors.ink} />
        </Pressable>
        <T variant="h2" style={{ flex: 1 }}>{t('partner.statements')}</T>
      </View>
      <ScrollView contentContainerStyle={{ padding: spacing.lg }}>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm }}>
          {months.map((m) => <Chip key={m} label={m} selected={month === m} onPress={() => setMonth(m)} />)}
        </View>
        {q.isPending ? (
          <ActivityIndicator style={{ marginTop: spacing.xl }} color={colors.primary} />
        ) : ownerOnly ? (
          <EmptyState icon="🔒" title={t('partner.statements')} body={t('partner.statements_owner_only')} />
        ) : q.isError ? (
          <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => q.refetch()} />
        ) : (
          (q.data ?? []).map((s) => (
            <View key={s.property_id} style={{ marginTop: spacing.lg }}>
              <T variant="h2">{s.hotel}</T>
              <View style={{ marginTop: spacing.sm, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
                <Row label={t('partner.st_opening')} value={price(s.opening_santim)} />
                <Row label={t('partner.st_online_share')} value={price(s.online_hotel_share_santim)} />
                <Row label={t('partner.st_online_commission')} value={price(s.online_commission_santim)} />
                <Row label={t('partner.st_online_vat')} value={price(s.online_vat_santim)} />
                {s.pay_at_hotel_collected_santim > 0 ? <Row label={t('partner.st_pah_collected')} value={price(s.pay_at_hotel_collected_santim)} /> : null}
                {s.pay_at_hotel_commission_santim > 0 ? <Row label={t('partner.st_pah_commission')} value={price(s.pay_at_hotel_commission_santim + s.pay_at_hotel_vat_santim)} /> : null}
                <Row label={t('partner.st_paid')} value={price(s.paid_santim)} />
                {s.received_santim > 0 ? <Row label={t('partner.st_received')} value={price(s.received_santim)} /> : null}
                <View style={{ height: 1, backgroundColor: colors.line, marginVertical: spacing.xs }} />
                <Row strong label={t('partner.st_closing')} value={`${price(Math.abs(s.closing_santim))} · ${s.closing_santim >= 0 ? t('partner.st_owed_to_you') : t('partner.st_owed_to_bookdan')}`} />
              </View>
              <T variant="h3" style={{ marginTop: spacing.lg }}>{t('partner.st_stays')} ({s.lines.length})</T>
              {s.lines.length === 0 ? <T color={colors.muted} style={{ marginTop: spacing.xs }}>{t('partner.st_no_stays')}</T> : s.lines.map((l, i) => (
                <View key={i} style={{ marginTop: spacing.sm, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
                  <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
                    <T variant="h3" style={{ flex: 1 }} numberOfLines={1}>{l.guest_name}</T>
                    <T variant="h3">{price(l.net_santim)}</T>
                  </View>
                  <T variant="small" color={colors.muted}>{l.reference} · {l.date} · {l.method === 'pay_at_hotel' ? t('partner.st_method_pah') : t('partner.st_method_online')}</T>
                </View>
              ))}
              <Button variant="secondary" icon="share-outline" title={t('partner.st_share')} onPress={() => Share.share({ message: summaryText(s, t as never) })} style={{ marginTop: spacing.lg }} />
            </View>
          ))
        )}
      </ScrollView>
    </SafeAreaView>
  );
}
