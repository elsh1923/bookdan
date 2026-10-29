import {
  NotoSansEthiopic_400Regular,
  NotoSansEthiopic_500Medium,
  NotoSansEthiopic_700Bold,
  useFonts,
} from '@expo-google-fonts/noto-sans-ethiopic';
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client';
import { getLocales } from 'expo-localization';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect } from 'react';
import { View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { i18next, initI18n, pickLanguage } from '@bookdan/i18n';
import { DisplayFontContext, colors, fonts } from '@bookdan/ui';
import { PERSISTED_QUERY_KEYS, queryClient, queryPersister } from '../src/api';
import { usePaymentRecovery } from '../src/payment';
import { usePushUpdates } from '../src/notifications';
import { DisplaySync } from '../src/DisplaySync';
import { SavedSync } from '../src/SavedSync';
import { UpdateBanner } from '../src/UpdateBanner';
import { LaunchScreen } from '../src/LaunchScreen';
import { usePrefs } from '../src/store';

export default function RootLayout() {
  const [fontsLoaded] = useFonts({
    NotoSansEthiopic_400Regular,
    NotoSansEthiopic_500Medium,
    NotoSansEthiopic_700Bold,
    CormorantGaramond_600SemiBold: require('@expo-google-fonts/cormorant-garamond/600SemiBold/CormorantGaramond_600SemiBold.ttf'),
    CormorantGaramond_700Bold: require('@expo-google-fonts/cormorant-garamond/700Bold/CormorantGaramond_700Bold.ttf'),
  });
  usePushUpdates();
  usePaymentRecovery(); // an interrupted payment takes the guest back to its booking on the next launch
  const hydrated = usePrefs((s) => s.hydrated);
  const language = usePrefs((s) => s.language);

  // Propose the phone language on first launch; the guest can change it with one tap.
  const effective = language ?? pickLanguage(getLocales()[0]?.languageTag);
  // Initialise once during render so the first paint is translated; later language
  // changes happen in the effect, never during render.
  if (!i18next.isInitialized) initI18n(effective);
  useEffect(() => {
    initI18n(effective);
  }, [effective]);

  if (!fontsLoaded || !hydrated) {
    return <LaunchScreen />;
  }

  return (
    <SafeAreaProvider>
      <DisplayFontContext.Provider value={effective === 'am' ? fonts.bold : fonts.display}>
      <PersistQueryClientProvider
        client={queryClient}
        persistOptions={{
          persister: queryPersister,
          maxAge: 24 * 3_600_000,
          buster: '6', // bump when the cached shapes change (6: ID photo flags on the profile; 5: ... room photos, reply time, breakfast and cheaper-rate flags, directions)
          dehydrateOptions: {
            shouldDehydrateQuery: (q) => q.state.status === 'success' && PERSISTED_QUERY_KEYS.has(String(q.queryKey[0])),
          },
        }}
      >
        <StatusBar style="dark" />
          <UpdateBanner />
          <DisplaySync />
          <SavedSync />
        <View style={{ flex: 1, alignItems: 'center', backgroundColor: colors.background }}>
          <View style={{ flex: 1, width: '100%', maxWidth: 640 }}>
        <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.background }, animation: 'slide_from_right', animationDuration: 280 }}>
          <Stack.Screen name="(tabs)" />
          <Stack.Screen name="welcome" options={{ gestureEnabled: false }} />
          <Stack.Screen name="search" options={{ presentation: 'modal', animation: 'slide_from_bottom' }} />
          <Stack.Screen name="results" />
          <Stack.Screen name="property/[id]" />
          <Stack.Screen name="signin" options={{ presentation: 'modal' }} />
          <Stack.Screen name="checkout" />
          <Stack.Screen name="booking/[id]" />
          <Stack.Screen name="voucher/[id]" />
          <Stack.Screen name="guest-details" options={{ presentation: 'modal' }} />
          <Stack.Screen name="mock-gateway/[session]" options={{ presentation: 'modal' }} />
          <Stack.Screen name="payment-return" />
          <Stack.Screen name="change-dates" options={{ presentation: 'modal' }} />
          <Stack.Screen name="support/index" />
          <Stack.Screen name="support/new" options={{ presentation: 'modal' }} />
          <Stack.Screen name="support/[id]" />
          <Stack.Screen name="support/dispute" options={{ presentation: 'modal' }} />
        </Stack>
          </View>
        </View>
      </PersistQueryClientProvider>
      </DisplayFontContext.Provider>
    </SafeAreaProvider>
  );
}
