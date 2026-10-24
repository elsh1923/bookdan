import { FlashList } from '@shopify/flash-list';
import { useRouter, useLocalSearchParams } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { ALL_AMENITIES, nightsBetween, type Amenity, type PropertyType, type SearchSort } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, EmptyState, Reveal, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { HotelMap } from '../src/HotelMap';
import { Photo } from '../src/Photo';
import { PropertyCard } from '../src/PropertyCard';
import { price, shortDate } from '../src/format';
import { useSearch, type SearchCriteria } from '../src/queries';

const TYPES: PropertyType[] = ['hotel', 'guesthouse', 'lodge', 'resort'];
const SORTS: SearchSort[] = ['recommended', 'price_asc', 'price_desc', 'rating'];
const SORTS_NEAR: SearchSort[] = ['distance', ...SORTS]; // with the guest's position, nearest first is on offer
const RATINGS = [0, 3, 3.5, 4, 4.5];
// Price per night in santim: [from, to], 0 = no limit on that side.
const PRICE_BRACKETS: [number, number][] = [[0, 0], [0, 150_000], [150_000, 300_000], [300_000, 500_000], [500_000, 0]];
const STALE_AFTER_MS = 5 * 60_000;

interface Filters {
  types: PropertyType[];
  amenities: Amenity[];
  min_price: number;
  max_price: number;
  min_rating: number;
  breakfast: boolean;
  deals: boolean;
}
const NO_FILTERS: Filters = { types: [], amenities: [], min_price: 0, max_price: 0, min_rating: 0, breakfast: false, deals: false };

function bracketLabel(t: (k: string, o?: Record<string, unknown>) => string, [from, to]: [number, number]): string {
  if (!from && !to) return t('filters.price_any');
  if (!from) return t('filters.price_under', { amount: price(to) });
  if (!to) return t('filters.price_over', { amount: price(from) });
  return t('filters.price_between', { from: price(from), to: price(to) });
}

function Sheet({ visible, onClose, children }: { visible: boolean; onClose: () => void; children: React.ReactNode }) {
  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={onClose}>
      <Pressable style={{ flex: 1, backgroundColor: 'rgba(0,0,0,0.4)' }} onPress={onClose} />
      <View style={{ backgroundColor: colors.surface, borderTopLeftRadius: radius.lg, borderTopRightRadius: radius.lg, padding: spacing.xl, maxHeight: '85%', width: '100%', maxWidth: 640, alignSelf: 'center' }}>
        {children}
      </View>
    </Modal>
  );
}

function SortSheet({ visible, value, near, onClose, onPick }: { visible: boolean; value: SearchSort; near: boolean; onClose: () => void; onPick: (s: SearchSort) => void }) {
  const { t } = useTranslation();
  return (
    <Sheet visible={visible} onClose={onClose}>
      <T variant="h2">{t('results.sort_title')}</T>
      {(near ? SORTS_NEAR : SORTS).map((s) => (
        <Pressable
          key={s}
          accessibilityRole="radio"
          accessibilityState={{ selected: value === s }}
          onPress={() => onPick(s)}
          style={{ minHeight: 52, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', borderBottomWidth: 1, borderBottomColor: colors.line }}
        >
          <T color={value === s ? colors.primaryInk : colors.ink}>{t(`results.sort_${s}`)}</T>
          {value === s ? <T color={colors.primaryInk}>✓</T> : null}
        </Pressable>
      ))}
    </Sheet>
  );
}

function FilterSheet({ visible, filters, facets, onClose, onApply }: {
  visible: boolean; filters: Filters; facets?: { types: Record<string, number>; amenities: Record<string, number> };
  onClose: () => void; onApply: (f: Filters) => void;
}) {
  const { t } = useTranslation();
  const [f, setF] = useState(filters);
  const toggle = <V,>(list: V[], v: V) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);
  const count = (n?: number) => (n === undefined ? '' : ` (${n})`);
  return (
    <Sheet visible={visible} onClose={onClose}>
      <T variant="h2">{t('filters.title')}</T>
      <ScrollView>
        <T variant="h3" style={{ marginTop: spacing.lg }}>{t('filters.price')}</T>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
          {PRICE_BRACKETS.map((b) => (
            <Chip key={b.join('-')} label={bracketLabel(t, b)} selected={f.min_price === b[0] && f.max_price === b[1]} onPress={() => setF({ ...f, min_price: b[0], max_price: b[1] })} />
          ))}
        </View>
        <T variant="h3" style={{ marginTop: spacing.lg }}>{t('filters.rating')}</T>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
          {RATINGS.map((r) => (
            <Chip key={r} label={r === 0 ? t('filters.rating_any') : `★ ${t('filters.rating_min', { n: r })}`} selected={f.min_rating === r} onPress={() => setF({ ...f, min_rating: r })} />
          ))}
        </View>
        <T variant="h3" style={{ marginTop: spacing.lg }}>{t('filters.offers')}</T>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
          <Chip label={t('filters.breakfast')} selected={f.breakfast} onPress={() => setF({ ...f, breakfast: !f.breakfast })} />
          <Chip label={t('filters.deals')} selected={f.deals} onPress={() => setF({ ...f, deals: !f.deals })} />
        </View>
        <T variant="h3" style={{ marginTop: spacing.lg }}>{t('filters.type')}</T>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
          {TYPES.map((x) => <Chip key={x} label={`${t(`type.${x}`)}${count(facets?.types[x] ?? (facets ? 0 : undefined))}`} selected={f.types.includes(x)} onPress={() => setF({ ...f, types: toggle(f.types, x) })} />)}
        </View>
        <T variant="h3" style={{ marginTop: spacing.lg }}>{t('filters.amenities')}</T>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
          {/* Only features some hotel in these results has (plus any already ticked), so no filter leads to an empty list. */}
          {ALL_AMENITIES.filter((x) => !facets || (facets.amenities[x] ?? 0) > 0 || f.amenities.includes(x)).map((x) => <Chip key={x} label={`${t(`amenity.${x}`)}${count(facets?.amenities[x] ?? (facets ? 0 : undefined))}`} selected={f.amenities.includes(x)} onPress={() => setF({ ...f, amenities: toggle(f.amenities, x) })} />)}
        </View>
      </ScrollView>
      <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.xl }}>
        <Button variant="secondary" title={t('filters.reset')} style={{ flex: 1 }} onPress={() => setF(NO_FILTERS)} />
        <Button title={t('filters.apply')} style={{ flex: 2 }} onPress={() => onApply(f)} />
      </View>
    </Sheet>
  );
}

export default function Results() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const p = useLocalSearchParams<{ q?: string; in: string; out: string; guests: string; children?: string; rooms?: string; lat?: string; lng?: string }>();
  // "Near me": the guest's position came with the search.
  const near = p.lat && p.lng && !isNaN(Number(p.lat)) && !isNaN(Number(p.lng)) ? { lat: Number(p.lat), lng: Number(p.lng) } : null;
  const months = t('months.short', { returnObjects: true }) as unknown as string[];

  const [filters, setFilters] = useState<Filters>(NO_FILTERS);
  const [sort, setSort] = useState<SearchSort>(near ? 'distance' : 'recommended');
  const [picked, setPicked] = useState<string | null>(null); // the hotel tapped on the map
  const [showFilters, setShowFilters] = useState(false);
  const [showSort, setShowSort] = useState(false);
  const [view, setView] = useState<'list' | 'map'>('list');

  const criteria = useMemo<SearchCriteria>(
    () => ({
      q: p.q ?? '', check_in: p.in, check_out: p.out, guests: Number(p.guests) || 1, rooms: Number(p.rooms) || 1,
      types: filters.types, amenities: filters.amenities, sort,
      min_price: filters.min_price || undefined, max_price: filters.max_price || undefined, min_rating: filters.min_rating || undefined,
      breakfast: filters.breakfast || undefined, deals: filters.deals || undefined,
      lat: near?.lat, lng: near?.lng,
    }),
    [p.q, p.in, p.out, p.guests, p.rooms, filters, sort, near?.lat, near?.lng],
  );
  const query = useSearch(criteria);
  const items = query.data?.pages.flatMap((pg) => pg.items) ?? [];
  const total = query.data?.pages[0]?.total ?? items.length;
  const facets = query.data?.pages[0]?.facets;
  // The map shows every hotel, not just the first page.
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = query;
  useEffect(() => {
    if (view === 'map' && hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [view, hasNextPage, isFetchingNextPage, fetchNextPage]);
  const pickedItem = items.find((i) => i.id === picked) ?? null;
  const nights = nightsBetween(p.in, p.out);
  const stale = query.data && Date.now() - query.dataUpdatedAt > STALE_AFTER_MS;
  const filterCount =
    filters.types.length + filters.amenities.length + (filters.min_price || filters.max_price ? 1 : 0) + (filters.min_rating ? 1 : 0) + (filters.breakfast ? 1 : 0) + (filters.deals ? 1 : 0);
  const quickRating = filters.min_rating === 4;

  const errorView = () => {
    const offline = query.error instanceof ApiError && query.error.code === 'NETWORK';
    return (
      <EmptyState
        icon={offline ? '📡' : '⚠️'}
        title={t(offline ? 'results.offline_title' : 'results.error_title')}
        body={t(offline ? 'results.offline_body' : 'results.error_body')}
        actionLabel={t('common.retry')}
        onAction={() => query.refetch()}
      />
    );
  };

  return (
    <SafeAreaView style={{ flex: 1 }} edges={['top']}>
      <View style={{ paddingTop: spacing.lg, paddingBottom: spacing.sm }}>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingHorizontal: spacing.lg }}>
          <Pressable accessibilityLabel="Back" onPress={() => router.back()} style={{ width: 44, height: 44, justifyContent: 'center' }}>
            <T variant="h2">←</T>
          </Pressable>
          <View style={{ flex: 1 }}>
            <T variant="h3" numberOfLines={1}>{p.q || (near ? t('results.near_me') : t('results.title'))}</T>
            <T variant="small" color={colors.inkSoft}>
              {shortDate(p.in, months)} – {shortDate(p.out, months)} · {t('common.night', { count: nights })} · {t('common.guest', { count: Number(p.guests) || 1 })}{Number(p.rooms) > 1 ? ` · ${t('common.room', { count: Number(p.rooms) })}` : ''}
            </T>
          </View>
        </View>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: spacing.sm, paddingHorizontal: spacing.lg, paddingTop: spacing.sm }}>
          <Chip label={`${t('results.sort')}: ${t(`results.sort_${sort}`)}`} selected={sort !== 'recommended'} onPress={() => setShowSort(true)} />
          <Chip label={`${t('results.filters')}${filterCount ? ` (${filterCount})` : ''}`} selected={filterCount > 0} onPress={() => setShowFilters(true)} />
          <Chip label={t('results.quick_rating')} selected={quickRating} onPress={() => setFilters({ ...filters, min_rating: quickRating ? 0 : 4 })} />
          <Chip label={t('filters.breakfast')} selected={filters.breakfast} onPress={() => setFilters({ ...filters, breakfast: !filters.breakfast })} />
          <Chip label={t('amenity.wifi')} selected={filters.amenities.includes('wifi')} onPress={() => setFilters({ ...filters, amenities: filters.amenities.includes('wifi') ? filters.amenities.filter((a) => a !== 'wifi') : [...filters.amenities, 'wifi'] })} />
          <Chip label={t('results.list')} selected={view === 'list'} onPress={() => setView('list')} />
          <Chip label={t('results.map')} selected={view === 'map'} onPress={() => setView('map')} />
        </ScrollView>
        {stale ? <T variant="label" color={colors.warning} style={{ marginTop: spacing.sm, paddingHorizontal: spacing.lg }}>⏱ {t('common.stale')}</T> : null}
      </View>

      {view === 'map' ? (
        query.isPending ? (
          <View style={{ flex: 1, justifyContent: 'center' }}><ActivityIndicator size="large" color={colors.primary} /></View>
        ) : (
          <View style={{ flex: 1 }}>
            <HotelMap items={items} me={near} onPick={setPicked} />
            {pickedItem ? (
              <Pressable
                accessibilityRole="button"
                onPress={() => router.push({ pathname: '/property/[id]', params: { id: pickedItem.id, in: p.in, out: p.out, guests: p.guests, children: p.children ?? '0', rooms: p.rooms ?? '1' } })}
                style={{ position: 'absolute', left: spacing.lg, right: spacing.lg, bottom: spacing.lg, flexDirection: 'row', alignItems: 'center', gap: spacing.md, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.line }}
              >
                <Photo uri={pickedItem.photo_url} accent={pickedItem.accent} name={pickedItem.name} showInitial={false} style={{ width: 64, height: 64, borderRadius: radius.md }} />
                <View style={{ flex: 1 }}>
                  <T variant="h3" numberOfLines={1}>{pickedItem.name}</T>
                  <T variant="small" color={colors.inkSoft}>{pickedItem.rating ? `★ ${pickedItem.rating.toFixed(1)} · ` : ''}{price(pickedItem.from_price_santim, pickedItem.currency)} {t('common.per_night')}</T>
                </View>
                <T variant="h3" color={colors.primaryInk}>{t('results.map_view')}</T>
              </Pressable>
            ) : (
              <View pointerEvents="none" style={{ position: 'absolute', left: 0, right: 0, bottom: spacing.lg, alignItems: 'center' }}>
                <T variant="small" color={colors.ink} style={{ backgroundColor: 'rgba(255,255,255,0.92)', paddingHorizontal: spacing.md, paddingVertical: spacing.xs, borderRadius: radius.pill }}>{t('results.map_hint')}</T>
              </View>
            )}
          </View>
        )
      ) : query.isPending ? (
        <View style={{ padding: spacing.lg, gap: spacing.lg }}>
          {[0, 1, 2].map((i) => (
            <View key={i}>
              <Skeleton height={190} radius={radius.lg} />
              <Skeleton height={20} width="70%" style={{ marginTop: spacing.md }} />
              <Skeleton height={14} width="45%" style={{ marginTop: spacing.sm }} />
            </View>
          ))}
        </View>
      ) : query.isError ? (
        errorView()
      ) : items.length === 0 ? (
        <EmptyState icon="🔎" title={t('results.empty_title')} body={t('results.empty_body')} actionLabel={filterCount ? t('filters.reset') : undefined} onAction={filterCount ? () => setFilters(NO_FILTERS) : undefined} />
      ) : (
        <FlashList
          data={items}
          keyExtractor={(i) => i.id}
          contentContainerStyle={{ padding: spacing.lg }}
          renderItem={({ item, index }) => (
            <Reveal index={index}><PropertyCard item={item} onPress={() => router.push({ pathname: '/property/[id]', params: { id: item.id, in: p.in, out: p.out, guests: p.guests, children: p.children ?? '0', rooms: p.rooms ?? '1' } })} /></Reveal>
          )}
          onEndReached={() => query.hasNextPage && !query.isFetchingNextPage && query.fetchNextPage()}
          onEndReachedThreshold={0.5}
          ListHeaderComponent={<T variant="h3" style={{ marginBottom: spacing.md }}>{t('results.showing', { count: total })}</T>}
          ListFooterComponent={query.isFetchingNextPage ? <ActivityIndicator color={colors.primary} /> : null}
          onRefresh={() => query.refetch()}
          refreshing={query.isRefetching && !query.isFetchingNextPage}
          extraData={i18n.language}
        />
      )}

      <SortSheet visible={showSort} value={sort} near={!!near} onClose={() => setShowSort(false)} onPick={(s) => { setSort(s); setShowSort(false); }} />
      <FilterSheet
        key={`${JSON.stringify(filters)}-${showFilters}`}
        visible={showFilters}
        filters={filters}
        facets={facets}
        onClose={() => setShowFilters(false)}
        onApply={(f) => { setFilters(f); setShowFilters(false); }}
      />
    </SafeAreaView>
  );
}
