import { Ionicons } from '@expo/vector-icons';
import * as Updates from 'expo-updates';
import { useEffect } from 'react';
import { AppState, Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, radius, spacing } from '@bookdan/ui';

const RECHECK_AFTER_MS = 60_000; // coming back to the app checks again, but never more than once a minute

/**
 * Over-the-air updates. A new version downloads quietly in the background; this banner says it is ready and
 * restarts the app on tap. It never restarts by itself, because that could interrupt a payment or an answer.
 * It does nothing in development or in a browser, where there are no updates.
 */
export function UpdateBanner() {
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const { isUpdatePending } = Updates.useUpdates();

  useEffect(() => {
    if (!Updates.isEnabled) return;
    let last = Date.now();
    const sub = AppState.addEventListener('change', async (state) => {
      if (state !== 'active' || Date.now() - last < RECHECK_AFTER_MS) return;
      last = Date.now();
      try {
        const check = await Updates.checkForUpdateAsync();
        if (check.isAvailable) await Updates.fetchUpdateAsync();
      } catch {
        // offline or the update service is unreachable: try again next time
      }
    });
    return () => sub.remove();
  }, []);

  if (!Updates.isEnabled || !isUpdatePending) return null;
  return (
    <View pointerEvents="box-none" style={{ position: 'absolute', top: insets.top + spacing.sm, left: spacing.md, right: spacing.md, zIndex: 100 }}>
      <Pressable
        accessibilityRole="button"
        onPress={() => Updates.reloadAsync()}
        style={{
          flexDirection: 'row', alignItems: 'center', gap: spacing.sm, minHeight: 52, paddingHorizontal: spacing.md,
          borderRadius: radius.md, backgroundColor: colors.ink,
          shadowColor: '#000', shadowOpacity: 0.25, shadowRadius: 12, shadowOffset: { width: 0, height: 6 }, elevation: 8,
        }}
      >
        <Ionicons name="sparkles-outline" size={20} color="#fff" />
        <T variant="small" color="#fff" style={{ flex: 1 }}>{t('updates.ready')}</T>
        <T variant="small" color="#7DB1FF" style={{ fontWeight: '700' }}>{t('updates.restart')}</T>
      </Pressable>
    </View>
  );
}
