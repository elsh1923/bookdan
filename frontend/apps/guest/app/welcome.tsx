import { Image } from 'expo-image';
import { getLocales } from 'expo-localization';
import { useRouter } from 'expo-router';
import { ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, pickLanguage, useTranslation } from '@bookdan/i18n';
import { Button, Chip, T, colors, spacing } from '@bookdan/ui';
import { usePrefs } from '../src/store';

export default function Welcome() {
  const { t } = useTranslation();
  const router = useRouter();
  const { language, currency, setLanguage, setCurrency, finishOnboarding } = usePrefs();
  const current = language ?? pickLanguage(getLocales()[0]?.languageTag);

  return (
    <SafeAreaView style={{ flex: 1 }}>
     <ScrollView contentContainerStyle={{ flexGrow: 1, padding: spacing.xl, justifyContent: 'space-between', gap: spacing.xl }}>
      <View style={{ marginTop: spacing.xl }}>
        <Image source={require('../assets/icon.png')} accessibilityLabel="BookDan" style={{ width: 72, height: 72, borderRadius: 20 }} />
        <T variant="h1" style={{ marginTop: spacing.xl }}>{t('welcome.title')}</T>
        <T color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('welcome.subtitle')}</T>

        <T variant="label" color={colors.muted} style={{ marginTop: spacing.xxl }}>{t('welcome.language').toUpperCase()}</T>
        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm, flexWrap: 'wrap' }}>
          {SUPPORTED_LANGUAGES.map((l) => (
            <Chip key={l} label={LANGUAGE_NAMES[l]} selected={current === l} onPress={() => setLanguage(l)} />
          ))}
        </View>

        <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('welcome.currency').toUpperCase()}</T>
        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
          <Chip label="ETB" selected={currency === 'ETB'} onPress={() => setCurrency('ETB')} />
        </View>
      </View>

      <Button
        title={t('welcome.start')}
        onPress={() => {
          setLanguage(current);
          finishOnboarding();
          router.replace('/');
        }}
      />
     </ScrollView>
    </SafeAreaView>
  );
}
