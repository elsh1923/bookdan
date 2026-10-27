import { Ionicons } from '@expo/vector-icons';
import { useQueryClient } from '@tanstack/react-query';
import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { DISPUTE_REASONS, type DisputeReason } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { useAuth } from '../../src/auth';
import { Field } from '../../src/Field';

/** Open a dispute on a booking: what went wrong and what happened. The booking is then held (no money goes to the hotel) until support decides. */
export default function RaiseDispute() {
  const { t } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const token = useAuth((s) => s.token);
  const p = useLocalSearchParams<{ booking: string; ref?: string }>();
  const [reason, setReason] = useState<DisputeReason>('not_as_described');
  const [message, setMessage] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  if (!token) return <Redirect href="/signin" />;
  if (!p.booking) return <Redirect href="/bookings" />;

  const close = () => (router.canGoBack() ? router.back() : router.replace('/bookings'));

  async function send() {
    if (message.trim().length < 10) return setError(t('support.dispute_message_required'));
    setBusy(true);
    setError(null);
    try {
      const b = await api.raiseDispute(p.booking, { reason, message: message.trim() });
      await qc.invalidateQueries({ queryKey: ['bookings'] });
      await qc.invalidateQueries({ queryKey: ['booking', p.booking] });
      await qc.invalidateQueries({ queryKey: ['support'] });
      if (b.dispute_ticket_id) router.replace({ pathname: '/support/[id]', params: { id: b.dispute_ticket_id } });
      else close();
    } catch (e) {
      setError(
        t(
          e instanceof ApiError && e.code === 'DISPUTE_WINDOW_CLOSED' ? 'support.dispute_window_closed'
            : e instanceof ApiError && e.code === 'DISPUTE_EXISTS' ? 'support.dispute_exists'
            : e instanceof ApiError && e.code === 'NETWORK' ? 'results.offline_body'
            : 'results.error_body',
        ),
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <View style={{ paddingHorizontal: spacing.lg }}>
          <Pressable accessibilityLabel={t('common.cancel')} onPress={close} style={{ width: 44, height: 44, justifyContent: 'center' }}>
            <Ionicons name="close" size={26} color={colors.ink} />
          </Pressable>
        </View>
        <ScrollView contentContainerStyle={{ padding: spacing.lg, paddingTop: 0, paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
          <T variant="display">{t('support.dispute_title')}</T>
          {p.ref ? <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('support.about_booking', { code: p.ref })}</T> : null}
          <View style={{ marginTop: spacing.md, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.primarySoft }}>
            <T variant="small" color={colors.primaryInk}>{t('support.dispute_intro')}</T>
          </View>

          <T variant="label" color={colors.muted} style={{ marginTop: spacing.lg }}>{t('support.dispute_reason').toUpperCase()}</T>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
            {DISPUTE_REASONS.map((r) => (
              <Chip key={r} label={t(`support.reason_${r}`)} selected={reason === r} onPress={() => setReason(r)} />
            ))}
          </View>

          <Field
            label={t('support.dispute_message')}
            hint={t('support.dispute_message_hint')}
            value={message}
            onChangeText={setMessage}
            maxLength={2000}
            multiline
            textAlignVertical="top"
            style={{ minHeight: 140, paddingTop: spacing.md }}
          />
          {error ? <T color={colors.danger} style={{ marginTop: spacing.md }}>✕ {error}</T> : null}
          <Button title={t('support.dispute_send')} loading={busy} onPress={send} style={{ marginTop: spacing.xl }} />
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
