import { Ionicons } from '@expo/vector-icons';
import { useCallback, useEffect, useState } from 'react';
import { AppState, Linking, View } from 'react-native';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';
import { enableBookingUpdates, readNotifyStatus, type NotifyStatus } from './notifications';

/**
 * Shown while the guest waits for the hotel: a message when it answers, so they do not have to keep the app open.
 * It disappears once updates are on, and it is not shown at all where push cannot work (browser, Expo Go).
 */
export function NotifyMeCard() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<NotifyStatus | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(() => {
    readNotifyStatus().then(setStatus).catch(() => setStatus('unsupported'));
  }, []);
  useEffect(refresh, [refresh]);
  // Coming back from the system settings: look again.
  useEffect(() => {
    const sub = AppState.addEventListener('change', (s) => { if (s === 'active') refresh(); });
    return () => sub.remove();
  }, [refresh]);

  if (status === null || status === 'unsupported' || status === 'on') return null;
  const denied = status === 'denied';
  return (
    <View style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1, borderColor: colors.primary, backgroundColor: colors.primarySoft }}>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
        <Ionicons name="notifications-outline" size={22} color={colors.primaryInk} />
        <T variant="h3" color={colors.primaryInk} style={{ flex: 1 }}>{t('notify.title')}</T>
      </View>
      <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t(denied ? 'notify.denied' : 'notify.body')}</T>
      <Button
        title={t(denied ? 'notify.open_settings' : 'notify.enable')}
        icon={denied ? 'settings-outline' : 'notifications'}
        loading={busy}
        onPress={async () => {
          if (denied) return Linking.openSettings();
          setBusy(true);
          setStatus(await enableBookingUpdates());
          setBusy(false);
        }}
        style={{ marginTop: spacing.md }}
      />
    </View>
  );
}
