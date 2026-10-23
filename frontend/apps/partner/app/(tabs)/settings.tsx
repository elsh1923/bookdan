import { useQueryClient } from '@tanstack/react-query';
import { ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, useTranslation } from '@bookdan/i18n';
import { Badge, Button, Chip, Reveal, T, colors, spacing } from '@bookdan/ui';
import { api, isMockApi } from '../../src/api';
import { AlertsChecklist } from '../../src/AlertsChecklist';
import { useAuth } from '../../src/auth';
import { useInbox } from '../../src/queries';
import { endSession } from '../../src/session';
import { usePrefs } from '../../src/store';

export default function Settings() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const identity = useAuth((s) => s.identity);
  const setLanguage = usePrefs((s) => s.setLanguage);
  const inbox = useInbox();

  return (
    <SafeAreaView style={{ flex: 1 }} edges={['top']}>
      <ScrollView contentContainerStyle={{ padding: spacing.lg }}>
        <Reveal><T variant="display">{t('partner.settings_title')}</T></Reveal>

        <Reveal index={1}>
          <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('partner.hotel').toUpperCase()}</T>
          <T variant="h3" style={{ marginTop: spacing.xs }}>{inbox.data?.property.name ?? '—'}</T>
        </Reveal>

        <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('partner.language').toUpperCase()}</T>
        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
          {SUPPORTED_LANGUAGES.map((l) => (
            <Chip key={l} label={LANGUAGE_NAMES[l]} selected={i18n.language === l} onPress={() => setLanguage(l)} />
          ))}
        </View>

        <Reveal index={3}><AlertsChecklist /></Reveal>

        <View style={{ marginTop: spacing.xl }}>
          {identity ? <T color={colors.inkSoft}>{t('partner.signed_in_as', { who: identity.value })}</T> : null}
          <Button variant="secondary" icon="log-out-outline" title={t('partner.sign_out')} onPress={() => { endSession(); }} style={{ marginTop: spacing.sm }} />
        </View>

        {isMockApi && api.demo ? (
          <View style={{ marginTop: spacing.xxl, gap: spacing.sm }}>
            <Badge tone="warning" icon="⚠" label={`SIMULATED · ${t('partner.demo')}`} />
            <Button
              variant="secondary"
              icon="add-circle-outline"
              title={t('partner.demo_add')}
              onPress={async () => {
                await api.demo?.addIncomingBooking();
                qc.invalidateQueries({ queryKey: ['inbox'] });
              }}
            />
            <Button variant="ghost" title={t('partner.demo_expire')} onPress={() => api.demo?.expireAccessToken()} />
            <Button variant="ghost" title={t('partner.demo_revoke')} onPress={() => api.demo?.revokeRefreshToken()} />
          </View>
        ) : null}
      </ScrollView>
    </SafeAreaView>
  );
}
