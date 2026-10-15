import { Ionicons } from '@expo/vector-icons';
import { Pressable, View } from 'react-native';
import type { SearchItem } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, radius, shadow, spacing } from '@bookdan/ui';
import { Photo } from './Photo';
import { price } from './format';

export function PropertyCard({ item, onPress }: { item: SearchItem; onPress: () => void }) {
  const { t } = useTranslation();
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      style={({ pressed }) => [
        { backgroundColor: colors.surface, borderRadius: radius.lg, marginBottom: spacing.lg, overflow: 'hidden', opacity: pressed ? 0.94 : 1 },
        shadow.card,
      ]}
    >
      <View>
        <Photo uri={item.photo_url} accent={item.accent} name={item.name} style={{ width: '100%', aspectRatio: 4 / 3 }} />
        {item.rating ? (
          <View style={{ position: 'absolute', top: spacing.md, right: spacing.md, flexDirection: 'row', alignItems: 'center', gap: 4, backgroundColor: colors.charcoal, borderRadius: radius.pill, paddingHorizontal: 12, paddingVertical: 6 }}>
            <T variant="label" color={colors.star}>★</T>
            <T variant="label" color="#fff">{item.rating.toFixed(1)}</T>
          </View>
        ) : null}
        {item.badge === 'verified' ? (
          <View style={{ position: 'absolute', top: spacing.md, left: spacing.md, backgroundColor: 'rgba(255,255,255,0.94)', borderRadius: radius.pill, paddingHorizontal: 12, paddingVertical: 6 }}>
            <T variant="label" color={colors.primaryInk}>✓ {t('property.verified')}</T>
          </View>
        ) : null}
      </View>

      <View style={{ padding: spacing.lg }}>
        <T variant="h2" numberOfLines={2}>{item.name}</T>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4, marginTop: spacing.xs }}>
          <Ionicons name="location-outline" size={14} color={colors.inkSoft} />
          <T variant="small" color={colors.inkSoft} style={{ flexShrink: 1 }}>{item.area} · {item.city}</T>
        </View>
        <T variant="label" color={item.available ? colors.success : colors.danger} style={{ marginTop: spacing.sm }}>
          {item.available ? '● ' + t('results.available') : '✕ ' + t('results.sold_out')}
        </T>

        <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: spacing.md, marginTop: spacing.md }}>
          <View style={{ flexShrink: 1 }}>
            <T variant="label" color={colors.muted}>{t('common.from')}</T>
            <T variant="h2" color={colors.ink}>{price(item.from_price_santim, item.currency)}</T>
            <T variant="label" color={colors.muted}>{t('common.per_night')}</T>
          </View>
          <View style={{ minHeight: 48, paddingHorizontal: spacing.lg, borderRadius: radius.md, backgroundColor: colors.primary, alignItems: 'center', justifyContent: 'center' }}>
            <T variant="h3" color={colors.onPrimary}>{t('common.book_now')}</T>
          </View>
        </View>
      </View>
    </Pressable>
  );
}
