import { useRouter } from 'expo-router';
import { useState } from 'react';
import { ScrollView, View, useWindowDimensions } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { addDays, todayAddis } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, Reveal, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { PropertyTile } from '../../src/PropertyTile';
import { useAllProperties } from '../../src/queries';
import { usePrefs } from '../../src/store';

const MAX_COLUMN = 640;

export default function Wishlist() {
  const { t } = useTranslation();
  const router = useRouter();
  const { width } = useWindowDimensions();
  const saved = usePrefs((s) => s.saved);
  const [today] = useState(() => todayAddis());
  const checkOut = addDays(today, 1);
  const all = useAllProperties({ check_in: today, check_out: checkOut, guests: 1, types: [] });

  // Two columns with the same gutters as the home carousels.
  const tileWidth = Math.floor((Math.min(width, MAX_COLUMN) - spacing.lg * 2 - spacing.md) / 2);
  const items = all.items.filter((i) => saved.includes(i.id));

  return (
    <SafeAreaView style={{ flex: 1 }} edges={['top']}>
      <T variant="h1" style={{ padding: spacing.lg }}>{t('wishlist.title')}</T>
      {all.isPending && saved.length > 0 ? (
        <View style={{ flexDirection: 'row', gap: spacing.md, paddingHorizontal: spacing.lg }}>
          <Skeleton width={tileWidth} height={tileWidth + 56} radius={radius.lg} />
          <Skeleton width={tileWidth} height={tileWidth + 56} radius={radius.lg} />
        </View>
      ) : items.length === 0 ? (
        <EmptyState icon="🤍" title={t('wishlist.empty_title')} body={t('wishlist.empty_body')} />
      ) : (
        <ScrollView contentContainerStyle={{ padding: spacing.lg, paddingTop: 0 }}>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.md }}>
            {items.map((item, i) => (
              <Reveal key={item.id} index={i}>
                <PropertyTile
                  item={item}
                  width={tileWidth}
                  onPress={() => router.push({ pathname: '/property/[id]', params: { id: item.id, in: today, out: checkOut, guests: '1' } })}
                />
              </Reveal>
            ))}
          </View>
        </ScrollView>
      )}
    </SafeAreaView>
  );
}
