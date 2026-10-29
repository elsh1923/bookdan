import { Image } from 'expo-image';
import { getLocales } from 'expo-localization';
import { useRouter } from 'expo-router';
import { ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, pickLanguage, useTranslation } from '@bookdan/i18n';
import { Button, Chip, Pop, PressableScale, Reveal, T, colors, radius, spacing } from '@bookdan/ui';
import { currencyInfo } from '@bookdan/domain';
import { Ionicons } from '@expo/vector-icons';
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
        <Pop><Image source={require('../assets/icon.png')} accessibilityLabel="BookDan" style={{ width: 72, height: 72, borderRadius: 20 }} /></Pop>
        <Reveal index={1}>
          <T variant="h1" style={{ marginTop: spacing.xl }}>{t('welcome.title')}</T>
          <T color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('welcome.subtitle')}</T>
        </Reveal>

        <Reveal index={2}>
        <T variant="label" color={colors.muted} style={{ marginTop: spacing.xxl }}>{t('welcome.language').toUpperCase()}</T>
        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm, flexWrap: 'wrap' }}>
          {SUPPORTED_LANGUAGES.map((l) => (
            <Chip key={l} label={LANGUAGE_NAMES[l]} selected={current === l} onPress={() => setLanguage(l)} />
          ))}
        </View>
        </Reveal>

        <Reveal index={3}>
        <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('welcome.currency').toUpperCase()}</T>
        <PressableScale
          onPress={() => router.push('/currency')}
          accessibilityRole="button"
          accessibilityLabel={t('currency.title')}
          scaleTo={0.99}
          style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, marginTop: spacing.sm, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}
        >
          <T variant="h3" style={{ flex: 1 }}>{currency === 'ETB' ? t('currency.etb_name') : currencyInfo(currency).name}</T>
          <T color={colors.muted}>{currency}</T>
          <Ionicons name="chevron-forward" size={18} color={colors.muted} />
        </PressableScale>
        </Reveal>
      </View>

      <Reveal index={4}>
      <Button
        title={t('welcome.start')}
        onPress={() => {
          setLanguage(current);
          finishOnboarding();
          router.replace('/');
        }}
      />
      </Reveal>
     </ScrollView>
    </SafeAreaView>
  );
}
