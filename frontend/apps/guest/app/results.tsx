import { FlashList } from '@shopify/flash-list';
import { useRouter, useLocalSearchParams } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { nightsBetween, type Amenity, type PropertyType, type SearchItem } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, EmptyState, Reveal, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { PropertyCard } from '../src/PropertyCard';
import { shortDate } from '../src/format';
import { useSearch } from '../src/queries';

const TYPES: PropertyType[] = ['hotel', 'guesthouse', 'lodge', 'resort'];
const AMENITIES: Amenity[] = ['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel'];
const STALE_AFTER_MS = 5 * 60_000;

function FilterSheet({ visible, types, amenities, onClose, onApply }: {
  visible: boolean; types: PropertyType[]; amenities: Amenity[]; onClose: () => void;
  onApply: (types: PropertyType[], amenities: Amenity[]) => void;
}) {
  const { t } = useTranslation();
  const [lt, setLt] = useState(types);
  const [la, setLa] = useState(amenities);
  const toggle = <V,>(list: V[], v: V) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);
  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={onClose}>
      <Pressable style={{ flex: 1, backgroundColor: 'rgba(0,0,0,0.4)' }} onPress={onClose} />
      <View style={{ backgroundColor: colors.surface, borderTopLeftRadius: radius.lg, borderTopRightRadius: radius.lg, padding: spacing.xl, maxHeight: '80%', width: '100%', maxWidth: 640, alignSelf: 'center' }}>
        <T variant="h2">{t('filters.title')}</T>
        <ScrollView>
          <T variant="h3" style={{ marginTop: spacing.lg }}>{t('filters.type')}</T>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
            {TYPES.map((x) => <Chip key={x} label={t(`type.${x}`)} selected={lt.includes(x)} onPress={() => setLt(toggle(lt, x))} />)}
          </View>
          <T variant="h3" style={{ marginTop: spacing.lg }}>{t('filters.amenities')}</T>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
            {AMENITIES.map((x) => <Chip key={x} label={t(`amenity.${x}`)} selected={la.includes(x)} onPress={() => setLa(toggle(la, x))} />)}
          </View>
        </ScrollView>
        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.xl }}>
          <Button variant="secondary" title={t('filters.reset')} style={{ flex: 1 }} onPress={() => { setLt([]); setLa([]); }} />
          <Button title={t('filters.apply')} style={{ flex: 2 }} onPress={() => onApply(lt, la)} />
        </View>
      </View>
    </Modal>
  );
}

export default function Results() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const p = useLocalSearchParams<{ q?: string; in: string; out: string; guests: string; rooms?: string }>();
  const months = t('months.short', { returnObjects: true }) as unknown as string[];

  const [types, setTypes] = useState<PropertyType[]>([]);
  const [amenities, setAmenities] = useState<Amenity[]>([]);
  const [showFilters, setShowFilters] = useState(false);
  const [view, setView] = useState<'list' | 'map'>('list');

  const criteria = useMemo(
    () => ({ q: p.q ?? '', check_in: p.in, check_out: p.out, guests: Number(p.guests) || 1, rooms: Number(p.rooms) || 1, types, amenities }),
    [p.q, p.in, p.out, p.guests, p.rooms, types, amenities],
  );
  const query = useSearch(criteria);
  const items = query.data?.pages.flatMap((pg) => pg.items) ?? [];
  const nights = nightsBetween(p.in, p.out);
  const stale = query.data && Date.now() - query.dataUpdatedAt > STALE_AFTER_MS;
  const filterCount = types.length + amenities.length;

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
      <View style={{ padding: spacing.lg, paddingBottom: spacing.sm }}>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
          <Pressable accessibilityLabel="Back" onPress={() => router.back()} style={{ width: 44, height: 44, justifyContent: 'center' }}>
            <T variant="h2">←</T>
          </Pressable>
          <View style={{ flex: 1 }}>
            <T variant="h3" numberOfLines={1}>{p.q || t('results.title')}</T>
            <T variant="small" color={colors.inkSoft}>
              {shortDate(p.in, months)} – {shortDate(p.out, months)} · {t('common.night', { count: nights })} · {t('common.guest', { count: Number(p.guests) || 1 })}{Number(p.rooms) > 1 ? ` · ${t('common.room', { count: Number(p.rooms) })}` : ''}
            </T>
          </View>
        </View>
        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
          <Chip label={`${t('results.filters')}${filterCount ? ` (${filterCount})` : ''}`} selected={filterCount > 0} onPress={() => setShowFilters(true)} />
          <Chip label={t('results.list')} selected={view === 'list'} onPress={() => setView('list')} />
          <Chip label={t('results.map')} selected={view === 'map'} onPress={() => setView('map')} />
        </View>
        {stale ? <T variant="label" color={colors.warning} style={{ marginTop: spacing.sm }}>⏱ {t('common.stale')}</T> : null}
      </View>

      {view === 'map' ? (
        <EmptyState icon="🗺️" title={t('results.map')} body={t('results.map_soon')} />
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
        <EmptyState icon="🔎" title={t('results.empty_title')} body={t('results.empty_body')} />
      ) : (
        <FlashList
          data={items}
          keyExtractor={(i) => i.id}
          contentContainerStyle={{ padding: spacing.lg }}
          renderItem={({ item, index }) => (
            <Reveal index={index}><PropertyCard item={item} onPress={() => router.push({ pathname: '/property/[id]', params: { id: item.id, in: p.in, out: p.out, guests: p.guests, rooms: p.rooms ?? '1' } })} /></Reveal>
          )}
          onEndReached={() => query.hasNextPage && !query.isFetchingNextPage && query.fetchNextPage()}
          onEndReachedThreshold={0.5}
          ListHeaderComponent={<T variant="small" color={colors.inkSoft} style={{ marginBottom: spacing.md }}>{t('results.count', { count: items.length })}</T>}
          ListFooterComponent={query.isFetchingNextPage ? <ActivityIndicator color={colors.primary} /> : null}
          onRefresh={() => query.refetch()}
          refreshing={query.isRefetching && !query.isFetchingNextPage}
          extraData={i18n.language}
        />
      )}

      <FilterSheet
        key={`${types.join()}-${amenities.join()}-${showFilters}`}
        visible={showFilters}
        types={types}
        amenities={amenities}
        onClose={() => setShowFilters(false)}
        onApply={(tp, am) => { setTypes(tp); setAmenities(am); setShowFilters(false); }}
      />
    </SafeAreaView>
  );
}
