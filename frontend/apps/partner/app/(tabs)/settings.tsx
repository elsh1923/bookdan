import { useQueryClient } from '@tanstack/react-query';
import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { ScrollView, View } from 'react-native';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, useTranslation } from '@bookdan/i18n';
import { Badge, Button, Chip, PressableScale, Reveal, T, colors, radius, spacing } from '@bookdan/ui';
import { HeaderBand } from '../../src/HeaderBand';
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
  const router = useRouter();

  return (
    <View style={{ flex: 1 }}>
      <ScrollView contentContainerStyle={{ paddingBottom: spacing.xxl }}>
        <HeaderBand eyebrow={inbox.data?.property.name ?? ''} title={t('partner.settings_title')} />
        <View style={{ padding: spacing.lg }}>

        <Reveal index={1}>
          <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('partner.hotel').toUpperCase()}</T>
          <T variant="h3" style={{ marginTop: spacing.xs }}>{inbox.data?.property.name ?? '—'}</T>
        </Reveal>

        <PressableScale scaleTo={0.98} onPress={() => router.push('/support')} accessibilityRole="button" style={{ marginTop: spacing.xl, minHeight: 56, flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line }}>
          <Ionicons name="chatbubbles-outline" size={22} color={colors.primaryInk} />
          <View style={{ flex: 1 }}>
            <T variant="h3">{t('support.title')}</T>
            <T variant="small" color={colors.inkSoft}>{t('support.entry_sub')}</T>
          </View>
          <Ionicons name="chevron-forward" size={20} color={colors.muted} />
        </PressableScale>

        <PressableScale scaleTo={0.98} onPress={() => router.push('/reviews')} accessibilityRole="button" style={{ marginTop: spacing.md, minHeight: 56, flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line }}>
          <Ionicons name="star-outline" size={22} color={colors.primaryInk} />
          <View style={{ flex: 1 }}>
            <T variant="h3">{t('partner.reviews')}</T>
            <T variant="small" color={colors.inkSoft}>{t('partner.reviews_sub')}</T>
          </View>
          <Ionicons name="chevron-forward" size={20} color={colors.muted} />
        </PressableScale>

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
        </View>
      </ScrollView>
    </View>
  );
}
