import { Ionicons } from '@expo/vector-icons';
import { View } from 'react-native';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, radius, shadow, spacing } from '@bookdan/ui';
import { useAuth } from './auth';
import { useLoyalty } from './queries';

/** The guest's member level: the tier held, hotels stayed at, the discount and how far to the next tier. */
export function LoyaltyCard() {
  const { t } = useTranslation();
  const token = useAuth((s) => s.token);
  const loyalty = useLoyalty(!!token);
  if (!loyalty.data) return null;
  const { tier, hotels, discount_bps, next_tier, hotels_to_next } = loyalty.data;
  return (
    <View style={[{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.lg, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.line }, shadow.card]}>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md }}>
        <Ionicons name="ribbon-outline" size={28} color={colors.primaryInk} />
        <View style={{ flex: 1 }}>
          <T variant="label" color={colors.muted}>{t('loyalty.title').toUpperCase()}</T>
          <T variant="h2">{tier ? t(`loyalty.tier_${tier}`, { defaultValue: tier }) : t('loyalty.none_yet')}</T>
        </View>
      </View>
      <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('loyalty.hotels_count', { count: hotels })}</T>
      {tier && discount_bps > 0 ? <T variant="small" color={colors.success} style={{ marginTop: spacing.xs }}>{t('loyalty.discount', { percent: discount_bps / 100 })}</T> : null}
      {next_tier ? <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('loyalty.to_next', { count: hotels_to_next, tier: t(`loyalty.tier_${next_tier}`, { defaultValue: next_tier }) })}</T> : null}
    </View>
  );
}
