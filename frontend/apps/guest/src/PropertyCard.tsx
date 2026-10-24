import { Ionicons } from '@expo/vector-icons';
import { Pressable, View } from 'react-native';
import type { SearchItem } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Animated, PressableScale, T, colors, radius, shadow, spacing, useBounce } from '@bookdan/ui';
import { Photo } from './Photo';
import { price } from './format';
import { RatingBadge, distanceText } from './rating';
import { usePrefs } from './store';

const LOW_STOCK = 3; // "Only 3 left" appears at this many rooms or fewer

/** One hotel in the results: photo, score, name, how far from a landmark, rooms left, and the price per night. */
export function PropertyCard({ item, onPress }: { item: SearchItem; onPress: () => void }) {
  const { t, i18n } = useTranslation();
  const saved = usePrefs((s) => s.saved.includes(item.id));
  const toggleSaved = usePrefs((s) => s.toggleSaved);
  const heart = useBounce();
  const place = item.landmark ? { name: i18n.language === 'am' && item.landmark.name_am ? item.landmark.name_am : item.landmark.name, distance_m: item.landmark.distance_m } : null;
  const low = item.available && item.rooms_left > 0 && item.rooms_left <= LOW_STOCK;

  return (
    <View style={[{ backgroundColor: colors.surface, borderRadius: radius.lg, marginBottom: spacing.lg, overflow: 'hidden' }, shadow.card]}>
      <PressableScale onPress={onPress} accessibilityRole="button" accessibilityLabel={item.name} scaleTo={0.99}>
        <View>
          <Photo uri={item.photo_url} accent={item.accent} name={item.name} style={{ width: '100%', aspectRatio: 4 / 3 }} />
          {item.badge === 'verified' ? (
            <View style={{ position: 'absolute', top: spacing.md, left: spacing.md, backgroundColor: 'rgba(255,255,255,0.94)', borderRadius: radius.pill, paddingHorizontal: 12, paddingVertical: 6 }}>
              <T variant="label" color={colors.primaryInk}>✓ {t('property.verified')}</T>
            </View>
          ) : null}
        </View>

        <View style={{ padding: spacing.lg }}>
          <RatingBadge rating={item.rating} count={item.review_count} />
          <T variant="h2" numberOfLines={2} style={{ marginTop: spacing.sm }}>{item.name}</T>
          <T variant="label" color={colors.muted}>{t(`type.${item.type}`).toUpperCase()}</T>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4, marginTop: spacing.xs }}>
            <Ionicons name="location-outline" size={14} color={colors.inkSoft} />
            <T variant="small" color={colors.inkSoft} style={{ flexShrink: 1 }}>{item.area} · {item.city}</T>
          </View>
          {item.distance_m != null ? (
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4, marginTop: 2 }}>
              <Ionicons name="walk-outline" size={14} color={colors.primaryInk} />
              <T variant="small" color={colors.primaryInk} style={{ flexShrink: 1 }}>{t('results.from_you', { distance: item.distance_m < 1000 ? `${Math.max(10, Math.round(item.distance_m / 10) * 10)} m` : `${(item.distance_m / 1000).toFixed(1)} km` })}</T>
            </View>
          ) : null}
          {place ? (
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4, marginTop: 2 }}>
              <Ionicons name="navigate-outline" size={14} color={colors.inkSoft} />
              <T variant="small" color={colors.inkSoft} style={{ flexShrink: 1 }}>{distanceText(t, place)}</T>
            </View>
          ) : null}

          <View style={{ flexDirection: 'row', alignItems: 'flex-end', justifyContent: 'space-between', gap: spacing.md, marginTop: spacing.md }}>
            <View style={{ flexShrink: 1 }}>
              {item.available ? (
                low ? <T variant="label" color={colors.danger}>⚑ {t('results.rooms_left', { count: item.rooms_left })}</T> : null
              ) : (
                <T variant="label" color={colors.danger}>✕ {t('results.sold_out')}</T>
              )}
            </View>
            <View style={{ alignItems: 'flex-end' }}>
              <T variant="h1" color={item.available ? colors.ink : colors.muted}>{price(item.from_price_santim, item.currency)}</T>
              <T variant="label" color={colors.muted}>{t('common.per_night')}</T>
              <T variant="label" color={colors.muted}>{t('common.taxes_extra')}</T>
            </View>
          </View>
        </View>
      </PressableScale>

      {/* A sibling of the card button, not a child: nested buttons are invalid on the web. */}
      <Pressable
        onPress={() => { if (!saved) heart.bounce(); toggleSaved(item.id); }}
        accessibilityRole="button"
        accessibilityLabel={t('wishlist.save')}
        accessibilityState={{ selected: saved }}
        hitSlop={8}
        style={{ position: 'absolute', top: spacing.sm, right: spacing.sm, width: 44, height: 44, borderRadius: 22, backgroundColor: 'rgba(255,255,255,0.92)', alignItems: 'center', justifyContent: 'center' }}
      >
        <Animated.View style={heart.style}><Ionicons name={saved ? 'heart' : 'heart-outline'} size={22} color={saved ? colors.favorite : colors.ink} /></Animated.View>
      </Pressable>
    </View>
  );
}
