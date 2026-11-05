import { Ionicons } from '@expo/vector-icons';
import { Image } from 'expo-image';
import { getLocales } from 'expo-localization';
import { LinearGradient } from 'expo-linear-gradient';
import { useRouter } from 'expo-router';
import { useEffect, useRef, useState } from 'react';
import { Animated, Easing, Platform, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { currencyInfo } from '@bookdan/domain';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, pickLanguage, useTranslation, type Language } from '@bookdan/i18n';
import { Pop, PressableScale, Reveal, T, colors, fonts, radius, shadow, spacing } from '@bookdan/ui';
import { usePrefs } from '../src/store';

const HERO_HEIGHT = 380;

/** A card that drifts up and down by itself, each at its own pace, so the hero feels alive. */
function Floating({ children, delay = 0, distance = 7, duration = 2600, style }: { children: React.ReactNode; delay?: number; distance?: number; duration?: number; style?: object }) {
  const y = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    const native = Platform.OS !== 'web';
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(y, { toValue: -distance, duration, delay, easing: Easing.inOut(Easing.sin), useNativeDriver: native }),
        Animated.timing(y, { toValue: distance, duration, easing: Easing.inOut(Easing.sin), useNativeDriver: native }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [y, delay, distance, duration]);
  return <Animated.View style={[{ position: 'absolute', transform: [{ translateY: y }] }, style]}>{children}</Animated.View>;
}

/** A small postcard of a place guests can go: a colourful top, the name and one line about it. */
function PlaceCard({ icon, from, to, name, line, tilt }: { icon: keyof typeof Ionicons.glyphMap; from: string; to: string; name: string; line: string; tilt: string }) {
  return (
    <View style={{ width: 168, borderRadius: radius.md, backgroundColor: '#FFFFFF', overflow: 'hidden', transform: [{ rotate: tilt }], ...shadow.card, shadowOpacity: 0.28, shadowRadius: 22 }}>
      <LinearGradient colors={[from, to]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ height: 70, alignItems: 'center', justifyContent: 'center' }}>
        <Ionicons name={icon} size={30} color="rgba(255,255,255,0.92)" />
        <View style={{ position: 'absolute', right: 10, top: 8, flexDirection: 'row', alignItems: 'center', gap: 3, backgroundColor: 'rgba(255,255,255,0.22)', paddingHorizontal: 7, paddingVertical: 2, borderRadius: radius.pill }}>
          <Ionicons name="shield-checkmark" size={11} color="#FFFFFF" />
        </View>
      </LinearGradient>
      <View style={{ padding: spacing.sm + 2 }}>
        <T variant="h3" style={{ fontSize: 15, lineHeight: 20 }} numberOfLines={1}>{name}</T>
        <T variant="small" color={colors.muted} numberOfLines={1} style={{ fontSize: 12, lineHeight: 16 }}>{line}</T>
        <View style={{ flexDirection: 'row', gap: 2, marginTop: 4 }}>
          {[0, 1, 2, 3, 4].map((i) => <Ionicons key={i} name="star" size={10} color={colors.star} />)}
        </View>
      </View>
    </View>
  );
}

/** One promise, in its own colour: a tinted tile that tilts slightly, with the words under it. */
function Point({ icon, label, ink, soft, tilt }: { icon: keyof typeof Ionicons.glyphMap; label: string; ink: string; soft: string; tilt: string }) {
  return (
    <View style={{ flex: 1, alignItems: 'center', gap: 8 }}>
      <View style={{ width: 52, height: 52, borderRadius: 18, backgroundColor: soft, alignItems: 'center', justifyContent: 'center', transform: [{ rotate: tilt }], borderWidth: 1, borderColor: ink + '22' }}>
        <Ionicons name={icon} size={25} color={ink} />
      </View>
      <T variant="small" color={colors.charcoal} style={{ textAlign: 'center', fontSize: 12.5, lineHeight: 17, fontFamily: fonts.medium }}>{label}</T>
    </View>
  );
}

/** The language switch: one rounded track, and a white thumb that slides to the language you pick. */
function LanguageSwitch({ current, onPick }: { current: Language; onPick: (l: Language) => void }) {
  const x = useRef(new Animated.Value(0)).current;
  const width = useRef(0);
  const [thumb, setThumb] = useState(0); // the width of one half of the track, in pixels
  const index = SUPPORTED_LANGUAGES.indexOf(current);
  const slide = (to: number) => Animated.spring(x, { toValue: to, useNativeDriver: Platform.OS !== 'web', friction: 9, tension: 90 }).start();
  useEffect(() => { slide(index * (width.current / SUPPORTED_LANGUAGES.length)); }, [index]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <View
      onLayout={(e) => { width.current = e.nativeEvent.layout.width - 8; x.setValue(index * (width.current / SUPPORTED_LANGUAGES.length)); setThumb(width.current / SUPPORTED_LANGUAGES.length); }}
      style={{ flexDirection: 'row', padding: 4, borderRadius: radius.pill, backgroundColor: colors.fill, borderWidth: 1, borderColor: colors.line }}
    >
      <Animated.View
        pointerEvents="none"
        style={{ position: 'absolute', top: 4, bottom: 4, left: 4, width: thumb || 150, borderRadius: radius.pill, backgroundColor: colors.surface, borderWidth: 1.5, borderColor: colors.primary, transform: [{ translateX: x }], ...shadow.card }}
      />
      {SUPPORTED_LANGUAGES.map((l) => {
        const selected = current === l;
        return (
          <PressableScale key={l} onPress={() => onPick(l)} accessibilityRole="button" accessibilityState={{ selected }} scaleTo={0.98} style={{ flex: 1, alignItems: 'center', paddingVertical: 10 }}>
            <T style={{ fontFamily: l === 'am' ? fonts.bold : fonts.display, fontSize: 22, lineHeight: 26, color: selected ? colors.primaryInk : colors.muted }}>{l === 'am' ? 'ሰላም' : 'Hello'}</T>
            <T variant="small" color={selected ? colors.ink : colors.muted} style={{ fontFamily: fonts.medium, fontSize: 13, lineHeight: 18 }}>{LANGUAGE_NAMES[l]}</T>
          </PressableScale>
        );
      })}
    </View>
  );
}

export default function Welcome() {
  const { t } = useTranslation();
  const router = useRouter();
  const { language, currency, setLanguage, finishOnboarding } = usePrefs();
  const current = language ?? pickLanguage(getLocales()[0]?.languageTag);

  return (
    <View style={{ flex: 1, backgroundColor: '#0B192C' }}>
      <ScrollView bounces={false} showsVerticalScrollIndicator={false} contentContainerStyle={{ flexGrow: 1 }}>
        {/* the hero: deep navy, soft rings, and three places drifting about */}
        <View style={{ height: HERO_HEIGHT, overflow: 'hidden' }}>
          <LinearGradient colors={['#0B192C', '#17305A', '#0B192C']} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }} />
          <View style={{ position: 'absolute', width: 420, height: 420, borderRadius: 210, borderWidth: 1, borderColor: 'rgba(255,255,255,0.07)', right: -150, top: -120 }} />
          <View style={{ position: 'absolute', width: 300, height: 300, borderRadius: 150, borderWidth: 1, borderColor: 'rgba(255,255,255,0.08)', right: -90, top: -60 }} />
          <View style={{ position: 'absolute', width: 520, height: 520, borderRadius: 260, backgroundColor: 'rgba(224,90,71,0.16)', left: -300, bottom: -330 }} />
          {[{ l: 22, t: 150, s: 10, c: colors.gold }, { l: 330, t: 232, s: 8, c: colors.primary }, { l: 250, t: 118, s: 6, c: '#FFFFFF' }, { l: 40, t: 300, s: 7, c: '#FFFFFF' }].map((d, i) => (
            <View key={i} style={{ position: 'absolute', left: d.l, top: d.t, width: d.s, height: d.s, backgroundColor: d.c, opacity: 0.55, transform: [{ rotate: '45deg' }] }} />
          ))}

          <SafeAreaView edges={['top']} style={{ paddingHorizontal: spacing.lg }}>
            <Pop>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, marginTop: spacing.sm }}>
                <Image source={require('../assets/icon.png')} accessibilityLabel="BookDan" style={{ width: 38, height: 38, borderRadius: 11 }} />
                <T style={{ fontFamily: fonts.display, fontSize: 24, lineHeight: 28, color: '#FFFFFF', letterSpacing: 0.3 }}>BookDan</T>
              </View>
            </Pop>
          </SafeAreaView>

          <Floating delay={0} style={{ left: 18, top: 92 }}><PlaceCard icon="business-outline" from="#E05A47" to="#F5A524" name={t('welcome.city_addis')} line={t('welcome.city_addis_line')} tilt="-6deg" /></Floating>
          <Floating delay={500} distance={9} duration={3000} style={{ right: 14, top: 62 }}><PlaceCard icon="water-outline" from="#0F6B4F" to="#14566E" name={t('welcome.city_bahir')} line={t('welcome.city_bahir_line')} tilt="5deg" /></Floating>
          <Floating delay={900} distance={6} duration={2400} style={{ left: 104, top: 208 }}><PlaceCard icon="triangle-outline" from="#7C3AED" to="#E05A47" name={t('welcome.city_lalibela')} line={t('welcome.city_lalibela_line')} tilt="-2deg" /></Floating>
        </View>

        {/* the sheet: what the app is, and the two choices */}
        <View style={{ flex: 1, marginTop: -34, backgroundColor: colors.background, borderTopLeftRadius: 32, borderTopRightRadius: 32, padding: spacing.lg, paddingTop: spacing.xl, gap: spacing.lg }}>
          <Reveal index={1}>
            <T variant="h1" style={{ fontSize: 30, lineHeight: 36 }}>{t('welcome.title')}</T>
            <T color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('welcome.subtitle')}</T>
          </Reveal>

          <Reveal index={2}>
            <View style={{ flexDirection: 'row', gap: spacing.sm, paddingVertical: spacing.md, paddingHorizontal: spacing.sm, borderRadius: radius.lg, backgroundColor: colors.surface, ...shadow.card }}>
              <Point icon="shield-checkmark" label={t('welcome.point_verified')} ink="#0F6B4F" soft="#E3F2EC" tilt="-6deg" />
              <Point icon="wallet" label={t('welcome.point_pay')} ink="#B26A00" soft="#FFF1D6" tilt="4deg" />
              <Point icon="refresh-circle" label={t('welcome.point_cancel')} ink="#1D5FA8" soft="#E4EEFA" tilt="-3deg" />
            </View>
          </Reveal>

          <Reveal index={3}>
            <T variant="label" color={colors.muted}>{t('welcome.language').toUpperCase()}</T>
            <View style={{ marginTop: spacing.sm }}>
              <LanguageSwitch current={current} onPick={setLanguage} />
            </View>

            <T variant="label" color={colors.muted} style={{ marginTop: spacing.lg }}>{t('welcome.currency').toUpperCase()}</T>
            <PressableScale
              onPress={() => router.push('/currency')}
              accessibilityRole="button"
              accessibilityLabel={t('currency.title')}
              scaleTo={0.985}
              style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, marginTop: spacing.sm, padding: spacing.sm + 2, paddingRight: spacing.md, borderRadius: radius.lg, backgroundColor: colors.surface, ...shadow.card }}
            >
              {/* the currency's own sign, big, on a gradient tile */}
              <LinearGradient colors={['#14233B', '#1F3A66']} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ width: 52, height: 52, borderRadius: 16, alignItems: 'center', justifyContent: 'center' }}>
                <T style={{ fontFamily: fonts.display, fontSize: 24, lineHeight: 28, color: '#FFFFFF' }}>{currency === 'ETB' ? 'Br' : currencyInfo(currency).symbol ?? currency}</T>
              </LinearGradient>
              <View style={{ flex: 1 }}>
                <T variant="h3" numberOfLines={1}>{currency === 'ETB' ? t('currency.etb_name') : currencyInfo(currency).name}</T>
                <T variant="small" color={colors.muted}>{currency}</T>
              </View>
              <View style={{ width: 32, height: 32, borderRadius: 16, backgroundColor: colors.fill, alignItems: 'center', justifyContent: 'center' }}>
                <Ionicons name="swap-horizontal" size={17} color={colors.charcoal} />
              </View>
            </PressableScale>
          </Reveal>

          <Reveal index={4}>
            <PressableScale
              accessibilityRole="button"
              accessibilityLabel={t('welcome.start')}
              scaleTo={0.97}
              onPress={() => {
                setLanguage(current);
                finishOnboarding();
                router.replace('/');
              }}
              style={{ borderRadius: radius.pill, overflow: 'hidden', ...shadow.button, shadowOpacity: 0.32, shadowRadius: 16 }}
            >
              <LinearGradient colors={['#EA6A56', '#C74430']} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ minHeight: 58, flexDirection: 'row', alignItems: 'center', justifyContent: 'center', gap: spacing.sm }}>
                <T variant="h3" color="#FFFFFF" style={{ fontSize: 17, letterSpacing: 0.3 }}>{t('welcome.start')}</T>
                <View style={{ width: 28, height: 28, borderRadius: 14, backgroundColor: 'rgba(255,255,255,0.22)', alignItems: 'center', justifyContent: 'center' }}>
                  <Ionicons name="arrow-forward" size={17} color="#FFFFFF" />
                </View>
              </LinearGradient>
            </PressableScale>
          </Reveal>
          <SafeAreaView edges={['bottom']} />
        </View>
      </ScrollView>
    </View>
  );
}
