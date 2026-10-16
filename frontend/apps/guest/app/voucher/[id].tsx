import { Ionicons } from '@expo/vector-icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, View, useWindowDimensions } from 'react-native';
import QRCode from 'react-native-qrcode-svg';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { useTranslation } from '@bookdan/i18n';
import { Badge, Button, EmptyState, Pop, Reveal, T, colors, radius, shadow, spacing } from '@bookdan/ui';
import { ActionTile } from '../../src/ActionTile';
import { isMockApi } from '../../src/api';
import { callPhone, openDirections } from '../../src/links';
import { price, shortDate } from '../../src/format';
import { ensureVoucher, loadVoucher } from '../../src/voucherStore';

function Line({ label, value }: { label: string; value: string }) {
  return (
    <View style={{ flexDirection: 'row', justifyContent: 'space-between', paddingVertical: spacing.sm, gap: spacing.md }}>
      <T color={colors.inkSoft}>{label}</T>
      <T style={{ flexShrink: 1, textAlign: 'right' }}>{value}</T>
    </View>
  );
}

export default function VoucherScreen() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const { width } = useWindowDimensions();
  const { id } = useLocalSearchParams<{ id: string }>();
  const [showPin, setShowPin] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<string | null>(null);

  // Reads ONLY from this phone's secure storage, so it works in airplane mode.
  const voucher = useQuery({ queryKey: ['voucher', id], queryFn: () => loadVoucher(id), staleTime: Infinity, gcTime: Infinity });

  // If the phone cannot open the dialer or a map, show the number or address instead of doing nothing.
  async function attempt(opened: Promise<boolean>, fallback: string) {
    if (!(await opened)) Alert.alert(t('common.cannot_open_title'), t('common.cannot_open_body', { value: fallback }));
  }

  const back = () => (router.canGoBack() ? router.back() : router.replace('/bookings'));
  const header = (
    <View style={{ paddingHorizontal: spacing.lg }}>
      <Pressable accessibilityLabel={t('common.back')} onPress={back} style={{ width: 44, height: 44, justifyContent: 'center' }}>
        <Ionicons name="arrow-back" size={24} color={colors.ink} />
      </Pressable>
    </View>
  );

  async function download() {
    setDownloading(true);
    setDownloadError(null);
    try {
      await ensureVoucher(id);
      await qc.invalidateQueries({ queryKey: ['voucher', id] });
    } catch (e) {
      setDownloadError(
        e instanceof ApiError && e.code === 'NETWORK' ? t('results.offline_body')
          : e instanceof ApiError && e.code === 'VOUCHER_NOT_READY' ? t('voucher.not_ready')
          : t('results.error_body'),
      );
    } finally {
      setDownloading(false);
    }
  }

  if (voucher.isPending) {
    return <SafeAreaView style={{ flex: 1 }}>{header}<View style={{ flex: 1, justifyContent: 'center' }}><ActivityIndicator size="large" color={colors.primary} /></View></SafeAreaView>;
  }

  const v = voucher.data;
  if (!v) {
    return (
      <SafeAreaView style={{ flex: 1 }}>
        {header}
        <EmptyState
          icon="🎫"
          title={t('voucher.not_downloaded_title')}
          body={downloadError ?? t('voucher.not_downloaded_body')}
          actionLabel={downloading ? t('common.loading') : t('voucher.download')}
          onAction={download}
        />
      </SafeAreaView>
    );
  }

  const months = i18n.t('months.short', { returnObjects: true }) as unknown as string[];
  const qrSize = Math.min(Math.min(width, 640) - spacing.lg * 2 - 56, 260);
  const owed = v.balance_due_santim;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }}>
      {header}
      <ScrollView contentContainerStyle={{ padding: spacing.lg, paddingTop: 0, paddingBottom: spacing.xxl }}>
        <T variant="display">{t('voucher.title')}</T>
        <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('voucher.subtitle')}</T>

        <Pop delay={120} style={{ alignItems: 'center', marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.lg, borderWidth: 1.5, borderColor: colors.primary, backgroundColor: '#fff' }}>
          <QRCode value={v.token} size={qrSize} ecl="M" quietZone={8} />
          <T variant="h2" style={{ marginTop: spacing.md, letterSpacing: 2 }}>{v.reference}</T>
        </Pop>

        <Pressable
          onPress={() => setShowPin((s) => !s)}
          accessibilityRole="button"
          accessibilityLabel={showPin ? t('voucher.hide_pin') : t('voucher.show_pin')}
          style={{ marginTop: spacing.md, minHeight: 64, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.lg, borderRadius: radius.md, backgroundColor: colors.primarySoft }}
        >
          <View>
            <T variant="label" color={colors.inkSoft}>{t('voucher.pin').toUpperCase()}</T>
            <T variant="displaySm" style={{ letterSpacing: 6 }}>{showPin ? v.pin : '••••'}</T>
          </View>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
            <T variant="small" color={colors.primaryInk}>{showPin ? t('voucher.hide_pin') : t('voucher.show_pin')}</T>
            <Ionicons name={showPin ? 'eye-off-outline' : 'eye-outline'} size={22} color={colors.primaryInk} />
          </View>
        </Pressable>
        <T variant="small" color={colors.muted} style={{ marginTop: spacing.sm }}>{t('voucher.pin_note')}</T>

        <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, marginTop: spacing.md }}>
          <Ionicons name="phone-portrait-outline" size={18} color={colors.success} />
          <T variant="small" color={colors.success} style={{ flex: 1 }}>{t('voucher.offline_saved')}</T>
        </View>
        {isMockApi ? <View style={{ marginTop: spacing.sm }}><Badge tone="warning" icon="⚠" label="SIMULATED VOUCHER · not valid at a real hotel" /></View> : null}

        <Reveal index={3} style={[{ marginTop: spacing.xl, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }, shadow.card]}>
          <T variant="displaySm">{v.property_name}</T>
          <T color={colors.inkSoft} style={{ marginBottom: spacing.sm }}>{v.address}</T>
          <Line label={t('booking.check_in')} value={shortDate(v.check_in, months)} />
          <Line label={t('booking.check_out')} value={shortDate(v.check_out, months)} />
          {v.rooms > 1 ? <Line label={t('booking.room')} value={t('common.room', { count: v.rooms })} /> : null}
          <Line label={t('booking.guests')} value={`${v.guests} · ${v.guest_name}`} />
          <View style={{ height: 1, backgroundColor: colors.line, marginVertical: spacing.sm }} />
          <Line label={t('voucher.balance_due')} value={owed > 0 ? price(owed, v.currency) : t('voucher.nothing_due')} />
        </Reveal>

        <View style={{ flexDirection: 'row', gap: spacing.md, marginTop: spacing.lg }}>
          <ActionTile icon="call-outline" label={t('booking.call_hotel')} onPress={() => attempt(callPhone(v.property_phone), v.property_phone)} />
          <ActionTile
            icon="navigate-outline"
            label={t('voucher.directions')}
            onPress={() => attempt(openDirections({ lat: v.lat, lng: v.lng, label: v.property_name }), v.address)}
          />
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}
