import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { Image } from 'expo-image';
import { useFocusEffect, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useCallback, useMemo, useState } from 'react';
import { ActivityIndicator, Pressable, RefreshControl, ScrollView, View, useWindowDimensions } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import type { PropertyType, SearchItem } from '@bookdan/domain';
import { addDays, todayAddis } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, PressableScale, Reveal, Skeleton, T, dark, gradients, radius, spacing } from '@bookdan/ui';
import { StayCard } from '../../src/StayCard';
import { useAllProperties } from '../../src/queries';
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

function Pill({ label, selected, onPress }: { label: string; selected: boolean; onPress: () => void }) {
  return (
    <PressableScale
      onPress={onPress}
      accessibilityRole="button"
      accessibilityState={{ selected }}
      scaleTo={0.94}
      style={{ minHeight: 44, borderRadius: radius.pill, overflow: 'hidden', borderWidth: 1, borderColor: selected ? 'transparent' : dark.line, justifyContent: 'center' }}
    >
      {selected ? (
        <LinearGradient colors={gradients.primary} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }} />
      ) : null}
      <T variant="small" color={selected ? '#FFFFFF' : dark.text} style={{ paddingHorizontal: spacing.md }}>{label}</T>
    </PressableScale>
  );
}

export default function Home() {
  const { t } = useTranslation();
  const router = useRouter();
  // Light status-bar icons only while this dark screen is showing.
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

  const open = (item: SearchItem) =>
    router.push({ pathname: '/property/[id]', params: { id: item.id, in: today, out: checkOut, guests: '1' } });

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: dark.bg }} edges={['top']}>
      {focused ? <StatusBar style="light" /> : null}
      <ScrollView
        contentContainerStyle={{ paddingBottom: spacing.xxl }}
        refreshControl={
          <RefreshControl
            refreshing={false}
            tintColor={dark.accent}
            onRefresh={() => { setShuffleSeed(Math.random()); all.refetch(); }}
          />
        }
      >
        <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.lg, paddingTop: spacing.md }}>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
            <Image source={require('../../assets/icon.png')} accessibilityLabel="BookDan" style={{ width: 36, height: 36, borderRadius: 10 }} />
            <T variant="displaySm" color={dark.accent} style={{ fontSize: 22 }}>Bookdan</T>
          </View>
          <Pressable
            onPress={() => router.navigate('/profile')}
            accessibilityRole="button"
            accessibilityLabel={t('tabs.profile')}
            style={{ width: 44, height: 44, borderRadius: 22, borderWidth: 1, borderColor: dark.line, backgroundColor: dark.surface, alignItems: 'center', justifyContent: 'center' }}
          >
            <Ionicons name="person-outline" size={20} color={dark.accent} />
          </Pressable>
        </View>

        <Reveal>
          <T variant="display" color={dark.text} style={{ paddingHorizontal: spacing.lg, marginTop: spacing.lg }}>{t('home.welcome')}</T>
        </Reveal>

        <Reveal index={1}>
        <PressableScale
          onPress={() => router.push('/search')}
          accessibilityRole="search"
          scaleTo={0.98}
          style={{
            flexDirection: 'row', alignItems: 'center', marginHorizontal: spacing.lg, marginTop: spacing.lg, minHeight: 56,
            borderRadius: radius.md, borderWidth: 1, borderColor: dark.line, backgroundColor: dark.surface, padding: spacing.sm,
          }}
        >
          <View style={{ width: 40, height: 40, borderRadius: 10, overflow: 'hidden', alignItems: 'center', justifyContent: 'center' }}>
            <LinearGradient colors={gradients.primary} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }} />
            <Ionicons name="location" size={20} color="#FFFFFF" />
          </View>
          <T variant="small" color={dark.text} numberOfLines={1} style={{ flex: 1, marginLeft: spacing.md }}>{t('search.start')}</T>
          <View style={{ width: 1, height: 24, backgroundColor: dark.line, marginHorizontal: spacing.sm }} />
          <Ionicons name="calendar-outline" size={18} color={dark.accent} />
          <T variant="small" color={dark.textSoft} style={{ marginLeft: spacing.sm, marginRight: spacing.sm }}>{t('home.dates')}</T>
        </PressableScale>
        </Reveal>

        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, paddingVertical: spacing.md, gap: spacing.sm }}>
          {CATEGORIES.map((c) => (
            <Pill key={c} label={c === 'all' ? t('common.all') : t(`type.${c}`)} selected={category === c} onPress={() => setCategory(c)} />
          ))}
        </ScrollView>

        {all.isPending ? (
          <View style={{ flexDirection: 'row', gap: spacing.md, paddingHorizontal: spacing.lg }}>
            <Skeleton dark width={wideWidth} height={Math.round(wideWidth * 0.78)} radius={radius.lg} />
            <Skeleton dark width={wideWidth} height={Math.round(wideWidth * 0.78)} radius={radius.lg} />
          </View>
        ) : all.isError ? (
          <EmptyState icon="⚠️" title={t('results.error_title')} body={t('results.error_body')} actionLabel={t('common.retry')} onAction={() => all.refetch()} />
        ) : featured.length === 0 ? (
          <EmptyState icon="🔎" title={t('results.empty_title')} body={t('results.empty_body')} />
        ) : (
          <>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
              {featured.map((item, i) => (
                <Reveal key={item.id} index={i}><StayCard item={item} large width={wideWidth} height={Math.round(wideWidth * 0.78)} onPress={() => open(item)} /></Reveal>
              ))}
            </ScrollView>

            {recent.length > 0 ? (
              <View style={{ marginTop: spacing.xl }}>
                <T variant="displaySm" color={dark.text} style={{ paddingHorizontal: spacing.lg, marginBottom: spacing.md }}>{t('home.recently_viewed')}</T>
                <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: spacing.lg, gap: spacing.md }}>
                  {recent.map((item) => (
                    <StayCard key={item.id} item={item} width={gridWidth} height={Math.round(gridWidth * 0.95)} onPress={() => open(item)} />
                  ))}
                </ScrollView>
              </View>
            ) : null}

            {more.length > 0 ? (
              <View style={{ marginTop: spacing.xl }}>
                <T variant="displaySm" color={dark.text} style={{ paddingHorizontal: spacing.lg }}>{t('home.more')}</T>
                <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.md, paddingHorizontal: spacing.lg, marginTop: spacing.md }}>
                  {more.map((item, i) => (
                    <Reveal key={item.id} index={i}><StayCard item={item} width={gridWidth} height={Math.round(gridWidth * 0.95)} onPress={() => open(item)} /></Reveal>
                  ))}
                </View>
              </View>
            ) : null}
          </>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}
