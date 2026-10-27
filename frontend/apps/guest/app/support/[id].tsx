import { Ionicons } from '@expo/vector-icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, KeyboardAvoidingView, Platform, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { useTranslation } from '@bookdan/i18n';
import { Badge, Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { useAuth } from '../../src/auth';
import { Field } from '../../src/Field';
import { statusTone } from '../../src/support';

/** One request: the conversation, a reply box while it is open, and a way to close it. */
export default function SupportThread() {
  const { t } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const token = useAuth((s) => s.token);
  const { id } = useLocalSearchParams<{ id: string }>();
  const q = useQuery({ queryKey: ['support', id], queryFn: () => api.getSupportTicket(id), enabled: !!token && !!id, refetchOnMount: 'always', refetchInterval: 30_000 });
  const [reply, setReply] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  if (!token) return <Redirect href="/signin" />;

  async function run(fn: () => Promise<void>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await qc.invalidateQueries({ queryKey: ['support'] });
    } catch (e) {
      setError(t(e instanceof ApiError && e.code === 'INVALID_STATE' ? 'support.closed_note' : e instanceof ApiError && e.code === 'NETWORK' ? 'results.offline_body' : 'results.error_body'));
    } finally {
      setBusy(false);
    }
  }

  const ticket = q.data;
  return (
    <SafeAreaView style={{ flex: 1 }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <View style={{ flexDirection: 'row', alignItems: 'center', paddingHorizontal: spacing.lg, gap: spacing.sm }}>
          <Pressable accessibilityLabel={t('common.back')} onPress={() => (router.canGoBack() ? router.back() : router.replace('/support'))} style={{ width: 44, height: 44, justifyContent: 'center' }}>
            <Ionicons name="arrow-back" size={24} color={colors.ink} />
          </Pressable>
          <T variant="h3" style={{ flex: 1 }} numberOfLines={1}>{ticket?.subject ?? t('support.title')}</T>
          {ticket ? <Badge tone={statusTone(ticket.status)} label={t(`support.status_${ticket.status}`)} /> : null}
        </View>
        {!ticket ? (
          <ActivityIndicator style={{ marginTop: spacing.xxl }} color={colors.primary} />
        ) : (
          <ScrollView contentContainerStyle={{ padding: spacing.lg, gap: spacing.md, paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
            {ticket.booking_code ? <T variant="small" color={colors.muted}>{t('support.about_booking', { code: ticket.booking_code })}</T> : null}
            {(ticket.messages ?? []).map((m, i) => (
              <View
                key={i}
                style={{
                  alignSelf: m.role === 'guest' ? 'flex-end' : 'flex-start',
                  maxWidth: '88%',
                  padding: spacing.md,
                  borderRadius: radius.md,
                  backgroundColor: m.role === 'guest' ? colors.primarySoft : colors.surface,
                  borderWidth: 1,
                  borderColor: m.role === 'guest' ? colors.primarySoft : colors.line,
                }}
              >
                <T variant="label" color={colors.muted}>{m.role === 'hotel' ? t('support.hotel') : m.role === 'support' ? t('support.support') : t('support.you')}</T>
                <T style={{ marginTop: 2 }}>{m.body}</T>
              </View>
            ))}

            {ticket.status === 'closed' ? (
              <View>
                {ticket.outcome ? <Badge tone={ticket.outcome === 'rejected' || ticket.outcome === 'withdrawn' ? 'neutral' : 'success'} label={t(`support.outcome_${ticket.outcome}`)} /> : null}
                <T color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('support.closed_note')}</T>
                <Button title={t('support.new_request')} onPress={() => router.replace('/support/new')} style={{ marginTop: spacing.md }} />
              </View>
            ) : (
              <View>
                <Field label={t('support.reply_label')} value={reply} onChangeText={setReply} maxLength={2000} multiline textAlignVertical="top" style={{ minHeight: 96, paddingTop: spacing.md }} />
                {error ? <T color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {error}</T> : null}
                <Button
                  title={t('support.reply')}
                  loading={busy}
                  disabled={reply.trim().length === 0}
                  onPress={() => run(async () => { await api.replySupportTicket(ticket.id, reply.trim()); setReply(''); })}
                  style={{ marginTop: spacing.md }}
                />
                {ticket.kind === 'dispute' ? <T variant="small" color={colors.muted} style={{ marginTop: spacing.md }}>{t('support.withdraw_note')}</T> : null}
                <Button
                  variant="ghost"
                  title={ticket.kind === 'dispute' ? t('support.withdraw') : t('support.close')}
                  onPress={() => run(() => api.closeSupportTicket(ticket.id))}
                  style={{ marginTop: spacing.xs }}
                />
              </View>
            )}
          </ScrollView>
        )}
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
