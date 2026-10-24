import { View } from 'react-native';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, radius, spacing } from '@bookdan/ui';

/** The word that goes with a rating, as on big booking sites. */
export function ratingKey(rating: number): 'excellent' | 'very_good' | 'good' | 'average' | 'fair' {
  if (rating >= 4.5) return 'excellent';
  if (rating >= 4) return 'very_good';
  if (rating >= 3.5) return 'good';
  if (rating >= 3) return 'average';
  return 'fair';
}

/** A blue score box, the word for it and how many guests rated: "4.2  Very Good (724 ratings)". */
export function RatingBadge({ rating, count, compact }: { rating: number | null; count: number; compact?: boolean }) {
  const { t } = useTranslation();
  if (!rating) {
    return <T variant="label" color={colors.primaryInk}>{t('results.no_reviews')}</T>;
  }
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, flexWrap: 'wrap' }}>
      <View style={{ backgroundColor: colors.primary, borderRadius: radius.sm, paddingHorizontal: 8, paddingVertical: 3 }}>
        <T variant="h3" color={colors.onPrimary}>{rating.toFixed(1)}</T>
      </View>
      <T variant="h3" color={colors.primaryInk}>{t(`rating.${ratingKey(rating)}`)}</T>
      {compact ? null : <T variant="small" color={colors.inkSoft}>({t('results.ratings', { count })})</T>}
    </View>
  );
}

/** "1.4 km to Bole Airport" (or metres when it is very close). */
export function distanceText(t: (k: string, o?: Record<string, unknown>) => string, place: { name: string; distance_m: number }): string {
  return place.distance_m < 1000
    ? t('results.m_to', { m: Math.max(10, Math.round(place.distance_m / 10) * 10), place: place.name })
    : t('results.km_to', { km: (place.distance_m / 1000).toFixed(1), place: place.name });
}
