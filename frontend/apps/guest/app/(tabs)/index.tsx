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
import { Chip, EmptyState, PressableScale, Reveal, Skeleton, T, colors, gradients, radius, shadow, spacing } from '@bookdan/ui';
import { CurrencyChip } from '../../src/CurrencyChip';
import { StayCard } from '../../src/StayCard';
import { useAllProperties } from '../../src/queries';
import { usePrefs } from '../../src/store';
import { useDisplay } from '../../src/display';

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
  all: 'apps-outline',
  hotel: 'business-outline',
  guesthouse: 'home-outline',
  lodge: 'bonfire-outline',
  resort: 'sunny-outline',
};

function Pill(props: { label: string; icon: keyof typeof Ionicons.glyphMap; selected: boolean; onPress: () => void }) {
  return <Chip {...props} />;
}

// One tappable field inside the search card.
function SearchField({ icon, label, value, onPress, style }: {
  icon: keyof typeof Ionicons.glyphMap; label: string; value: string; onPress: () => void; style?: object;
}) {
  return (
    <Pressable onPress={onPress} accessibilityRole="button" accessibilityLabel={`${label}: ${value}`} style={[{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }, style]}>
      <View style={{ width: 34, height: 34, borderRadius: 17, backgroundColor: colors.primarySoft, alignItems: 'center', justifyContent: 'center' }}>
        <Ionicons name={icon} size={17} color={colors.primaryInk} />
      </View>
      <View style={{ flex: 1 }}>
        <T variant="label" color={colors.muted}>{label}</T>
        <T variant="h3" color={colors.ink} numberOfLines={1}>{value}</T>
      </View>
    </Pressable>
  );
}

/** A quick-action tile: an icon in a soft circle, a short label. */
function QuickTile({ icon, label, onPress }: { icon: keyof typeof Ionicons.glyphMap; label: string; onPress: () => void }) {
  return (
    <PressableScale onPress={onPress} accessibilityRole="button" accessibilityLabel={label} scaleTo={0.97} style={[{ flex: 1, alignItems: 'center', gap: spacing.sm, paddingVertical: spacing.md, borderRadius: radius.lg, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.line }, shadow.card]}>
      <View style={{ width: 44, height: 44, borderRadius: 22, backgroundColor: colors.primarySoft, alignItems: 'center', justifyContent: 'center' }}>
        <Ionicons name={icon} size={22} color={colors.primaryInk} />
      </View>
      <T variant="small" color={colors.ink} style={{ textAlign: 'center' }}>{label}</T>
    </PressableScale>
  );
}

/** A promo card with a soft gradient, a title, one line of text and a call to action. */
function OfferCard({ icon, title, body, cta, gradient, width, onPress }: {
  icon: keyof typeof Ionicons.glyphMap; title: string; body: string; cta: string; gradient: readonly [string, string, ...string[]]; width: number; onPress: () => void;
}) {
  return (
    <PressableScale onPress={onPress} accessibilityRole="button" scaleTo={0.98} style={{ width, borderRadius: radius.lg, overflow: 'hidden' }}>
      <LinearGradient colors={gradient} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ padding: spacing.lg, minHeight: 150, justifyContent: 'space-between' }}>
        <View style={{ width: 40, height: 40, borderRadius: 12, backgroundColor: 'rgba(255,255,255,0.18)', alignItems: 'center', justifyContent: 'center' }}>
          <Ionicons name={icon} size={20} color="#FFFFFF" />
        </View>
        <View style={{ marginTop: spacing.md }}>
          <T variant="h3" color="#FFFFFF">{title}</T>
          <T variant="small" color="rgba(255,255,255,0.88)" style={{ marginTop: 2 }}>{body}</T>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4, marginTop: spacing.sm }}>
            <T variant="small" color="#FFFFFF" style={{ fontWeight: '700' }}>{cta}</T>
            <Ionicons name="arrow-forward" size={14} color="#FFFFFF" />
          </View>
        </View>
      </LinearGradient>
    </PressableScale>
  );
}

/** A small reason-to-book card: an icon, a title and one line. */
function WhyCard({ icon, title, body, width }: { icon: keyof typeof Ionicons.glyphMap; title: string; body: string; width: number }) {
  return (
    <View style={[{ width, padding: spacing.md, borderRadius: radius.lg, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.line, gap: spacing.xs }, shadow.card]}>
      <Ionicons name={icon} size={22} color={colors.primaryInk} />
      <T variant="h3">{title}</T>
      <T variant="small" color={colors.inkSoft}>{body}</T>
    </View>
  );
}

function SectionHeader({ title, actionLabel, onAction }: { title: string; actionLabel?: string; onAction?: () => void }) {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.lg, marginBottom: spacing.md }}>
      <T variant="displaySm" color={colors.ink}>{title}</T>
      {actionLabel && onAction ? (
        <Pressable accessibilityRole="button" onPress={onAction} hitSlop={12}>
          <T variant="small" color={colors.primaryInk} style={{ fontWeight: '700' }}>{actionLabel}</T>
        </Pressable>
      ) : null}
    </View>
  );
}

export default function Home() {
  useDisplay((st) => st.version); // redraw when the display currency changes
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
  const offerWidth = Math.round(column * 0.72);

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
        {/* Hero: logo, a short welcome, and the search card floating over the navy band. */}
        <View>
          <LinearGradient
            colors={gradients.header}
            start={{ x: 0, y: 0 }}
            end={{ x: 1, y: 1 }}
            style={{ paddingTop: insets.top + spacing.md, paddingBottom: 88, paddingHorizontal: spacing.lg, borderBottomLeftRadius: 32, borderBottomRightRadius: 32 }}
          >
            <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' }}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                <Image source={require('../../assets/icon.png')} accessibilityLabel="BookDan" style={{ width: 36, height: 36, borderRadius: 11 }} />
                <T variant="h2" color="#FFFFFF">Bookdan</T>
              </View>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                <CurrencyChip onDark />
                <Pressable
                  onPress={() => router.navigate('/profile')}
                  accessibilityRole="button"
                  accessibilityLabel={t('tabs.profile')}
                  style={{ width: 42, height: 42, borderRadius: 21, backgroundColor: 'rgba(255,255,255,0.14)', alignItems: 'center', justifyContent: 'center' }}
                >
                  <Ionicons name="person-outline" size={20} color="#FFFFFF" />
                </Pressable>
              </View>
            </View>

            <Reveal>
              <T variant="display" color="#FFFFFF" style={{ marginTop: spacing.xl }}>{t('home.welcome')}</T>
              <T variant="small" color="rgba(255,255,255,0.82)" style={{ marginTop: spacing.xs }}>{t('home.tagline')}</T>
            </Reveal>
          </LinearGradient>

          {/* The search card: one container with the three fields and a full-width search button. */}
          <Reveal index={1}>
            <PressableScale
              onPress={() => router.push('/search')}
              accessibilityRole="search"
              scaleTo={0.99}
              style={[{ marginTop: -60, marginHorizontal: spacing.lg, backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.lg, borderWidth: 1, borderColor: colors.line }, shadow.card]}
            >
              <SearchField icon="location-outline" label={t('search.heading')} value={t('search.placeholder')} onPress={() => router.push('/search')} />
              <View style={{ height: 1, backgroundColor: colors.hairline, marginVertical: spacing.md }} />
              <View style={{ flexDirection: 'row', alignItems: 'center' }}>
                <SearchField icon="calendar-outline" label={t('search.check_in')} value={t('home.dates')} onPress={() => router.push('/search')} style={{ flex: 1 }} />
                <View style={{ width: 1, height: 40, backgroundColor: colors.hairline, marginHorizontal: spacing.sm }} />
                <SearchField icon="person-outline" label={t('search.guests')} value="1" onPress={() => router.push('/search')} style={{ flex: 1 }} />
              </View>
              <View style={{ marginTop: spacing.lg }}>
                <Pressable
                  accessibilityRole="button"
                  onPress={() => router.push('/search')}
                  style={{ height: 52, borderRadius: 14, overflow: 'hidden', alignItems: 'center', justifyContent: 'center' }}
                >
                  <LinearGradient colors={gradients.primary} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }} />
                  <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                    <Ionicons name="search" size={18} color="#FFFFFF" />
                    <T variant="h3" color="#FFFFFF">{t('search.submit')}</T>
                  </View>
                </Pressable>
              </View>
            </PressableScale>
          </Reveal>
        </View>

        {/* Quick actions: the three things most guests come here for. */}
        <View style={{ flexDirection: 'row', gap: spacing.md, paddingHorizontal: spacing.lg, marginTop: spacing.xl }}>
          <QuickTile icon="search-outline" label={t('search.submit')} onPress={() => router.push('/search')} />
          <QuickTile icon="navigate-outline" label={t('search.use_gps')} onPress={() => router.push('/search')} />
          <QuickTile icon="receipt-outline" label={t('bookings.title')} onPress={() => router.navigate('/bookings')} />
        </View>

        {/* Category pills with icons. */}
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, paddingTop: spacing.xl, paddingBottom: spacing.xs, gap: spacing.sm }}>
          {CATEGORIES.map((c) => (
            <Pill key={c} icon={CATEGORY_ICON[c]} label={c === 'all' ? t('common.all') : t(`type.${c}`)} selected={category === c} onPress={() => setCategory(c)} />
          ))}
        </ScrollView>

        {/* Offers: soft gradients, one for paying at the hotel and one for members. */}
        <View style={{ marginTop: spacing.xl }}>
          <SectionHeader title={t('home.offers')} />
          <ScrollView horizontal showsHorizontalScrollIndicator={false} snapToInterval={offerWidth + spacing.md} decelerationRate="fast" contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
            <OfferCard icon="pricetag-outline" title={t('home.deal_title')} body={t('home.deal_body')} cta={t('home.see_all')} gradient={['#EE8C74', '#E05A47']} width={offerWidth} onPress={() => router.push('/search')} />
            <OfferCard icon="ribbon-outline" title={t('home.member_title')} body={t('home.member_body')} cta={t('home.see_all')} gradient={['#15243D', '#24304A']} width={offerWidth} onPress={() => router.navigate('/profile')} />
          </ScrollView>
        </View>

        {all.isPending ? (
          <View style={{ flexDirection: 'row', gap: spacing.md, paddingHorizontal: spacing.lg, marginTop: spacing.xl }}>
            <Skeleton width={wideWidth} height={Math.round(wideWidth * 0.5625)} radius={radius.lg} />
            <Skeleton width={wideWidth} height={Math.round(wideWidth * 0.5625)} radius={radius.lg} />
          </View>
        ) : all.isError ? (
          <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => all.refetch()} />
        ) : featured.length === 0 ? (
          <EmptyState icon="🔎" title={t('results.empty_title')} body={t('results.empty_body')} />
        ) : (
          <>
            <View style={{ marginTop: spacing.xl }}>
              <SectionHeader title={t('home.featured')} actionLabel={t('home.see_all')} onAction={() => router.push('/search')} />
              <ScrollView horizontal showsHorizontalScrollIndicator={false} snapToInterval={wideWidth + spacing.md} decelerationRate="fast" contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
                {featured.map((item, i) => (
                  <Reveal key={item.id} index={i}><StayCard item={item} large width={wideWidth} height={Math.round(wideWidth * 0.5625)} onPress={() => open(item)} /></Reveal>
                ))}
              </ScrollView>
            </View>

            {/* Why BookDan: three short reasons to book here. */}
            <View style={{ marginTop: spacing.xl }}>
              <SectionHeader title={t('home.why_title')} />
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
                <WhyCard icon="shield-checkmark-outline" title={t('home.why_secure')} body={t('home.why_secure_body')} width={Math.round(column * 0.62)} />
                <WhyCard icon="flash-outline" title={t('home.why_fast')} body={t('home.why_fast_body')} width={Math.round(column * 0.62)} />
                <WhyCard icon="card-outline" title={t('home.why_pay')} body={t('home.why_pay_body')} width={Math.round(column * 0.62)} />
              </ScrollView>
            </View>

            {recent.length > 0 ? (
              <View style={{ marginTop: spacing.xl }}>
                <SectionHeader title={t('home.recently_viewed')} />
                <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
                  {recent.map((item) => (
                    <StayCard key={item.id} item={item} width={gridWidth} height={Math.round(gridWidth * 0.95)} onPress={() => open(item)} />
                  ))}
                </ScrollView>
              </View>
            ) : null}

            {more.length > 0 ? (
              <View style={{ marginTop: spacing.xl }}>
                <SectionHeader title={t('home.more')} />
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
