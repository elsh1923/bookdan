import { Ionicons } from '@expo/vector-icons';
import { Pressable, View } from 'react-native';
import { T, colors, radius, shadow, spacing } from '@bookdan/ui';

/** A square-ish quick action: round tinted icon over a short label. Sit two or three in a row. */
export function ActionTile({ icon, label, onPress }: { icon: keyof typeof Ionicons.glyphMap; label: string; onPress: () => void }) {
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityLabel={label}
      style={({ pressed }) => [
        {
          flex: 1, minHeight: 88, alignItems: 'center', justifyContent: 'center', gap: spacing.sm,
          paddingVertical: spacing.md, paddingHorizontal: spacing.sm,
          borderRadius: radius.md, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.line,
          opacity: pressed ? 0.85 : 1,
        },
        shadow.card,
      ]}
    >
      <View style={{ width: 40, height: 40, borderRadius: 20, backgroundColor: colors.primarySoft, alignItems: 'center', justifyContent: 'center' }}>
        <Ionicons name={icon} size={20} color={colors.primaryInk} />
      </View>
      <T variant="small" color={colors.ink} style={{ textAlign: 'center' }} numberOfLines={2}>{label}</T>
    </Pressable>
  );
}
