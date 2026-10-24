import { Ionicons } from '@expo/vector-icons';
import { useState } from 'react';
import { Modal, NativeScrollEvent, NativeSyntheticEvent, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTranslation } from '@bookdan/i18n';
import { T, spacing } from '@bookdan/ui';
import { Photo } from './Photo';

/**
 * The hotel's photos side by side: swipe through them, dots show where you are, and the "N photos" badge opens them full screen.
 * A hotel without photos shows its coloured placeholder.
 */
export function PhotoCarousel({ photos, accent, name, width, height }: { photos: string[]; accent?: string; name: string; width: number; height: number }) {
  const { t } = useTranslation();
  const [index, setIndex] = useState(0);
  const [open, setOpen] = useState(false);
  const onEnd = (e: NativeSyntheticEvent<NativeScrollEvent>) => setIndex(Math.round(e.nativeEvent.contentOffset.x / width));

  if (photos.length === 0) return <Photo uri={null} accent={accent} name={name} style={{ width, height }} />;
  return (
    <View style={{ width, height }}>
      <ScrollView horizontal pagingEnabled showsHorizontalScrollIndicator={false} onMomentumScrollEnd={onEnd} scrollEventThrottle={32}>
        {photos.map((uri) => <Photo key={uri} uri={uri} accent={accent} name={name} style={{ width, height }} />)}
      </ScrollView>
      {photos.length > 1 ? (
        <View pointerEvents="none" style={{ position: 'absolute', bottom: 40, left: 0, right: 0, flexDirection: 'row', justifyContent: 'center', gap: 6 }}>
          {photos.slice(0, 12).map((_, i) => (
            <View key={i} style={{ width: i === index ? 18 : 6, height: 6, borderRadius: 3, backgroundColor: i === index ? '#fff' : 'rgba(255,255,255,0.55)' }} />
          ))}
        </View>
      ) : null}
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t('property.photos', { count: photos.length })}
        onPress={() => setOpen(true)}
        style={{ position: 'absolute', bottom: 40, right: spacing.lg, flexDirection: 'row', alignItems: 'center', gap: 6, backgroundColor: 'rgba(0,0,0,0.65)', borderRadius: 999, paddingHorizontal: 12, paddingVertical: 6, minHeight: 32 }}
      >
        <T variant="label" color="#fff">{t('property.photos', { count: photos.length })}</T>
        <Ionicons name="expand-outline" size={14} color="#fff" />
      </Pressable>

      <PhotoGallery photos={photos} visible={open} accent={accent} name={name} onClose={() => setOpen(false)} />
    </View>
  );
}

/** Every photo, one under the other, full screen. */
export function PhotoGallery({ photos, visible, accent, name, onClose }: { photos: string[]; visible: boolean; accent?: string; name: string; onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <Modal visible={visible} animationType="fade" onRequestClose={onClose}>
      <SafeAreaView style={{ flex: 1, backgroundColor: '#000' }}>
        <Pressable accessibilityRole="button" accessibilityLabel={t('property.close')} onPress={onClose} style={{ width: 48, height: 48, alignItems: 'center', justifyContent: 'center', alignSelf: 'flex-end' }}>
          <Ionicons name="close" size={28} color="#fff" />
        </Pressable>
        <ScrollView style={{ flex: 1 }}>
          {photos.map((uri) => (
            <View key={uri} style={{ marginBottom: spacing.sm }}>
              <Photo uri={uri} accent={accent} name={name} style={{ width: '100%', aspectRatio: 4 / 3 }} />
            </View>
          ))}
        </ScrollView>
      </SafeAreaView>
    </Modal>
  );
}
