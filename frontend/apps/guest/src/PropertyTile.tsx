import { Ionicons } from '@expo/vector-icons';
import { Pressable, View } from 'react-native';
import type { SearchItem } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Animated, PressableScale, T, colors, radius, spacing, useBounce } from '@bookdan/ui';
import { Photo } from './Photo';
import { price } from './format';
import { usePrefs } from './store';
import { useDisplay } from './display';

export const GUEST_FAVORITE_MIN_RATING = 4.6;

/** Square photo tile for carousels and grids: heart on the photo, details underneath. */
export function PropertyTile({ item, width, onPress }: { item: SearchItem; width: number; onPress: () => void }) {
  useDisplay((st) => st.version); // redraw when the display currency changes
  const { t } = useTranslation();
  const saved = usePrefs((s) => s.saved.includes(item.id));
  const toggleSaved = usePrefs((s) => s.toggleSaved);
  const heart = useBounce();

  return (
    <View style={{ width }}>
      <PressableScale onPress={onPress} accessibilityRole="button" scaleTo={0.97}>
        <Photo
          uri={item.photo_url}
          accent={item.accent}
          name={item.name}
          style={{ width: '100%', aspectRatio: 1, borderRadius: radius.lg }}
        />
        <View style={{ paddingTop: spacing.sm }}>
          <T variant="h3" numberOfLines={1}>{item.name}</T>
          <T variant="small" color={colors.inkSoft} numberOfLines={1}>{item.area} · {item.city}</T>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
            <T variant="small" color={colors.inkSoft} numberOfLines={1} style={{ flexShrink: 1 }}>
              {t('home.price_night', { price: price(item.from_price_santim, item.currency) })}
            </T>
            {item.rating ? (
              <>
                <T variant="small" color={colors.inkSoft}>·</T>
                <Ionicons name="star" size={12} color={colors.star} />
                <T variant="small" color={colors.inkSoft}>{item.rating.toFixed(1)}</T>
              </>
            ) : null}
          </View>
        </View>
      </PressableScale>

      {item.rating && item.rating >= GUEST_FAVORITE_MIN_RATING ? (
        <View pointerEvents="none" style={{ position: 'absolute', top: spacing.sm + 4, left: spacing.sm + 4, backgroundColor: '#fff', borderRadius: radius.pill, paddingHorizontal: 10, paddingVertical: 4 }}>
          <T variant="label" color={colors.ink}>{t('home.guest_favorite')}</T>
        </View>
      ) : null}

      {/* Sibling of the card Pressable, not a child: nested buttons are invalid on the web. */}
      <Pressable
        onPress={() => { if (!saved) heart.bounce(); toggleSaved(item.id); }}
        accessibilityRole="button"
        accessibilityLabel={t('wishlist.save')}
        accessibilityState={{ selected: saved }}
        hitSlop={8}
        style={{ position: 'absolute', top: spacing.sm, right: spacing.sm, width: 40, height: 40, alignItems: 'center', justifyContent: 'center' }}
      >
        <Animated.View style={heart.style}><Ionicons name={saved ? 'heart' : 'heart-outline'} size={26} color={saved ? colors.favorite : '#fff'} /></Animated.View>
      </Pressable>
    </View>
  );
}
