import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { ActivityIndicator, TextInput, View } from 'react-native';
import { ApiError } from '@bookdan/api-client';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, fonts, radius, spacing } from '@bookdan/ui';
import { api } from './api';

/**
 * A guest's dispute about this booking, as the hotel sees it: the reason, what the guest said, and a box for the hotel's own account.
 * The payment for the stay is held back until BookDan support decides. Support's own words are never shown here.
 */
export function DisputePanel({ bookingId }: { bookingId: string }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ['dispute', bookingId], queryFn: () => api.getDispute(bookingId), retry: false });
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  if (q.isPending) return <ActivityIndicator color={colors.primary} style={{ marginTop: spacing.lg }} />;
  if (!q.data) return null; // no dispute for this booking

  async function send() {
    if (text.trim().length < 10) return setError(t('partner.dispute_statement_hint'));
    setBusy(true);
    setError(null);
    try {
      await api.sendDisputeStatement(bookingId, text.trim());
      setText('');
      setSent(true);
      await qc.invalidateQueries({ queryKey: ['dispute', bookingId] });
    } catch (e) {
      setError(t(e instanceof ApiError && e.code === 'NETWORK' ? 'results.offline_body' : 'results.error_body'));
    } finally {
      setBusy(false);
    }
  }

  const d = q.data;
  return (
    <View style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1.5, borderColor: colors.warning, backgroundColor: colors.warningSoft }}>
      <T variant="h3">{t('partner.dispute_title')}</T>
      <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.dispute_reason', { reason: d.reason })}</T>
      <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.dispute_held')}</T>
      {d.messages.map((m, i) => (
        <View key={i} style={{ marginTop: spacing.md, padding: spacing.md, borderRadius: radius.sm, backgroundColor: colors.surface }}>
          <T variant="label" color={colors.muted}>{m.role === 'hotel' ? t('partner.dispute_hotel') : t('partner.dispute_guest')}</T>
          <T style={{ marginTop: 2 }}>{m.body}</T>
        </View>
      ))}
      {d.status !== 'closed' ? (
        <View style={{ marginTop: spacing.md }}>
          <T variant="label" color={colors.muted}>{t('partner.dispute_statement').toUpperCase()}</T>
          <TextInput
            accessibilityLabel={t('partner.dispute_statement')}
            placeholder={t('partner.dispute_statement_hint')}
            placeholderTextColor={colors.muted}
            value={text}
            onChangeText={(v) => { setText(v); setSent(false); }}
            multiline
            maxLength={2000}
            textAlignVertical="top"
            style={{ minHeight: 100, marginTop: spacing.xs, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface, fontFamily: fonts.regular, fontSize: 16, color: colors.ink }}
          />
          {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.xs }}>✕ {error}</T> : null}
          {sent ? <T variant="small" color={colors.success} style={{ marginTop: spacing.xs }}>{t('partner.dispute_sent')}</T> : null}
          <Button title={t('partner.dispute_send')} onPress={send} loading={busy} style={{ marginTop: spacing.md }} />
        </View>
      ) : null}
    </View>
  );
}
