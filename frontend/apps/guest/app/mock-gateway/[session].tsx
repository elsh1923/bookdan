import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTranslation } from '@bookdan/i18n';
import { Badge, Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { price } from '../../src/format';
import { useBooking } from '../../src/queries';

/**
 * Stands in for the Telebirr / Chapa payment page until real gateways exist. It is shown in place of the
 * in-app browser and returns with router.back(), like the browser closing. The buttons are demo controls
 * (force a result, replay the callback) and exist only when the app runs on the mock.
 */
export default function MockGateway() {
  const { t } = useTranslation();
  const router = useRouter();
  const { session, booking } = useLocalSearchParams<{ session: string; booking: string }>();
  const query = useBooking(booking);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function send(outcome: 'success' | 'failed' | 'duplicate') {
    if (!api.demo) return;
    setBusy(outcome);
    setError(null);
    try {
      await api.demo.completePayment(session, outcome);
      router.back(); // back to the booking screen, which polls for the result
    } catch {
      setError(t('results.error_body'));
    } finally {
      setBusy(null);
    }
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.fill }}>
      <ScrollView contentContainerStyle={{ padding: spacing.lg }}>
        <View style={{ alignItems: 'center', marginTop: spacing.xl }}>
          <View style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: colors.primarySoft, alignItems: 'center', justifyContent: 'center' }}>
            <Ionicons name="lock-closed-outline" size={28} color={colors.primaryInk} />
          </View>
          <T variant="displaySm" style={{ marginTop: spacing.md }}>{t('gateway.title')}</T>
          <View style={{ marginTop: spacing.sm }}><Badge tone="warning" icon="⚠" label={t('gateway.sim')} /></View>
        </View>

        <View style={{ marginTop: spacing.xl, padding: spacing.lg, borderRadius: radius.md, backgroundColor: colors.surface }}>
          <T variant="label" color={colors.muted}>{t('gateway.amount').toUpperCase()}</T>
          <T variant="display">{query.data ? price(query.data.total_santim, query.data.currency) : '…'}</T>
          {query.data ? <T color={colors.inkSoft}>{query.data.property_name} · {query.data.reference}</T> : null}
        </View>

        <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('gateway.demo').toUpperCase()}</T>
        {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {error}</T> : null}
        <Button title={t('gateway.approve')} icon="checkmark-circle-outline" loading={busy === 'success'} disabled={!!busy} onPress={() => send('success')} style={{ marginTop: spacing.sm }} />
        <Button variant="secondary" title={t('gateway.decline')} icon="close-circle-outline" loading={busy === 'failed'} disabled={!!busy} onPress={() => send('failed')} style={{ marginTop: spacing.sm }} />
        <Button variant="ghost" title={t('gateway.duplicate')} loading={busy === 'duplicate'} disabled={!!busy} onPress={() => send('duplicate')} style={{ marginTop: spacing.sm }} />
        <Button variant="ghost" title={t('common.cancel')} disabled={!!busy} onPress={() => router.back()} />
      </ScrollView>
    </SafeAreaView>
  );
}
