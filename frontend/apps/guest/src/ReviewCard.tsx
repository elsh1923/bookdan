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
const ASPECTS = ['cleanliness', 'location', 'staff', 'value'] as const;
type Aspect = (typeof ASPECTS)[number];

/** One small row of five tappable stars for a sub-rating. Tapping the star you already chose clears it (the sub-ratings are optional). */
function AspectRow({ label, value, onChange }: { label: string; value: number; onChange: (n: number) => void }) {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', minHeight: 44 }}>
      <T variant="small" style={{ flex: 1 }}>{label}</T>
      <View style={{ flexDirection: 'row' }}>
        {[1, 2, 3, 4, 5].map((n) => (
          <Pressable key={n} accessibilityRole="button" accessibilityLabel={`${label} ${n}`} accessibilityState={{ selected: n <= value }} hitSlop={4} onPress={() => onChange(n === value ? 0 : n)} style={{ width: 36, height: 44, alignItems: 'center', justifyContent: 'center' }}>
            <Ionicons name={n <= value ? 'star' : 'star-outline'} size={22} color={n <= value ? colors.star : colors.muted} />
          </Pressable>
        ))}
      </View>
    </View>
  );
}

export function ReviewCard({ booking, onChanged }: { booking: Booking; onChanged: (b: Booking) => void }) {
  const { t } = useTranslation();
  const [rating, setRating] = useState(0);
  const [text, setText] = useState('');
  const [aspects, setAspects] = useState<Record<Aspect, number>>({ cleanliness: 0, location: 0, staff: 0, value: 0 });
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
      onChanged(await api.submitReview(booking.id, { rating, text: text.trim(), ...Object.fromEntries(ASPECTS.filter((a) => aspects[a] > 0).map((a) => [a, aspects[a]])) }));
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
      <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.md }}>{t('review.rate_more')}</T>
      {ASPECTS.map((a) => <AspectRow key={a} label={t(`review.cat_${a}`)} value={aspects[a]} onChange={(n) => setAspects({ ...aspects, [a]: n })} />)}
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
