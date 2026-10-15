import { Ionicons } from '@expo/vector-icons';
import { Tabs } from 'expo-router';
import type { ColorValue } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { isAwaitingAnswer } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { colors, fonts } from '@bookdan/ui';
import { useInbox } from '../../src/queries';

const icon = (name: keyof typeof Ionicons.glyphMap) => ({ color, size }: { color: ColorValue; size: number }) => (
  <Ionicons name={name} size={size} color={color} />
);

export default function TabsLayout() {
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const inbox = useInbox();
  // A red badge on the tab: how many bookings are waiting for an answer.
  const waiting = inbox.data?.items.filter(isAwaitingAnswer).length ?? 0;

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
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
      <Tabs.Screen name="settings" options={{ title: t('partner.tab_settings'), tabBarIcon: icon('settings-outline') }} />
    </Tabs>
  );
}
