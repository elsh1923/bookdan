import { Ionicons } from '@expo/vector-icons';
import { Tabs } from 'expo-router';
import type { ColorValue } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { isAwaitingAnswer } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { colors, fonts } from '@bookdan/ui';
import { ApiError } from '@bookdan/api-client';
import { useEarnings, useInbox } from '../../src/queries';
import { useOfflineSync } from '../../src/useOfflineSync';

const icon = (name: keyof typeof Ionicons.glyphMap) => ({ color, size }: { color: ColorValue; size: number }) => (
  <Ionicons name={name} size={size} color={color} />
);

export default function TabsLayout() {
  useOfflineSync();
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const inbox = useInbox();
  // Money is for owners: for a receptionist the server says FORBIDDEN and the tab disappears.
  const earnings = useEarnings();
  const owner = !(earnings.error instanceof ApiError && earnings.error.code === 'FORBIDDEN');
  // A red badge on the tab: how many bookings are waiting for an answer.
  const waiting = inbox.data?.items.filter(isAwaitingAnswer).length ?? 0;

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        animation: 'fade',
        sceneStyle: { backgroundColor: colors.background },
        tabBarActiveTintColor: colors.primaryInk,
        tabBarInactiveTintColor: colors.muted,
        tabBarLabelStyle: { fontFamily: fonts.medium, fontSize: 11, marginBottom: 2 },
        tabBarStyle: { height: 70 + insets.bottom, paddingBottom: Math.max(insets.bottom, 8), paddingTop: 8, backgroundColor: colors.surface, borderTopColor: colors.line },
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: t('partner.tab_bookings'),
          tabBarIcon: icon('file-tray-full-outline'),
          tabBarBadge: waiting > 0 ? waiting : undefined,
          tabBarBadgeStyle: { backgroundColor: colors.danger },
        }}
      />
      <Tabs.Screen name="checkin" options={{ title: t('partner.tab_checkin'), tabBarIcon: icon('qr-code-outline') }} />
      <Tabs.Screen name="rooms" options={{ title: t('partner.tab_rooms'), tabBarIcon: icon('grid-outline') }} />
      <Tabs.Screen name="earnings" options={{ title: t('partner.tab_earnings'), tabBarIcon: icon('wallet-outline'), href: owner ? undefined : null }} />
      <Tabs.Screen name="settings" options={{ title: t('partner.tab_settings'), tabBarIcon: icon('settings-outline') }} />
    </Tabs>
  );
}
