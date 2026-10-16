import {
  NotoSansEthiopic_400Regular,
  NotoSansEthiopic_500Medium,
  NotoSansEthiopic_700Bold,
  useFonts,
} from '@expo-google-fonts/noto-sans-ethiopic';
import { PlayfairDisplay_500Medium, PlayfairDisplay_700Bold } from '@expo-google-fonts/playfair-display';
import { QueryClientProvider } from '@tanstack/react-query';
import { getLocales } from 'expo-localization';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useState } from 'react';
import { ActivityIndicator, View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { i18next, initI18n, pickLanguage } from '@bookdan/i18n';
import { DisplayFontContext, colors, fonts } from '@bookdan/ui';
import { queryClient } from '../src/api';
import { useAuth } from '../src/auth';
import { usePushAlerts } from '../src/notifications';
import { UpdateBanner } from '../src/UpdateBanner';
import { usePrefs } from '../src/store';

export default function RootLayout() {
  const [fontsLoaded] = useFonts({
    NotoSansEthiopic_400Regular,
    NotoSansEthiopic_500Medium,
    NotoSansEthiopic_700Bold,
    PlayfairDisplay_500Medium,
    PlayfairDisplay_700Bold,
  });
  const prefsReady = usePrefs((s) => s.hydrated);
  const language = usePrefs((s) => s.language);
  const token = useAuth((s) => s.token);
  usePushAlerts(); // registers this phone for new-booking alerts and opens a booking when its alert is tapped

  // Wait for the stored session before choosing between login and the inbox, so neither flashes wrongly.
  const [authReady, setAuthReady] = useState(useAuth.persist.hasHydrated());
  useEffect(() => useAuth.persist.onFinishHydration(() => setAuthReady(true)), []);

  // Propose the phone language on first launch; staff can change it in Settings.
  const effective = language ?? pickLanguage(getLocales()[0]?.languageTag);
  if (!i18next.isInitialized) initI18n(effective);
  useEffect(() => {
    initI18n(effective);
  }, [effective]);

  if (!fontsLoaded || !prefsReady || !authReady) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: colors.background }}>
        <ActivityIndicator color={colors.primary} />
      </View>
    );
  }

  return (
    <SafeAreaProvider>
      <DisplayFontContext.Provider value={effective === 'am' ? fonts.bold : fonts.display}>
        <QueryClientProvider client={queryClient}>
          <StatusBar style="dark" />
          <UpdateBanner />
          <View style={{ flex: 1, alignItems: 'center', backgroundColor: colors.background }}>
            <View style={{ flex: 1, width: '100%', maxWidth: 640 }}>
              <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.background }, animation: 'slide_from_right', animationDuration: 280 }}>
                {/* Signed out: only the login screen. Signed in: only the app. The navigator redirects by itself. */}
                <Stack.Protected guard={!!token}>
                  <Stack.Screen name="(tabs)" />
                  <Stack.Screen name="booking/[id]" />
                </Stack.Protected>
                <Stack.Protected guard={!token}>
                  <Stack.Screen name="login" options={{ animation: 'fade' }} />
                </Stack.Protected>
              </Stack>
            </View>
          </View>
        </QueryClientProvider>
      </DisplayFontContext.Provider>
    </SafeAreaProvider>
  );
}
