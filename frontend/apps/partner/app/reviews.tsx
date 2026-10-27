import { Ionicons } from '@expo/vector-icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import type { PartnerReview } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, EmptyState, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { Field } from '../src/Field';

/** The latest reviews of the owner's hotels, with a box to answer each in public. Owners only: the server refuses anyone else. */
export default function Reviews() {
  const { t } = useTranslation();
  const router = useRouter();
  const q = useQuery({ queryKey: ['reviews'], queryFn: () => api.listReviews(), retry: false });
  const ownerOnly = q.error instanceof ApiError && q.error.code === 'FORBIDDEN';

  return (
    <SafeAreaView style={{ flex: 1 }}>
      <View style={{ flexDirection: 'row', alignItems: 'center', paddingHorizontal: spacing.lg, gap: spacing.sm }}>
        <Pressable accessibilityLabel={t('common.back')} onPress={() => (router.canGoBack() ? router.back() : router.replace('/settings'))} style={{ width: 44, height: 44, justifyContent: 'center' }}>
          <Ionicons name="arrow-back" size={24} color={colors.ink} />
        </Pressable>
        <T variant="h2" style={{ flex: 1 }}>{t('partner.reviews')}</T>
      </View>
      {q.isPending ? (
        <ActivityIndicator style={{ marginTop: spacing.xxl }} color={colors.primary} />
      ) : ownerOnly ? (
        <EmptyState icon="🔒" title={t('partner.reviews')} body={t('partner.reviews_owner_only')} />
      ) : q.isError ? (
        <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => q.refetch()} />
      ) : (q.data ?? []).length === 0 ? (
        <EmptyState icon="⭐" title={t('partner.reviews')} body={t('partner.reviews_empty')} />
      ) : (
        <ScrollView contentContainerStyle={{ padding: spacing.lg, gap: spacing.md }} keyboardShouldPersistTaps="handled">
          {(q.data ?? []).map((r) => <ReviewRow key={r.id} review={r} />)}
        </ScrollView>
      )}
    </SafeAreaView>
  );
}

function ReviewRow({ review }: { review: PartnerReview }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [editing, setEditing] = useState(!review.reply);
  const [text, setText] = useState(review.reply?.text ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function run(fn: () => Promise<void>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await qc.invalidateQueries({ queryKey: ['reviews'] });
      setEditing(false);
    } catch (e) {
      setError(t(e instanceof ApiError && e.code === 'NETWORK' ? 'results.offline_body' : 'results.error_body'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <View style={{ padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
      <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
        <T variant="h3">{review.author} · <T variant="small" color={colors.star}>{'★'.repeat(review.rating)}</T></T>
        <T variant="label" color={colors.muted}>{review.date}</T>
      </View>
      <T variant="label" color={colors.muted}>{review.hotel} · {review.booking_code}</T>
      {review.text ? <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{review.text}</T> : null}
      {review.reply && !editing ? (
        <View style={{ marginTop: spacing.md, paddingLeft: spacing.md, borderLeftWidth: 3, borderLeftColor: colors.line }}>
          <T variant="label" color={colors.muted}>{t('partner.reply_yours')} · {review.reply.date}</T>
          <T color={colors.inkSoft} style={{ marginTop: 2 }}>{review.reply.text}</T>
          <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
            <Button variant="secondary" title={t('partner.reply_edit')} onPress={() => setEditing(true)} style={{ flex: 1 }} />
            <Button variant="ghost" title={t('partner.reply_remove')} onPress={() => run(() => api.removeReviewReply(review.id))} loading={busy} style={{ flex: 1 }} />
          </View>
        </View>
      ) : (
        <View>
          <Field label={t('partner.reply_write')} hint={t('partner.reply_hint')} value={text} onChangeText={setText} maxLength={1000} multiline textAlignVertical="top" style={{ minHeight: 90, paddingTop: spacing.md }} />
          {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.xs }}>✕ {error}</T> : null}
          <Button title={t('partner.reply_send')} onPress={() => run(() => api.replyToReview(review.id, text.trim()))} disabled={text.trim().length < 3} loading={busy} style={{ marginTop: spacing.md }} />
        </View>
      )}
    </View>
  );
}
