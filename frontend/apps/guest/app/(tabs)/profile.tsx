import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { Alert, Pressable, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, useTranslation } from '@bookdan/i18n';
import { currencyInfo, maskId } from '@bookdan/domain';
import { Badge, Button, Chip, PressableScale, Reveal, T, colors, radius, spacing } from '@bookdan/ui';
import { api, isMockApi } from '../../src/api';
import { useAuth } from '../../src/auth';
import { useProfile } from '../../src/queries';
import { signOutAndWipe } from '../../src/session';
import { useDisplay } from '../../src/display';
import { usePrefs } from '../../src/store';

export default function Profile() {
  const { t, i18n } = useTranslation();
  // Closing the account is permanent, so it asks twice: a plain question, then the explanation of what goes and what stays.
  const askDelete = () =>
    Alert.alert(t('account.delete_title'), t('account.delete_body'), [
      { text: t('common.cancel'), style: 'cancel' },
      {
        text: t('account.delete_confirm'),
        style: 'destructive',
        onPress: async () => {
          try {
            await api.deleteAccount();
            await signOutAndWipe();
          } catch (e) {
            Alert.alert(t('account.delete_title'), e instanceof ApiError && e.code === 'ACTIVE_BOOKINGS' ? t('account.delete_active') : t('results.error_body'));
          }
        },
      },
    ]);
  const { setLanguage, setCurrency } = usePrefs();
  // Only currencies with a fresh server rate can be chosen; `shown` is what the app is actually showing.
  const shown = useDisplay((s) => s.currency);
  const available = useDisplay((s) => s.available);
  const router = useRouter();
  const identity = useAuth((s) => s.identity);
  const profile = useProfile(!!identity);
  return (
    <SafeAreaView style={{ flex: 1, padding: spacing.lg }} edges={['top']}>
      <Reveal><T variant="h1">{t('profile.title')}</T></Reveal>

      {identity ? (
        <Reveal index={1}>
        <PressableScale scaleTo={0.98} onPress={() => router.push('/guest-details')} accessibilityRole="button" style={{ marginTop: spacing.xl, minHeight: 64, flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: profile.data ? colors.line : colors.primary, backgroundColor: profile.data ? colors.surface : colors.primarySoft }}>
          <Ionicons name="id-card-outline" size={24} color={colors.primaryInk} />
          <View style={{ flex: 1 }}>
            <T variant="h3">{t('profile.my_details')}</T>
            <T variant="small" color={colors.inkSoft}>
              {profile.data ? `${profile.data.full_name} · ${t(`idType.${profile.data.id_type}`)} ${maskId(profile.data.id_last4)}` : t('profile.add_id')}
            </T>
          </View>
          <Ionicons name="chevron-forward" size={20} color={colors.muted} />
        </PressableScale>
        </Reveal>
      ) : null}

      <Reveal index={2}>
      <PressableScale scaleTo={0.98} onPress={() => router.push('/bookings')} accessibilityRole="button" style={{ marginTop: spacing.md, minHeight: 56, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line }}>
        <T variant="h3">{t('tabs.bookings')}</T>
        <Ionicons name="chevron-forward" size={20} color={colors.muted} />
      </PressableScale>
      </Reveal>

      <Reveal index={3}>
      <PressableScale scaleTo={0.98} onPress={() => router.push('/support')} accessibilityRole="button" style={{ marginTop: spacing.md, minHeight: 56, flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line }}>
        <Ionicons name="chatbubbles-outline" size={22} color={colors.primaryInk} />
        <View style={{ flex: 1 }}>
          <T variant="h3">{t('support.title')}</T>
          <T variant="small" color={colors.inkSoft}>{t('support.entry_sub')}</T>
        </View>
        <Ionicons name="chevron-forward" size={20} color={colors.muted} />
      </PressableScale>
      </Reveal>

      <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('profile.language').toUpperCase()}</T>
      <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
        {SUPPORTED_LANGUAGES.map((l) => (
          <Chip key={l} label={LANGUAGE_NAMES[l]} selected={i18n.language === l} onPress={() => setLanguage(l)} />
        ))}
      </View>

      <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('profile.currency').toUpperCase()}</T>
      <PressableScale
        onPress={() => router.push('/currency')}
        accessibilityRole="button"
        accessibilityLabel={t('currency.title')}
        scaleTo={0.99}
        style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, marginTop: spacing.sm, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}
      >
        <Ionicons name="cash-outline" size={22} color={colors.primaryInk} />
        <T variant="h3" style={{ flex: 1 }}>{shown === 'ETB' ? t('currency.etb_name') : currencyInfo(shown).name}</T>
        <T color={colors.muted}>{shown}</T>
        <Ionicons name="chevron-forward" size={18} color={colors.muted} />
      </PressableScale>
      {shown !== 'ETB' ? <T variant="small" color={colors.muted} style={{ marginTop: spacing.sm }}>{t('profile.currency_note', { cur: shown })}</T> : null}

      <View style={{ marginTop: spacing.xl }}>
        {identity ? (
          <>
            <T color={colors.inkSoft}>{t('auth.signed_in_as', { who: identity.value })}</T>
            <Button variant="secondary" title={t('auth.sign_out')} onPress={() => { signOutAndWipe(); }} style={{ marginTop: spacing.sm }} />
            <Button variant="ghost" title={t('account.delete')} onPress={askDelete} style={{ marginTop: spacing.xs }} />
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
