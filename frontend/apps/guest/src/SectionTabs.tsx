import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { useEffect, useState } from 'react';
import { ScrollView, View } from 'react-native';
import Animated, { useAnimatedStyle, useSharedValue, withSpring } from 'react-native-reanimated';
import { PressableScale, T, colors, gradients, radius, shadow, spacing } from '@bookdan/ui';

export interface SectionTab<K extends string> {
  key: K;
  label: string;
  icon: keyof typeof Ionicons.glyphMap;
}

/**
 * A row of tabs with an icon each. A terracotta pill slides (with a soft spring) to the open tab, the open tab's icon and
 * label turn white, and every tab shrinks slightly under the finger. The pill is measured from the real tab sizes, so it
 * fits any language.
 */
export function SectionTabs<K extends string>({ tabs, value, onChange }: { tabs: SectionTab<K>[]; value: K; onChange: (key: K) => void }) {
  const [boxes, setBoxes] = useState<Record<string, { x: number; w: number }>>({});
  const x = useSharedValue(0);
  const w = useSharedValue(0);
  const shown = useSharedValue(0);
  const box = boxes[value];

  useEffect(() => {
    if (!box) return;
    if (shown.value === 0) {
      // First time: place the pill without animating from the left edge.
      x.value = box.x;
      w.value = box.w;
      shown.value = 1;
    } else {
      x.value = withSpring(box.x, { damping: 18, stiffness: 190 });
      w.value = withSpring(box.w, { damping: 18, stiffness: 190 });
    }
  }, [box, x, w, shown]);

  const pill = useAnimatedStyle(() => ({ transform: [{ translateX: x.value }], width: w.value, opacity: shown.value }));

  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingVertical: spacing.lg }}>
      <View style={{ flexDirection: 'row', gap: spacing.sm, padding: 4, borderRadius: radius.pill, backgroundColor: colors.fill }}>
        <Animated.View pointerEvents="none" style={[{ position: 'absolute', top: 4, bottom: 4, left: 0, borderRadius: radius.pill, overflow: 'hidden' }, shadow.button, pill]}>
          <LinearGradient colors={gradients.primary} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ flex: 1 }} />
        </Animated.View>
        {tabs.map((tab) => {
          const selected = tab.key === value;
          return (
            <View key={tab.key} onLayout={(e) => { const { x: bx, width } = e.nativeEvent.layout; setBoxes((b) => (b[tab.key]?.x === bx && b[tab.key]?.w === width ? b : { ...b, [tab.key]: { x: bx, w: width } })); }}>
              <PressableScale
                onPress={() => onChange(tab.key)}
                accessibilityRole="tab"
                accessibilityState={{ selected }}
                scaleTo={0.93}
                style={{ flexDirection: 'row', alignItems: 'center', gap: 6, minHeight: 40, paddingHorizontal: spacing.md }}
              >
                <Ionicons name={tab.icon} size={16} color={selected ? '#FFFFFF' : colors.inkSoft} />
                <T variant="small" color={selected ? '#FFFFFF' : colors.inkSoft} style={{ fontWeight: selected ? '700' : '500' }}>{tab.label}</T>
              </PressableScale>
            </View>
          );
        })}
      </View>
    </ScrollView>
  );
}
