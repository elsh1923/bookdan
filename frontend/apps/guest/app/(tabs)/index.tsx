import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { Image } from 'expo-image';
import { useFocusEffect, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useCallback, useMemo, useState } from 'react';
import { Pressable, RefreshControl, ScrollView, View, useWindowDimensions } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import type { PropertyType, SearchItem } from '@bookdan/domain';
import { addDays, todayAddis } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, PressableScale, Reveal, Skeleton, T, colors, gradients, radius, shadow, spacing } from '@bookdan/ui';
import { useAuth } from '../../src/auth';
import { price, shortDate } from '../../src/format';
import { splitBookings } from '../../src/bookingHistory';
import { StayCard } from '../../src/StayCard';
import { StatusBadge } from '../../src/StatusBadge';
import { useAllProperties, useBookings } from '../../src/queries';
import { usePrefs } from '../../src/store';

type Category = PropertyType | 'all';
const CATEGORIES: Category[] = ['all', 'hotel', 'guesthouse', 'lodge', 'resort'];
const MAX_COLUMN = 640; // matches the centred column set in the root layout
const FEATURED_COUNT = 5;
const GRID_COUNT = 6;

// Stable pseudo-random value in [0,1) per (id, seed), so a refetch does not reshuffle the lists.
function hashUnit(id: string, seed: number): number {
  let x = Math.floor(seed * 2 ** 31) | 0;
  for (let i = 0; i < id.length; i++) x = Math.imul(x ^ id.charCodeAt(i), 2654435761);
  return ((x ^ (x >>> 15)) >>> 0) / 2 ** 32;
}

const CATEGORY_ICON: Record<Category, keyof typeof Ionicons.glyphMap> = {
  all: "apps-outline",
  hotel: "business-outline",
  guesthouse: "home-outline",
  lodge: "bonfire-outline",
  resort: "sunny-outline",
};

function Pill({ label, icon, selected, onPress }: { label: string; icon: keyof typeof Ionicons.glyphMap; selected: boolean; onPress: () => void }) {
  return (
    <PressableScale
      onPress={onPress}
      accessibilityRole="button"
      accessibilityState={{ selected }}
      scaleTo={0.94}
      style={{
        minHeight: 40, borderRadius: radius.pill, overflow: 'hidden', justifyContent: 'center',
        borderWidth: 1, borderColor: selected ? 'transparent' : colors.line,
        backgroundColor: selected ? 'transparent' : colors.surface,
      }}
    >
      {selected ? (
        <LinearGradient colors={gradients.primary} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }} />
      ) : null}
      <View style={{ flexDirection: "row", alignItems: "center", gap: 6, paddingHorizontal: spacing.md }}>
        <Ionicons name={icon} size={15} color={selected ? "#FFFFFF" : colors.inkSoft} />
        <T variant="small" color={selected ? "#FFFFFF" : colors.inkSoft}>{label}</T>
      </View>
    </PressableScale>
  );
}

// One tappable field inside the floating search card.
function SearchField({ icon, label, value, onPress, style }: {
  icon: keyof typeof Ionicons.glyphMap; label: string; value: string; onPress: () => void; style?: object;
}) {
  return (
    <Pressable onPress={onPress} accessibilityRole="button" accessibilityLabel={`${label}: ${value}`} style={[{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }, style]}>
      <Ionicons name={icon} size={18} color={colors.primaryInk} />
      <View style={{ flex: 1 }}>
        <T variant="label" color={colors.muted}>{label}</T>
        <T variant="h3" color={colors.ink} numberOfLines={1}>{value}</T>
      </View>
    </Pressable>
  );
}

function SectionTitle({ children }: { children: string }) {
  return <T variant="displaySm" color={colors.ink} style={{ paddingHorizontal: spacing.lg, marginBottom: spacing.md }}>{children}</T>;
}

export default function Home() {
  const { t } = useTranslation();
  const router = useRouter();
  const insets = useSafeAreaInsets();
  // Light status-bar icons while the dark hero band is at the top of the screen.
  const [focused, setFocused] = useState(true);
  useFocusEffect(useCallback(() => { setFocused(true); return () => setFocused(false); }, []));
  const { width } = useWindowDimensions();
  const column = Math.min(width, MAX_COLUMN);
  const wideWidth = Math.round(column * 0.82); // the next card peeks in
  const gridWidth = Math.floor((column - spacing.lg * 2 - spacing.md) / 2);

  // Home runs on tonight plus one night and one guest until the guest picks dates in the search sheet.
  const [today] = useState(() => todayAddis());
  const checkOut = addDays(today, 1);
  const [category, setCategory] = useState<Category>('all');
  // Order is random per visit and when the guest pulls to refresh.
  const [shuffleSeed, setShuffleSeed] = useState(() => Math.random());
  const all = useAllProperties({ check_in: today, check_out: checkOut, guests: 1, types: category === 'all' ? [] : [category] });

  const { featured, more } = useMemo(() => {
    const shuffled = [...all.items]
      .map((item) => ({ item, key: hashUnit(item.id, shuffleSeed) }))
      .sort((a, b) => a.key - b.key)
      .map((x) => x.item);
    return { featured: shuffled.slice(0, FEATURED_COUNT), more: shuffled.slice(FEATURED_COUNT, FEATURED_COUNT + GRID_COUNT) };
  }, [all.items, shuffleSeed]);

  // Hotels the guest looked at before, newest first, for a quick way back.
  const viewedIds = usePrefs((s) => s.viewed);
  const recent = useMemo(() => viewedIds.map((id) => all.items.find((i) => i.id === id)).filter((x): x is SearchItem => !!x).slice(0, 6), [viewedIds, all.items]);

  // The guest's own trips on the home page: the stays to come first, then the latest history.
  const token = useAuth((s) => s.token);
  const trips = useBookings(!!token);
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
  const recentTrips = useMemo(() => {
    const { upcoming, past } = splitBookings(trips.data ?? [], today);
    return [...upcoming, ...past].slice(0, 3);
  }, [trips.data, today]);

  const open = (item: SearchItem) =>
    router.push({ pathname: '/property/[id]', params: { id: item.id, in: today, out: checkOut, guests: '1' } });

  return (
    <View style={{ flex: 1, backgroundColor: colors.canvas }}>
      {focused ? <StatusBar style="light" /> : null}
      <ScrollView
        contentContainerStyle={{ paddingBottom: spacing.xxl }}
        showsVerticalScrollIndicator={false}
        refreshControl={<RefreshControl refreshing={false} tintColor={colors.primary} onRefresh={() => { setShuffleSeed(Math.random()); all.refetch(); }} />}
      >
        {/* Hero band: logo, welcome and the floating search card sit over a deep navy gradient. */}
        <View>
          <LinearGradient
            colors={gradients.header}
            start={{ x: 0, y: 0 }}
            end={{ x: 1, y: 1 }}
            style={{ paddingTop: insets.top + spacing.md, paddingBottom: 80, paddingHorizontal: spacing.lg, borderBottomLeftRadius: 30, borderBottomRightRadius: 30 }}
          >
            <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' }}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                <Image source={require('../../assets/icon.png')} accessibilityLabel="BookDan" style={{ width: 34, height: 34, borderRadius: 10 }} />
                <T variant="h2" color="#FFFFFF">Bookdan</T>
              </View>
              <Pressable
                onPress={() => router.navigate('/profile')}
                accessibilityRole="button"
                accessibilityLabel={t('tabs.profile')}
                style={{ width: 42, height: 42, borderRadius: 21, backgroundColor: 'rgba(255,255,255,0.14)', alignItems: 'center', justifyContent: 'center' }}
              >
                <Ionicons name="person-outline" size={20} color="#FFFFFF" />
              </Pressable>
            </View>

            <Reveal>
              <T variant="display" color="#FFFFFF" style={{ marginTop: spacing.xl }}>{t('home.welcome')}</T>
              <T variant="small" color="rgba(255,255,255,0.82)" style={{ marginTop: spacing.xs }}>{t('home.tagline')}</T>
            </Reveal>
          </LinearGradient>

          {/* Floating search card, overlapping the band. */}
          <Reveal index={1}>
            <PressableScale
              onPress={() => router.push('/search')}
              accessibilityRole="search"
              scaleTo={0.99}
              style={{ marginTop: -56, marginHorizontal: spacing.lg, backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.md, ...shadow.card }}
            >
              <SearchField icon="location" label={t('search.heading')} value={t('search.placeholder')} onPress={() => router.push('/search')} />
              <View style={{ height: 1, backgroundColor: colors.hairline, marginVertical: spacing.md }} />
              <View style={{ flexDirection: 'row', alignItems: 'center' }}>
                <SearchField icon="calendar-outline" label={t('search.check_in')} value={t('home.dates')} onPress={() => router.push('/search')} style={{ flex: 1 }} />
                <View style={{ width: 1, height: 36, backgroundColor: colors.hairline, marginHorizontal: spacing.sm }} />
                <SearchField icon="person-outline" label={t('search.guests')} value="1" onPress={() => router.push('/search')} style={{ flex: 1 }} />
              </View>
              <View style={{ marginTop: spacing.md, height: 48, borderRadius: radius.md, overflow: 'hidden', alignItems: 'center', justifyContent: 'center' }}>
                <LinearGradient colors={gradients.primary} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }} />
                <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                  <Ionicons name="search" size={18} color="#FFFFFF" />
                  <T variant="h3" color="#FFFFFF">{t('search.submit')}</T>
                </View>
              </View>
            </PressableScale>
          </Reveal>
        </View>

        {/* Category filter */}
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, paddingTop: spacing.lg, paddingBottom: spacing.sm, gap: spacing.sm }}>
          {CATEGORIES.map((c) => (
            <Pill key={c} icon={CATEGORY_ICON[c]} label={c === 'all' ? t('common.all') : t(`type.${c}`)} selected={category === c} onPress={() => setCategory(c)} />
          ))}
        </ScrollView>

        {/* Deals strip */}
        <View style={{ marginHorizontal: spacing.lg, marginTop: spacing.sm, marginBottom: spacing.lg, borderRadius: radius.md, overflow: 'hidden' }}>
          <LinearGradient colors={gradients.deal} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, padding: spacing.md }}>
            <View style={{ width: 44, height: 44, borderRadius: 12, backgroundColor: 'rgba(255,255,255,0.22)', alignItems: 'center', justifyContent: 'center' }}>
              <Ionicons name="pricetag" size={22} color="#FFFFFF" />
            </View>
            <View style={{ flex: 1 }}>
              <T variant="h3" color="#FFFFFF">{t('home.deal_title')}</T>
              <T variant="small" color="rgba(255,255,255,0.9)">{t('home.deal_body')}</T>
            </View>
          </LinearGradient>
        </View>

        {token && recentTrips.length > 0 ? (
          <View style={{ marginTop: spacing.xl }}>
            <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.lg, marginBottom: spacing.md }}>
              <T variant="displaySm" color={colors.ink}>{t('bookings.title')}</T>
              <Pressable accessibilityRole="button" onPress={() => router.navigate('/bookings')} hitSlop={12}>
                <T variant="small" color={colors.primaryInk}>{t('home.see_all')}</T>
              </Pressable>
            </View>
            <View style={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
              {recentTrips.map((b) => (
                <PressableScale
                  key={b.id}
                  accessibilityRole="button"
                  onPress={() => router.push({ pathname: '/booking/[id]', params: { id: b.id } })}
                  scaleTo={0.98}
                  style={[{ backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.md, borderWidth: 1, borderColor: colors.line }, shadow.card]}
                >
                  <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'flex-start', gap: spacing.sm }}>
                    <T variant="h3" style={{ flex: 1 }} numberOfLines={1}>{b.property_name}</T>
                    <StatusBadge status={b.status} />
                  </View>
                  <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>
                    {shortDate(b.check_in, months)} – {shortDate(b.check_out, months)} · {price(b.total_santim, b.currency)}
                  </T>
                </PressableScale>
              ))}
            </View>
          </View>
        ) : null}

        {all.isPending ? (
          <View style={{ flexDirection: 'row', gap: spacing.md, paddingHorizontal: spacing.lg }}>
            <Skeleton width={wideWidth} height={Math.round(wideWidth * 0.5625)} radius={radius.lg} />
            <Skeleton width={wideWidth} height={Math.round(wideWidth * 0.5625)} radius={radius.lg} />
          </View>
        ) : all.isError ? (
          <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => all.refetch()} />
        ) : featured.length === 0 ? (
          <EmptyState icon="🔎" title={t('results.empty_title')} body={t('results.empty_body')} />
        ) : (
          <>
            <SectionTitle>{t('home.featured')}</SectionTitle>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
              {featured.map((item, i) => (
                <Reveal key={item.id} index={i}><StayCard item={item} large width={wideWidth} height={Math.round(wideWidth * 0.5625)} onPress={() => open(item)} /></Reveal>
              ))}
            </ScrollView>

            {recent.length > 0 ? (
              <View style={{ marginTop: spacing.xl }}>
                <SectionTitle>{t('home.recently_viewed')}</SectionTitle>
                <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
                  {recent.map((item) => (
                    <StayCard key={item.id} item={item} width={gridWidth} height={Math.round(gridWidth * 0.95)} onPress={() => open(item)} />
                  ))}
                </ScrollView>
              </View>
            ) : null}

            {more.length > 0 ? (
              <View style={{ marginTop: spacing.xl }}>
                <SectionTitle>{t('home.more')}</SectionTitle>
                <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.md, paddingHorizontal: spacing.lg }}>
                  {more.map((item, i) => (
                    <Reveal key={item.id} index={i}><StayCard item={item} width={gridWidth} height={Math.round(gridWidth * 0.95)} onPress={() => open(item)} /></Reveal>
                  ))}
                </View>
              </View>
            ) : null}
          </>
        )}
      </ScrollView>
    </View>
  );
}
