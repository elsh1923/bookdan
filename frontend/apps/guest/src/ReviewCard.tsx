import { Ionicons } from '@expo/vector-icons';
import { useState } from 'react';
import { Pressable, View } from 'react-native';
import { ApiError } from '@bookdan/api-client';
import type { Booking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from './api';
import { Field } from './Field';

function Stars({ value, onChange }: { value: number; onChange?: (n: number) => void }) {
  return (
    <View style={{ flexDirection: 'row', gap: 4 }}>
      {[1, 2, 3, 4, 5].map((n) => (
        <Pressable
          key={n}
          disabled={!onChange}
          accessibilityRole="button"
          accessibilityLabel={`${n}`}
          accessibilityState={{ selected: n <= value }}
          hitSlop={6}
          onPress={() => onChange?.(n)}
          style={{ width: 44, height: 44, alignItems: 'center', justifyContent: 'center' }}
        >
          <Ionicons name={n <= value ? 'star' : 'star-outline'} size={32} color={n <= value ? colors.star : colors.muted} />
        </Pressable>
      ))}
    </View>
  );
}

/**
 * After the stay: stars and an optional comment, once per booking. The server decides when a review is allowed
 * (booking.can_review) and keeps the guest's review (booking.review), shown with the first name only.
 */
export function ReviewCard({ booking, onChanged }: { booking: Booking; onChanged: (b: Booking) => void }) {
  const { t } = useTranslation();
  const [rating, setRating] = useState(0);
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (booking.review) {
    return (
      <View style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
        <T variant="h3">{t('review.yours')}</T>
        <Stars value={booking.review.rating} />
        {booking.review.text ? <T color={colors.inkSoft}>{booking.review.text}</T> : null}
      </View>
    );
  }
  if (!booking.can_review) return null;

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      onChanged(await api.submitReview(booking.id, { rating, text: text.trim() }));
    } catch (e) {
      setError(
        e instanceof ApiError && e.code === 'NETWORK' ? t('results.offline_body')
          : e instanceof ApiError && e.code === 'ALREADY_REVIEWED' ? t('review.already')
          : t('review.failed'),
      );
      setBusy(false);
    }
  }

  return (
    <View style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1.5, borderColor: colors.primary, backgroundColor: colors.primarySoft }}>
      <T variant="h3" color={colors.primaryInk}>{t('review.title')}</T>
      <T variant="small" color={colors.inkSoft}>{t('review.subtitle', { hotel: booking.property_name })}</T>
      <View style={{ marginTop: spacing.sm }}><Stars value={rating} onChange={setRating} /></View>
      <Field
        label={t('review.comment')}
        value={text}
        onChangeText={setText}
        multiline
        maxLength={1000}
        placeholder={t('review.placeholder')}
        style={{ minHeight: 96, textAlignVertical: 'top', paddingTop: spacing.md }}
      />
      {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {error}</T> : null}
      <Button title={t('review.submit')} icon="send" onPress={submit} loading={busy} disabled={rating === 0} style={{ marginTop: spacing.md }} />
    </View>
  );
}
