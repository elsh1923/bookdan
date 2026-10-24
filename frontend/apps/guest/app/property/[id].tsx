import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useEffect, useRef, useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, View, useWindowDimensions } from 'react-native';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';
import { nightsBetween, type Amenity, type RoomOption, type RoomType } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Animated, Badge, Button, Chip, EmptyState, Reveal, T, colors, radius, spacing, useBounce } from '@bookdan/ui';
import { useAnimatedScrollHandler, useAnimatedStyle, useSharedValue } from 'react-native-reanimated';
import { Photo } from '../../src/Photo';
import { PhotoCarousel, PhotoGallery } from '../../src/PhotoCarousel';
import { RatingSummary } from '../../src/RatingSummary';
import { RoomOptionPicker, optionKey } from '../../src/RoomOptions';
import { callPhone, openDirections } from '../../src/links';
import { RatingBadge, distanceText } from '../../src/rating';
import { FOOTER_CLEARANCE, StickyFooter } from '../../src/StickyFooter';
import { price, shortDate } from '../../src/format';
import { useProperty } from '../../src/queries';
import { usePrefs } from '../../src/store';

const AMENITY_ICON: Record<Amenity, keyof typeof Ionicons.glyphMap> = {
  generator: 'flash-outline',
  hot_water: 'water-outline',
  wifi: 'wifi-outline',
  parking: 'car-outline',
  pay_at_hotel: 'cash-outline',
  ac: 'snow-outline',
  pool: 'sunny-outline',
  restaurant: 'restaurant-outline',
  gym: 'fitness-outline',
  spa: 'flower-outline',
  airport_shuttle: 'airplane-outline',
  laundry: 'shirt-outline',
  front_desk_24h: 'time-outline',
};
const LOW_STOCK = 3; // "Only 3 left" appears at this many rooms or fewer

type Section = 'overview' | 'amenities' | 'reviews' | 'rules';

function RoomCard({ room, nights, rooms, onSelect }: { room: RoomType; nights: number; rooms: number; onSelect: (o: RoomOption) => void }) {
  const { t } = useTranslation();
  // The hotel's extras (breakfast, non-refundable) show as choices; a room with none shows just its price.
  const options: RoomOption[] = room.options?.length ? room.options : [{ meal: 'room_only', refundable: true, price_santim: room.price_santim }];
  const [chosen, setChosen] = useState(optionKey(options[0]));
  const [gallery, setGallery] = useState(false);
  const option = options.find((o) => optionKey(o) === chosen) ?? options[0];
  const total = option.price_santim * nights;
  return (
    <View style={{ backgroundColor: colors.surface, borderRadius: radius.md, padding: spacing.md, borderWidth: 1, borderColor: colors.line, marginTop: spacing.md }}>
      <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
        {room.photo_thumb ? (
          <Pressable accessibilityRole="button" accessibilityLabel={t('property.photos', { count: room.photos.length })} onPress={() => setGallery(true)} style={{ marginRight: spacing.md }}>
            <Photo uri={room.photo_thumb} name={room.name} style={{ width: 96, height: 96, borderRadius: radius.md }} />
            <View style={{ position: 'absolute', right: 4, bottom: 4, backgroundColor: 'rgba(0,0,0,0.65)', borderRadius: 999, paddingHorizontal: 8, paddingVertical: 2 }}>
              <T variant="label" color="#fff">{room.photos.length}</T>
            </View>
          </Pressable>
        ) : null}
        <View style={{ flex: 1 }}>
          <T variant="h3">{room.name}</T>
          <T variant="small" color={colors.inkSoft}>{t('property.sleeps', { count: room.capacity })} · {room.amenities.join(' · ')}</T>
          {room.rooms_left > 0 && room.rooms_left <= LOW_STOCK
            ? <T variant="label" color={colors.danger} style={{ marginTop: spacing.xs }}>⚑ {t('property.rooms_left', { count: room.rooms_left })}</T>
            : null}
        </View>
        {options.length === 1 ? (
          <View style={{ alignItems: 'flex-end', marginLeft: spacing.sm }}>
            <T variant="h3" color={colors.primaryInk}>{price(option.price_santim, room.currency)}</T>
            <T variant="label" color={colors.muted}>{t('common.per_night')}</T>
            <T variant="label" color={colors.muted}>{t('common.taxes_extra')}</T>
          </View>
        ) : null}
      </View>
      {options.length > 1 ? (
        <>
          <RoomOptionPicker options={options} value={chosen} currency={room.currency} onChange={(o) => setChosen(optionKey(o))} />
          <T variant="label" color={colors.muted} style={{ marginTop: spacing.xs }}>{t('common.taxes_extra')}</T>
        </>
      ) : null}
      {nights > 1 ? (
        <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>
          {price(total, room.currency)} {t('property.total_for', { nights: t('common.night', { count: nights }) })}
        </T>
      ) : null}
      {room.photos.length ? <PhotoGallery photos={room.photos} visible={gallery} name={room.name} onClose={() => setGallery(false)} /> : null}
      <Button
        title={room.rooms_left >= rooms ? t('property.select_room') : room.rooms_left > 0 ? t('property.only_left', { count: room.rooms_left }) : t('results.sold_out')}
        disabled={room.rooms_left < rooms}
        onPress={() => onSelect(option)}
        style={{ marginTop: spacing.md }}
      />
    </View>
  );
}

export default function PropertyScreen() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { width } = useWindowDimensions();
  const p = useLocalSearchParams<{ id: string; in: string; out: string; guests?: string; rooms?: string }>();
  const query = useProperty(p.id, p.in, p.out);
  const [showVerified, setShowVerified] = useState(false);
  const scroller = useRef<ScrollView>(null);
  const [ys, setYs] = useState<Record<Section | 'rooms', number>>({ overview: 0, amenities: 0, reviews: 0, rules: 0, rooms: 0 });
  const [tab, setTab] = useState<Section>('overview');
  const saved = usePrefs((s) => s.saved.includes(p.id));
  const toggleSaved = usePrefs((s) => s.toggleSaved);
  const addRecentlyViewed = usePrefs((s) => s.addViewed);
  const nights = nightsBetween(p.in, p.out);
  const column = Math.min(width, 640);
  const heroHeight = Math.min(Math.round(column * 0.95), 460);
  const heart = useBounce();
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
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

  useEffect(() => { if (p.id) addRecentlyViewed(p.id); }, [p.id, addRecentlyViewed]);

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

  const { property: prop, room_types, reviews, rating_summary, nearby } = query.data;
  const available = room_types.filter((r) => r.rooms_left > 0);
  const cheapest = available.length ? Math.min(...available.map((r) => r.price_santim)) : null;
  const cheapestRoom = available.find((r) => r.price_santim === cheapest);
  const guests = Number(p.guests) || 1;
  const roomCount = Number(p.rooms) || 1;
  const sheetTop = heroHeight - 28; // the white sheet overlaps the photo by 28
  const goTo = (s: Section) => { setTab(s); scroller.current?.scrollTo({ y: Math.max(sheetTop + ys[s] - 8, 0), animated: true }); };
  const mark = (s: Section | 'rooms') => ({ onLayout: (e: { nativeEvent: { layout: { y: number } } }) => { const y = e.nativeEvent.layout.y; setYs((c) => (c[s] === y ? c : { ...c, [s]: y })); } });
  const placeName = (pl: { name: string; name_am?: string }) => (i18n.language === 'am' && pl.name_am ? pl.name_am : pl.name);
  const locationScore = rating_summary.categories.location;
  const closest = nearby[0];

  return (
    <View style={{ flex: 1, backgroundColor: colors.surface }}>
      <Animated.ScrollView ref={scroller as any} onScroll={onScroll} scrollEventThrottle={16} contentContainerStyle={{ paddingBottom: FOOTER_CLEARANCE }} showsVerticalScrollIndicator={false}>
        <View style={{ overflow: 'hidden' }}>
          <Animated.View style={heroStyle}>
            <PhotoCarousel photos={prop.photos.length ? prop.photos : prop.photo_url ? [prop.photo_url] : []} accent={prop.accent} name={prop.name} width={column} height={heroHeight} />
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

          <View {...mark('overview')}>
            <T variant="display" style={{ fontSize: 28, lineHeight: 36 }}>{prop.name}</T>
            <View style={{ flexDirection: 'row', gap: spacing.sm, alignItems: 'center', marginTop: spacing.xs }}>
              {prop.badge === 'verified' ? (
                <Pressable onPress={() => setShowVerified(!showVerified)} accessibilityRole="button">
                  <Badge icon="✓" label={t('property.verified')} />
                </Pressable>
              ) : null}
              <T variant="label" color={colors.muted}>{t(`type.${prop.type}`).toUpperCase()}</T>
            </View>
            {showVerified ? <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('property.verified_info')}</T> : null}
            <Pressable onPress={() => goTo('reviews')} accessibilityRole="button" style={{ marginTop: spacing.md }}>
              <RatingBadge rating={prop.rating} count={prop.review_count} />
            </Pressable>

            {/* Where it is, with a map link and the closest landmark */}
            <Pressable
              accessibilityRole="button"
              onPress={async () => { if (!(await openDirections({ lat: prop.lat, lng: prop.lng, label: prop.name }))) Alert.alert(t('common.cannot_open_title'), t('common.cannot_open_body', { value: prop.address })); }}
              style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, marginTop: spacing.md, minHeight: 48 }}
            >
              <View style={{ width: 40, height: 40, borderRadius: radius.md, backgroundColor: colors.primarySoft, alignItems: 'center', justifyContent: 'center' }}>
                <Ionicons name="location" size={20} color={colors.primaryInk} />
              </View>
              <View style={{ flex: 1 }}>
                <T variant="small" numberOfLines={2}>{prop.area}, {prop.city}</T>
                {closest ? <T variant="small" color={colors.inkSoft}>{distanceText(t, { name: placeName(closest), distance_m: closest.distance_m })}</T> : null}
              </View>
              <Ionicons name="chevron-forward" size={18} color={colors.primaryInk} />
            </Pressable>

            {/* Your stay, as chosen in the search */}
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
              <View style={{ flex: 1, flexDirection: 'row', alignItems: 'center', gap: 6, borderWidth: 1, borderColor: colors.primary, borderRadius: radius.md, padding: spacing.md }}>
                <Ionicons name="calendar-outline" size={18} color={colors.primaryInk} />
                <T variant="small" color={colors.primaryInk}>{shortDate(p.in, months)} – {shortDate(p.out, months)}</T>
              </View>
              <View style={{ flex: 1, flexDirection: 'row', alignItems: 'center', gap: 6, borderWidth: 1, borderColor: colors.primary, borderRadius: radius.md, padding: spacing.md }}>
                <Ionicons name="person-outline" size={18} color={colors.primaryInk} />
                <T variant="small" color={colors.primaryInk}>{t('common.guest', { count: guests })} · {t('common.room', { count: roomCount })}</T>
              </View>
            </View>
            <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>
              {t('property.check_in_from', { time: prop.check_in_time })} · {t('property.check_out_until', { time: prop.check_out_time })}
            </T>
          </View>

          {/* Jump to a part of the page */}
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: spacing.sm, paddingVertical: spacing.lg }}>
            <Chip label={t('property.tab_overview')} selected={tab === 'overview'} onPress={() => goTo('overview')} />
            <Chip label={t('property.tab_amenities')} selected={tab === 'amenities'} onPress={() => goTo('amenities')} />
            <Chip label={t('property.tab_reviews')} selected={tab === 'reviews'} onPress={() => goTo('reviews')} />
            <Chip label={t('property.tab_rules')} selected={tab === 'rules'} onPress={() => goTo('rules')} />
          </ScrollView>

          <T variant="displaySm">{t('property.about')}</T>
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

          <View {...mark('rooms')}>
            <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.rooms')}</T>
            {room_types.map((r, i) => (
              <Reveal key={r.id} index={i}>
                <RoomCard
                  room={r}
                  nights={nights}
                  rooms={roomCount}
                  onSelect={(o) => router.push({ pathname: '/checkout', params: { id: prop.id, room: r.id, in: p.in, out: p.out, guests: p.guests ?? '1', rooms: p.rooms ?? '1', meal: o.meal, refundable: String(o.refundable) } })}
                />
              </Reveal>
            ))}
          </View>

          <View {...mark('amenities')}>
            <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.tab_amenities')}</T>
            {/* Amenity icons, each with a caption (never icon alone) */}
            <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.md, marginTop: spacing.md }}>
              {prop.amenities.map((a) => (
                <View key={a} style={{ alignItems: 'center', width: 72 }}>
                  <View style={{ width: 48, height: 48, borderRadius: radius.md, backgroundColor: colors.primarySoft, alignItems: 'center', justifyContent: 'center' }}>
                    <Ionicons name={AMENITY_ICON[a]} size={22} color={colors.primaryInk} />
                  </View>
                  <T variant="label" color={colors.inkSoft} numberOfLines={2} style={{ textAlign: 'center', marginTop: spacing.xs }}>{t(`amenity.${a}`)}</T>
                </View>
              ))}
            </View>
          </View>

          <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.nearby')}</T>
          <View style={{ padding: spacing.lg, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, marginTop: spacing.md }}>
            <T variant="small" color={colors.inkSoft}>{t('property.address')}</T>
            <T>{prop.address}</T>
            {locationScore != null ? (
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 6, marginTop: spacing.sm }}>
                <Ionicons name="location-outline" size={16} color={colors.inkSoft} />
                <T variant="small" color={colors.inkSoft}>{t('property.location_rated', { score: locationScore.toFixed(1) })}</T>
              </View>
            ) : null}
            {nearby.map((pl) => (
              <View key={pl.name} style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingVertical: spacing.sm, borderTopWidth: 1, borderTopColor: colors.line, marginTop: spacing.sm }}>
                <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, flex: 1 }}>
                  <Ionicons name="flag-outline" size={18} color={colors.primaryInk} />
                  <T numberOfLines={1} style={{ flexShrink: 1 }}>{placeName(pl)}</T>
                </View>
                <T variant="small" color={colors.inkSoft}>{pl.distance_m < 1000 ? `${Math.max(10, Math.round(pl.distance_m / 10) * 10)} m` : `${(pl.distance_m / 1000).toFixed(1)} km`}</T>
              </View>
            ))}
            <Button
              variant="secondary"
              icon="map-outline"
              title={t('property.open_maps')}
              onPress={async () => { if (!(await openDirections({ lat: prop.lat, lng: prop.lng, label: prop.name }))) Alert.alert(t('common.cannot_open_title'), t('common.cannot_open_body', { value: prop.address })); }}
              style={{ marginTop: spacing.md }}
            />
          </View>

          <View {...mark('reviews')}>
            <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.reviews')}</T>
            <RatingSummary summary={rating_summary} />
            {reviews.length === 0 ? <T color={colors.muted} style={{ marginTop: spacing.sm }}>{t('results.no_reviews')}</T> : reviews.map((rv) => (
              <View key={rv.id} style={{ marginTop: spacing.lg }}>
                <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                  <T variant="h3">{rv.author}</T>
                  <T variant="small" color={colors.star}>{'★'.repeat(Math.round(rv.rating))}</T>
                </View>
                <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                  <T variant="label" color={colors.muted}>{rv.date}</T>
                  {rv.verified ? <T variant="label" color={colors.success}>✓ {t('property.verified_stay')}</T> : null}
                </View>
                {rv.text ? <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{rv.text}</T> : null}
              </View>
            ))}
          </View>

          <View {...mark('rules')}>
            <T variant="displaySm" style={{ marginTop: spacing.xl }}>{t('property.tab_rules')}</T>
            <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>• {t('property.check_in_from', { time: prop.check_in_time })}</T>
            <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>• {t('property.check_out_until', { time: prop.check_out_time })}</T>
            {prop.house_rules.map((rule) => <T key={rule} color={colors.inkSoft} style={{ marginTop: spacing.xs }}>• {rule}</T>)}
          </View>
        </View>
      </Animated.ScrollView>

      <StickyFooter>
        {cheapest !== null && cheapestRoom ? (
          <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: spacing.md }}>
            <View style={{ flexShrink: 1 }}>
              <T variant="h2">{price(cheapest, cheapestRoom.currency)}</T>
              <T variant="label" color={colors.muted}>{t('common.per_night')} · {t('common.taxes_extra')}</T>
            </View>
            <Button
              title={t('property.choose_room')}
              style={{ minWidth: 160 }}
              onPress={() => {
                // One room type: go straight to checkout. Several: show them so the guest chooses.
                if (room_types.length === 1) {
                  router.push({ pathname: '/checkout', params: { id: prop.id, room: cheapestRoom.id, in: p.in, out: p.out, guests: p.guests ?? '1', rooms: p.rooms ?? '1' } });
                } else {
                  scroller.current?.scrollTo({ y: Math.max(sheetTop + ys.rooms - 8, 0), animated: true });
                }
              }}
            />
          </View>
        ) : (
          <Button title={t('results.sold_out')} onPress={() => undefined} disabled />
        )}
      </StickyFooter>
    </View>
  );
}
