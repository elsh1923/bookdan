import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { useMemo, useState } from 'react';
import { Pressable, ScrollView, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { DISPLAY_CURRENCIES, type CurrencyInfo, type ShownCurrency } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, fonts, radius, shadow, spacing } from '@bookdan/ui';
import { useDisplay } from '../src/display';
import { usePrefs } from '../src/store';

/**
 * Choose the currency prices are shown in, like in a travel booking app: the default (birr), the popular currencies, then the rest. Only currencies the server
 * has a fresh rate for are listed. It is a guide only: the guest is always charged and refunded in birr, and the screen says so.
 */
export default function CurrencyScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const shown = useDisplay((s) => s.currency);
  const available = useDisplay((s) => s.available);
  const setCurrency = usePrefs((s) => s.setCurrency);
  const [searching, setSearching] = useState(false);
  const [needle, setNeedle] = useState('');

  const { popular, more } = useMemo(() => {
    const n = needle.trim().toLowerCase();
    const list = DISPLAY_CURRENCIES.filter((c) => available.includes(c.code)).filter((c) => !n || c.name.toLowerCase().includes(n) || c.code.toLowerCase().includes(n));
    return { popular: list.filter((c) => c.popular), more: list.filter((c) => !c.popular) };
  }, [available, needle]);
  const etbMatches = !needle.trim() || 'ethiopian birr'.includes(needle.trim().toLowerCase()) || 'etb'.includes(needle.trim().toLowerCase());

  const choose = (c: ShownCurrency) => {
    setCurrency(c);
    router.back();
  };
  const close = () => (router.canGoBack() ? router.back() : router.replace('/'));

  const Row = ({ code, name }: { code: ShownCurrency; name: string }) => {
    const selected = shown === code;
    return (
      <Pressable
        accessibilityRole="radio"
        accessibilityState={{ selected }}
        onPress={() => choose(code)}
        style={[
          { flexDirection: 'row', alignItems: 'center', gap: spacing.md, padding: spacing.md, marginBottom: spacing.sm, borderRadius: radius.md, borderWidth: 1.5, backgroundColor: selected ? colors.primarySoft : colors.surface, borderColor: selected ? colors.primary : colors.line },
          selected ? null : shadow.card,
        ]}
      >
        <Ionicons name={selected ? 'radio-button-on' : 'radio-button-off'} size={24} color={selected ? colors.primary : colors.muted} />
        <T variant="h3" color={selected ? colors.primaryInk : colors.ink} style={{ flex: 1 }}>{name}</T>
        <T style={{ fontFamily: fonts.bold }} color={selected ? colors.primaryInk : colors.muted}>{code}</T>
      </Pressable>
    );
  };
  const rows = (list: CurrencyInfo[]) => list.map((c) => <Row key={c.code} code={c.code} name={c.name} />);

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.canvas }} edges={['top']}>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.md, paddingVertical: spacing.sm, backgroundColor: colors.surface, borderBottomWidth: 1, borderBottomColor: colors.line }}>
        <Pressable accessibilityRole="button" accessibilityLabel={t('common.back')} onPress={close} hitSlop={8} style={{ width: 40, height: 40, alignItems: 'center', justifyContent: 'center' }}>
          <Ionicons name="arrow-back" size={24} color={colors.ink} />
        </Pressable>
        {searching ? (
          <TextInput
            autoFocus
            value={needle}
            onChangeText={setNeedle}
            placeholder={t('currency.search')}
            placeholderTextColor={colors.muted}
            style={{ flex: 1, fontFamily: fonts.regular, fontSize: 16, color: colors.ink, paddingVertical: spacing.xs }}
          />
        ) : (
          <T variant="h2" style={{ flex: 1 }}>{t('currency.title')}</T>
        )}
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={t('currency.search')}
          onPress={() => { setSearching((v) => !v); setNeedle(''); }}
          hitSlop={8}
          style={{ width: 40, height: 40, alignItems: 'center', justifyContent: 'center' }}
        >
          <Ionicons name={searching ? 'close' : 'search'} size={22} color={colors.ink} />
        </Pressable>
      </View>

      <ScrollView contentContainerStyle={{ padding: spacing.lg, paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
        <View style={{ flexDirection: 'row', gap: spacing.sm, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.warningSoft }}>
          <Ionicons name="information-circle-outline" size={20} color={colors.warning} />
          <T variant="small" color={colors.inkSoft} style={{ flex: 1 }}>{t('currency.note')}</T>
        </View>

        {etbMatches ? (
          <>
            <T variant="small" color={colors.muted} style={{ marginTop: spacing.lg, marginBottom: spacing.sm }}>{t('currency.default')}</T>
            <Row code="ETB" name={t('currency.etb_name')} />
          </>
        ) : null}
        {popular.length > 0 ? (
          <>
            <T variant="small" color={colors.muted} style={{ marginTop: spacing.lg, marginBottom: spacing.sm }}>{t('currency.popular')}</T>
            {rows(popular)}
          </>
        ) : null}
        {more.length > 0 ? (
          <>
            <T variant="small" color={colors.muted} style={{ marginTop: spacing.lg, marginBottom: spacing.sm }}>{t('currency.more')}</T>
            {rows(more)}
          </>
        ) : null}
        {available.length === 0 ? <T color={colors.inkSoft} style={{ marginTop: spacing.lg }}>{t('currency.none')}</T> : null}
        {needle.trim() && !etbMatches && popular.length + more.length === 0 ? <T color={colors.inkSoft} style={{ marginTop: spacing.lg }}>{t('currency.no_match')}</T> : null}
        <T variant="small" color={colors.muted} style={{ marginTop: spacing.xl }}>{t('currency.attribution')}</T>
      </ScrollView>
    </SafeAreaView>
  );
}
