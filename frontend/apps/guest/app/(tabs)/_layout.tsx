import { Ionicons } from '@expo/vector-icons';
import { Redirect, Tabs } from 'expo-router';
import type { ColorValue } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useTranslation } from '@bookdan/i18n';
import { colors, dark, fonts } from '@bookdan/ui';
import { usePrefs } from '../../src/store';

const icon = (name: keyof typeof Ionicons.glyphMap) => ({ color, size }: { color: ColorValue; size: number }) => (
  <Ionicons name={name} size={size} color={color} />
);

export default function TabsLayout() {
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const onboarded = usePrefs((s) => s.onboarded);
  if (!onboarded) return <Redirect href="/welcome" />;

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        animation: 'fade',
        sceneStyle: { backgroundColor: colors.background },
        // Deep navy bar with a light-blue active tab, on every tab screen.
        tabBarActiveTintColor: dark.accent,
        tabBarInactiveTintColor: dark.muted,
        tabBarLabelStyle: { fontFamily: fonts.medium, fontSize: 11, marginBottom: 2 },
        tabBarStyle: { height: 70 + insets.bottom, paddingBottom: Math.max(insets.bottom, 8), paddingTop: 8, backgroundColor: dark.surface, borderTopColor: dark.line },
      }}
    >
      <Tabs.Screen name="index" options={{ title: t('tabs.search'), tabBarIcon: icon('home-outline') }} />
      <Tabs.Screen name="bookings" options={{ href: null }} />
      <Tabs.Screen name="wishlist" options={{ title: t('tabs.wishlists'), tabBarIcon: icon('heart-outline') }} />
      <Tabs.Screen name="profile" options={{ title: t('tabs.profile'), tabBarIcon: icon('person-outline') }} />
    </Tabs>
  );
}
