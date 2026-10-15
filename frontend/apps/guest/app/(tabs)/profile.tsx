import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { Pressable, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, useTranslation } from '@bookdan/i18n';
import { Badge, Button, Chip, T, colors, radius, spacing } from '@bookdan/ui';
import { api, isMockApi } from '../../src/api';
import { useAuth } from '../../src/auth';
import { signOutAndWipe } from '../../src/session';
import { usePrefs } from '../../src/store';

export default function Profile() {
  const { t, i18n } = useTranslation();
  const { setLanguage, currency, setCurrency } = usePrefs();
  const router = useRouter();
  const identity = useAuth((s) => s.identity);
  return (
    <SafeAreaView style={{ flex: 1, padding: spacing.lg }} edges={['top']}>
      <T variant="h1">{t('profile.title')}</T>

      <Pressable onPress={() => router.push('/bookings')} accessibilityRole="button" style={{ marginTop: spacing.xl, minHeight: 56, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line }}>
        <T variant="h3">{t('tabs.bookings')}</T>
        <Ionicons name="chevron-forward" size={20} color={colors.muted} />
      </Pressable>

      <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('profile.language').toUpperCase()}</T>
      <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
        {SUPPORTED_LANGUAGES.map((l) => (
          <Chip key={l} label={LANGUAGE_NAMES[l]} selected={i18n.language === l} onPress={() => setLanguage(l)} />
        ))}
      </View>

      <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('profile.currency').toUpperCase()}</T>
      <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
        <Chip label="ETB" selected={currency === 'ETB'} onPress={() => setCurrency('ETB')} />
      </View>

      <View style={{ marginTop: spacing.xl }}>
        {identity ? (
          <>
            <T color={colors.inkSoft}>{t('auth.signed_in_as', { who: identity.value })}</T>
            <Button variant="secondary" title={t('auth.sign_out')} onPress={() => { signOutAndWipe(); }} style={{ marginTop: spacing.sm }} />
          </>
        ) : (
          <Button variant="secondary" title={t('bookings.sign_in')} onPress={() => router.push('/signin')} />
        )}
      </View>

      {isMockApi ? (
        <View style={{ marginTop: spacing.xxl, gap: spacing.sm }}>
          <Badge tone="warning" icon="⚠" label={`SIMULATED · ${t('profile.demo')}`} />
          {api.demo && identity ? (
            <>
              <Button variant="secondary" title={t('profile.demo_expire')} onPress={() => api.demo?.expireAccessToken()} />
              <Button variant="ghost" title={t('profile.demo_revoke')} onPress={() => api.demo?.revokeRefreshToken()} />
            </>
          ) : null}
        </View>
      ) : null}
    </SafeAreaView>
  );
}
