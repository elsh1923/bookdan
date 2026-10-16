import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useRef, useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, View, useWindowDimensions } from 'react-native';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';
import { nightsBetween, type Amenity, type RoomType } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Animated, Badge, Button, EmptyState, Reveal, T, colors, radius, spacing, useBounce } from '@bookdan/ui';
import { useAnimatedScrollHandler, useAnimatedStyle, useSharedValue } from 'react-native-reanimated';
import { Photo } from '../../src/Photo';
import { callPhone } from '../../src/links';
import { FOOTER_CLEARANCE, StickyFooter } from '../../src/StickyFooter';
import { price } from '../../src/format';
import { useProperty } from '../../src/queries';
import { usePrefs } from '../../src/store';

const AMENITY_ICON: Record<Amenity, keyof typeof Ionicons.glyphMap> = {
  generator: 'flash-outline',
  hot_water: 'water-outline',
  wifi: 'wifi-outline',
  parking: 'car-outline',
  pay_at_hotel: 'cash-outline',
};

function RoomCard({ room, nights, rooms, onSelect }: { room: RoomType; nights: number; rooms: number; onSelect: () => void }) {
  const { t } = useTranslation();
  const total = room.price_santim * nights;
  return (
    <View style={{ backgroundColor: colors.surface, borderRadius: radius.md, padding: spacing.md, borderWidth: 1, borderColor: colors.line, marginTop: spacing.md }}>
      <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
        <View style={{ flex: 1 }}>
          <T variant="h3">{room.name}</T>
          <T variant="small" color={colors.inkSoft}>{t('property.sleeps', { count: room.capacity })} · {room.amenities.join(' · ')}</T>
          {room.rooms_left > 0 && room.rooms_left <= 2
            ? <T variant="label" color={colors.warning} style={{ marginTop: spacing.xs }}>⚑ {t('property.rooms_left', { count: room.rooms_left })}</T>
            : null}
        </View>
        <View style={{ alignItems: 'flex-end', marginLeft: spacing.sm }}>
          <T variant="h3" color={colors.primaryInk}>{price(room.price_santim, room.currency)}</T>
          <T variant="label" color={colors.muted}>{t('common.per_night')}</T>
        </View>
      </View>
      {nights > 1 ? (
        <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>
          {price(total, room.currency)} {t('property.total_for', { nights: t('common.night', { count: nights }) })}
        </T>
      ) : null}
      <Button
        title={room.rooms_left >= rooms ? t('property.select_room') : room.rooms_left > 0 ? t('property.only_left', { count: room.rooms_left }) : t('results.sold_out')}
        disabled={room.rooms_left < rooms}
        onPress={onSelect}
        style={{ marginTop: spacing.md }}
      />
    </View>
  );
}

export default function PropertyScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { width } = useWindowDimensions();
  const p = useLocalSearchParams<{ id: string; in: string; out: string; guests?: string; rooms?: string }>();
  const query = useProperty(p.id, p.in, p.out);
  const [showVerified, setShowVerified] = useState(false);
  const scroller = useRef<ScrollView>(null);
  const [roomsY, setRoomsY] = useState(0);
  const saved = usePrefs((s) => s.saved.includes(p.id));
  const toggleSaved = usePrefs((s) => s.toggleSaved);
  const nights = nightsBetween(p.in, p.out);
  const heroHeight = Math.min(Math.round(Math.min(width, 640) * 0.95), 460);
  const heart = useBounce();
  // The photo moves at half speed while scrolling and stretches when pulled down.
  const scrollY = useSharedValue(0);
  const onScroll = useAnimatedScrollHandler((e) => { scrollY.value = e.contentOffset.y; });
  const heroStyle = useAnimatedStyle(() => {
    const y = scrollY.value;
    return {
      transform: [
        { translateY: y * 0.5 },
        { scale: y < 0 ? 1 + Math.min(-y, 200) / heroHeight : 1 },
      ],
    };
  });

  const circle = { width: 44, height: 44, borderRadius: 22, backgroundColor: 'rgba(255,255,255,0.92)', alignItems: 'center', justifyContent: 'center', zIndex: 2 } as const;
  const back = (
    <Pressable accessibilityLabel={t('common.back')} onPress={() => router.back()} style={[circle, { position: 'absolute', top: insets.top + 8, left: spacing.lg }]}>
      <Ionicons name="chevron-back" size={22} color={colors.ink} />
    </Pressable>
  );

  if (query.isPending) {
    return <View style={{ flex: 1, justifyContent: 'center' }}>{back}<ActivityIndicator color={colors.primary} size="large" /></View>;
  }
  if (query.isError) {
    return (
      <SafeAreaView style={{ flex: 1 }}>
        {back}
        <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => query.refetch()} />
      </SafeAreaView>
    );
  }

  const { property: prop, room_types, reviews } = query.data;
  const available = room_types.filter((r) => r.rooms_left > 0);
  const cheapest = available.length ? Math.min(...available.map((r) => r.price_santim)) : null;
  const cheapestRoom = available.find((r) => r.price_santim === cheapest);

  return (
    <View style={{ flex: 1, backgroundColor: colors.surface }}>
      <Animated.ScrollView ref={scroller as any} onScroll={onScroll} scrollEventThrottle={16} contentContainerStyle={{ paddingBottom: FOOTER_CLEARANCE }} showsVerticalScrollIndicator={false}>
        <View style={{ overflow: 'hidden' }}>
          <Animated.View style={heroStyle}>
            <Photo uri={prop.photos[0] ?? prop.photo_url} accent={prop.accent} name={prop.name} style={{ width: '100%', height: heroHeight }} />
          </Animated.View>
          {back}
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('wishlist.save')}
            accessibilityState={{ selected: saved }}
            onPress={() => { if (!saved) heart.bounce(); toggleSaved(prop.id); }}
            style={[circle, { position: 'absolute', top: insets.top + 8, right: spacing.lg }]}
          >
            <Animated.View style={heart.style}><Ionicons name={saved ? 'heart' : 'heart-outline'} size={22} color={saved ? colors.favorite : colors.ink} /></Animated.View>
          </Pressable>
        </View>

        {/* White sheet overlapping the photo */}
        <View style={{ marginTop: -28, backgroundColor: colors.surface, borderTopLeftRadius: 28, borderTopRightRadius: 28, paddingHorizontal: spacing.lg, paddingTop: spacing.md }}>
          <View style={{ alignSelf: 'center', width: 40, height: 4, borderRadius: 2, backgroundColor: colors.line, marginBottom: spacing.md }} />

          <T variant="display" style={{ fontSize: 28, lineHeight: 36 }}>{prop.name}</T>
          <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: spacing.sm, gap: spacing.md }}>
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: 6, flex: 1 }}>
              <Ionicons name="location-outline" size={16} color={colors.inkSoft} />
              <T variant="small" color={colors.inkSoft} numberOfLines={2} style={{ flexShrink: 1 }}>{prop.address}</T>
            </View>
            {prop.rating ? (
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
                <Ionicons name="star" size={16} color={colors.star} />
                <T variant="h3">{prop.rating.toFixed(1)}</T>
                <T variant="small" color={colors.muted}>/5</T>
              </View>
            ) : null}
          </View>
          <View style={{ flexDirection: 'row', gap: spacing.sm, alignItems: 'center', marginTop: spacing.sm }}>
            {prop.badge === 'verified' ? (
              <Pressable onPress={() => setShowVerified(!showVerified)} accessibilityRole="button">
                <Badge icon="✓" label={t('property.verified')} />
              </Pressable>
            ) : null}
            <T variant="label" color={colors.muted}>{t(`type.${prop.type}`).toUpperCase()}</T>
          </View>
          {showVerified ? <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('property.verified_info')}</T> : null}

          {/* Amenity icons, each with a caption (never icon alone) */}
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.md, marginTop: spacing.lg }}>
            {prop.amenities.map((a) => (
              <View key={a} style={{ alignItems: 'center', width: 72 }}>
                <View style={{ width: 48, height: 48, borderRadius: radius.md, backgroundColor: colors.primarySoft, alignItems: 'center', justifyContent: 'center' }}>
                  <Ionicons name={AMENITY_ICON[a]} size={22} color={colors.primaryInk} />
                </View>
                <T variant="label" color={colors.inkSoft} numberOfLines={2} style={{ textAlign: 'center', marginTop: spacing.xs }}>{t(`amenity.${a}`)}</T>
              </View>
            ))}
          </View>

          <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.about')}</T>
          <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{prop.description}</T>

          <Button
            variant="secondary"
            icon="call-outline"
            title={t('property.call')}
            onPress={async () => {
              if (!(await callPhone(prop.phone))) Alert.alert(t('common.cannot_open_title'), t('common.cannot_open_body', { value: prop.phone }));
            }}
            style={{ marginTop: spacing.lg }}
          />

          <View onLayout={(e) => setRoomsY(e.nativeEvent.layout.y)}>
            <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.rooms')}</T>
            {room_types.map((r, i) => (
              <Reveal key={r.id} index={i}>
              <RoomCard
                room={r}
                nights={nights}
                rooms={Number(p.rooms) || 1}
                onSelect={() => router.push({ pathname: '/checkout', params: { id: prop.id, room: r.id, in: p.in, out: p.out, guests: p.guests ?? '1', rooms: p.rooms ?? '1' } })}
              />
              </Reveal>
            ))}
          </View>

          <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.house_rules')}</T>
          {prop.house_rules.map((rule) => <T key={rule} color={colors.inkSoft} style={{ marginTop: spacing.xs }}>• {rule}</T>)}

          <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.reviews')}</T>
          {reviews.length === 0 ? <T color={colors.muted}>{t('results.no_reviews')}</T> : reviews.map((rv) => (
            <View key={rv.id} style={{ marginTop: spacing.md }}>
              <T variant="h3">{rv.author} · {'★'.repeat(Math.round(rv.rating))}</T>
              <T color={colors.inkSoft}>{rv.text}</T>
            </View>
          ))}
        </View>
      </Animated.ScrollView>

      <StickyFooter>
        {cheapest !== null && cheapestRoom ? (
          <Button
            title={t('property.select_from', { price: price(cheapest, cheapestRoom.currency) })}
            onPress={() => {
              // One room type: go straight to checkout. Several: show them so the guest chooses.
              if (room_types.length === 1) {
                router.push({ pathname: '/checkout', params: { id: prop.id, room: cheapestRoom.id, in: p.in, out: p.out, guests: p.guests ?? '1', rooms: p.rooms ?? '1' } });
              } else {
                scroller.current?.scrollTo({ y: Math.max(heroHeight - 28 + roomsY - 8, 0), animated: true }); // roomsY is relative to the sheet
              }
            }}
          />
        ) : (
          <Button title={t('results.sold_out')} onPress={() => undefined} disabled />
        )}
      </StickyFooter>
    </View>
  );
}
