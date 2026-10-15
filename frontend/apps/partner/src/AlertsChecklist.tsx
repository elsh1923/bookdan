import { Ionicons } from '@expo/vector-icons';
import { useFocusEffect } from 'expo-router';
import { useCallback, useEffect, useState } from 'react';
import { AppState, Linking, Platform, View } from 'react-native';
import { alertReadiness, isAwaitingAnswer, type AlertFacts, type AlertItem } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, shadow, spacing } from '@bookdan/ui';
import { useInbox } from './queries';
import { readAlertFacts, registerForAlerts, sendTestAlert } from './notifications';

const STATUS_ICON = {
  ok: { name: 'checkmark-circle', color: colors.success },
  action: { name: 'alert-circle', color: colors.danger },
  unknown: { name: 'help-circle', color: colors.muted },
} as const;

function Row({ item }: { item: AlertItem }) {
  const { t } = useTranslation();
  const icon = STATUS_ICON[item.status];
  // Status is the icon AND a word, never colour alone.
  const word = item.status === 'ok' ? t('partner.status_done') : item.status === 'action' ? t('partner.status_todo') : t('partner.status_manual');
  return (
    <View style={{ flexDirection: 'row', gap: spacing.md, paddingVertical: spacing.sm }}>
      <Ionicons name={icon.name} size={24} color={icon.color} />
      <View style={{ flex: 1 }}>
        <T variant="h3">{t(`partner.alert_${item.id}`)}</T>
        {item.status !== 'ok' ? <T variant="small" color={colors.inkSoft}>{t(`partner.alert_${item.id}_hint`)}</T> : null}
      </View>
      <T variant="label" color={icon.color}>{word.toUpperCase()}</T>
    </View>
  );
}

/**
 * Will this phone ring when a booking arrives? Budget phones often silence background apps, so staff get a
 * checklist (permission, loud channel, registered, battery) and a test alert they can hear with the phone locked.
 */
export function AlertsChecklist() {
  const { t } = useTranslation();
  const inbox = useInbox();
  const [facts, setFacts] = useState<AlertFacts | null>(null);
  const [testSent, setTestSent] = useState(false);

  const load = useCallback(() => { readAlertFacts().then(setFacts).catch(() => undefined); }, []);
  useFocusEffect(load);
  // Coming back from the system settings: check again.
  useEffect(() => {
    const sub = AppState.addEventListener('change', (s) => { if (s === 'active') load(); });
    return () => sub.remove();
  }, [load]);

  if (!facts) return null;
  const readiness = alertReadiness(facts);
  const firstWaiting = inbox.data?.items.find(isAwaitingAnswer)?.id ?? 'test-booking';

  return (
    <View style={[{ marginTop: spacing.xl, padding: spacing.lg, borderRadius: radius.lg, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }, shadow.card]}>
      <T variant="displaySm">{t('partner.alerts_title')}</T>
      <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.alerts_intro')}</T>

      {!facts.supported ? (
        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.warningSoft }}>
          <Ionicons name="information-circle-outline" size={20} color={colors.warning} />
          <T variant="small" color={colors.warning} style={{ flex: 1 }}>{t('partner.alerts_unsupported')}</T>
        </View>
      ) : (
        <>
          <View style={{ flexDirection: 'row', gap: spacing.sm, alignItems: 'center', marginTop: spacing.md }}>
            <Ionicons name={readiness.ready ? 'shield-checkmark' : 'warning-outline'} size={20} color={readiness.ready ? colors.success : colors.warning} />
            <T variant="h3" color={readiness.ready ? colors.success : colors.warning} style={{ flex: 1 }}>
              {t(readiness.ready ? 'partner.alerts_ready' : 'partner.alerts_not_ready')}
            </T>
          </View>

          <View style={{ marginTop: spacing.sm }}>
            {readiness.items.map((item) => <Row key={item.id} item={item} />)}
          </View>

          {facts.permission !== 'granted' ? (
            <Button icon="notifications-outline" title={t('partner.alert_enable')} onPress={async () => { await registerForAlerts(); load(); }} style={{ marginTop: spacing.sm }} />
          ) : null}
          <Button variant="secondary" icon="settings-outline" title={t('partner.alert_open_settings')} onPress={() => Linking.openSettings()} style={{ marginTop: spacing.sm }} />
          {Platform.OS === 'android' ? (
            <Button
              variant="secondary"
              icon="battery-charging-outline"
              title={t('partner.alert_open_battery')}
              onPress={() => Linking.sendIntent('android.settings.IGNORE_BATTERY_OPTIMIZATION_SETTINGS').catch(() => Linking.openSettings())}
              style={{ marginTop: spacing.sm }}
            />
          ) : null}
          <Button
            variant="ghost"
            icon="volume-high-outline"
            title={t('partner.alert_test')}
            onPress={async () => { await sendTestAlert(firstWaiting); setTestSent(true); }}
            style={{ marginTop: spacing.sm }}
          />
          {testSent ? <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.alert_test_sent')}</T> : null}
        </>
      )}
    </View>
  );
}
