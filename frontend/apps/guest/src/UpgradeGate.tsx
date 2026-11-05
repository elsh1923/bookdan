import { Ionicons } from '@expo/vector-icons';
import { useQuery } from '@tanstack/react-query';
import Constants from 'expo-constants';
import { Linking, View } from 'react-native';
import { isOlderVersion } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api, isMockApi } from './api';

/** This build's version, as in app.json. The server asks builds older than its minimum to update (see backend/internal/api/appversion.go). */
export const APP_VERSION = Constants.expoConfig?.version ?? '';

/**
 * Covers the app with an update screen when the server says this build is too old to work with it. It asks once when the app opens (and again every few
 * minutes), so the person sees a clear message instead of errors. If the server cannot be reached, or sets no minimum, it shows nothing.
 */
export function UpgradeGate() {
  const { t } = useTranslation();
  const config = useQuery({ queryKey: ['app-config'], queryFn: () => api.getAppConfig(), enabled: !isMockApi, staleTime: 5 * 60_000, retry: 1 });
  if (isMockApi || !config.data || !isOlderVersion(APP_VERSION, config.data.min_version)) return null;
  const link = config.data.download_url;
  return (
    <View
      accessibilityViewIsModal
      style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, zIndex: 1000, backgroundColor: colors.background, alignItems: 'center', justifyContent: 'center', padding: spacing.xl }}
    >
      <View style={{ width: 84, height: 84, borderRadius: radius.lg, backgroundColor: colors.primarySoft, alignItems: 'center', justifyContent: 'center', marginBottom: spacing.lg }}>
        <Ionicons name="cloud-download-outline" size={40} color={colors.primaryInk} />
      </View>
      <T variant="h1" style={{ textAlign: 'center' }}>{t('upgrade.title')}</T>
      <T color={colors.muted} style={{ textAlign: 'center', marginTop: spacing.sm, marginBottom: spacing.xl, maxWidth: 320 }}>{t('upgrade.body')}</T>
      {link ? <Button title={t('upgrade.button')} icon="download-outline" onPress={() => Linking.openURL(link)} /> : null}
    </View>
  );
}
