import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { Pressable, View } from 'react-native';
import type { SearchItem } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Animated, PressableScale, T, colors, dark, fonts, radius, spacing, useBounce } from '@bookdan/ui';
import { Photo } from './Photo';
import { price } from './format';
import { usePrefs } from './store';
import { useDisplay } from './display';

/** Photo-first card for the dark home screen: stars, name and price sit on a gradient over the photo. */
export function StayCard({ item, width, height, large, onPress }: {
  item: SearchItem; width: number; height: number; large?: boolean; onPress: () => void;
}) {
  useDisplay((st) => st.version); // redraw when the display currency changes
  const { t } = useTranslation();
  const saved = usePrefs((s) => s.saved.includes(item.id));
  const toggleSaved = usePrefs((s) => s.toggleSaved);
  const stars = Math.round(item.rating ?? 0);
  const heart = useBounce();

  return (
    <View style={{ width }}>
      <PressableScale
        onPress={onPress}
        accessibilityRole="button"
        scaleTo={0.97}
        style={{ borderRadius: radius.lg, overflow: 'hidden', borderWidth: 1, borderColor: dark.line, backgroundColor: dark.surface }}
      >
        <Photo uri={item.photo_url} accent={item.accent} name={item.name} showInitial={false} style={{ width: '100%', height }} />
        <LinearGradient
          pointerEvents="none"
          colors={['rgba(5,10,20,0)', 'rgba(5,10,20,0.88)']}
          style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: '65%' }}
        />
        {/* Rating badge: a small pill with a star and the score, top-left on the photo. */}
        {item.rating ? (
          <View style={{ position: 'absolute', top: spacing.sm, left: spacing.sm, flexDirection: 'row', alignItems: 'center', gap: 3, paddingHorizontal: spacing.sm, paddingVertical: 3, borderRadius: radius.pill, backgroundColor: 'rgba(11,25,44,0.72)' }}>
            <Ionicons name="star" size={12} color={colors.star} />
            <T variant="label" color="#FFFFFF">{item.rating.toFixed(1)}</T>
          </View>
        ) : null}
        <View style={{ position: 'absolute', left: spacing.md, right: spacing.md, bottom: spacing.md }}>
          <T variant={large ? 'displaySm' : 'h3'} color={dark.text} numberOfLines={2}>{item.name}</T>
          <T variant="small" color={dark.textSoft}>
            {t('common.from')} <T variant="small" color={dark.text} style={{ fontFamily: fonts.bold }}>{price(item.from_price_santim, item.currency)}</T>
          </T>
        </View>
      </PressableScale>

      {/* Sibling of the card Pressable, not a child: nested buttons are invalid on the web. */}
      <Pressable
        onPress={() => { if (!saved) heart.bounce(); toggleSaved(item.id); }}
        accessibilityRole="button"
        accessibilityLabel={t('wishlist.save')}
        accessibilityState={{ selected: saved }}
        hitSlop={8}
        style={{ position: 'absolute', top: spacing.sm, right: spacing.sm, width: 40, height: 40, alignItems: 'center', justifyContent: 'center' }}
      >
        <Animated.View style={heart.style}><Ionicons name={saved ? 'heart' : 'heart-outline'} size={24} color={saved ? colors.favorite : '#fff'} /></Animated.View>
      </Pressable>
    </View>
  );
}
