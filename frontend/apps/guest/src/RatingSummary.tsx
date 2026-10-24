import { View } from 'react-native';
import type { RatingSummary as Summary } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, radius, spacing } from '@bookdan/ui';
import { ratingKey } from './rating';

function Bar({ label, value, max }: { label: string; value: number; max: number }) {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, marginTop: 6 }}>
      <T variant="small" color={colors.inkSoft} style={{ width: 120 }} numberOfLines={1}>{label}</T>
      <View style={{ flex: 1, height: 8, borderRadius: 4, backgroundColor: colors.line, overflow: 'hidden' }}>
        <View style={{ width: `${Math.min(100, (value / max) * 100)}%`, height: 8, backgroundColor: colors.primary }} />
      </View>
    </View>
  );
}

/** Above the reviews: the average and its word, how many guests chose each star, and the four sub-ratings when guests gave them. */
export function RatingSummary({ summary }: { summary: Summary }) {
  const { t } = useTranslation();
  if (!summary.average || summary.count === 0) return null;
  const cats: [keyof Summary['categories'], string][] = [
    ['cleanliness', 'property.cat_cleanliness'], ['location', 'property.cat_location'], ['staff', 'property.cat_staff'], ['value', 'property.cat_value'],
  ];
  const given = cats.filter(([k]) => summary.categories[k] != null);
  return (
    <View style={{ padding: spacing.lg, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface, marginTop: spacing.md }}>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md }}>
        <View style={{ backgroundColor: colors.primary, borderRadius: radius.md, paddingHorizontal: 14, paddingVertical: 10 }}>
          <T variant="h1" color={colors.onPrimary}>{summary.average.toFixed(1)}</T>
        </View>
        <View style={{ flex: 1 }}>
          <T variant="h2" color={colors.primaryInk}>{t(`rating.${ratingKey(summary.average)}`)}</T>
          <T variant="small" color={colors.inkSoft}>{t('property.reviews_based', { count: summary.count })}</T>
        </View>
      </View>
      <View style={{ marginTop: spacing.md }}>
        {[5, 4, 3, 2, 1].map((star) => (
          <View key={star} style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, marginTop: 4 }}>
            <T variant="small" color={colors.inkSoft} style={{ width: 28 }}>{star}★</T>
            <View style={{ flex: 1, height: 8, borderRadius: 4, backgroundColor: colors.line, overflow: 'hidden' }}>
              <View style={{ width: `${(summary.distribution[star - 1] / summary.count) * 100}%`, height: 8, backgroundColor: colors.star }} />
            </View>
            <T variant="small" color={colors.inkSoft} style={{ width: 28, textAlign: 'right' }}>{summary.distribution[star - 1]}</T>
          </View>
        ))}
      </View>
      {given.length ? (
        <View style={{ marginTop: spacing.md, paddingTop: spacing.sm, borderTopWidth: 1, borderTopColor: colors.line }}>
          {given.map(([k, label]) => (
            <View key={k} style={{ flexDirection: 'row', alignItems: 'center' }}>
              <View style={{ flex: 1 }}><Bar label={t(label)} value={summary.categories[k] ?? 0} max={5} /></View>
              <T variant="small" style={{ width: 32, textAlign: 'right' }}>{(summary.categories[k] ?? 0).toFixed(1)}</T>
            </View>
          ))}
        </View>
      ) : null}
    </View>
  );
}
