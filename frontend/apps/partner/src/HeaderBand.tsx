import { Ionicons } from '@expo/vector-icons';
import { View } from 'react-native';
import { T, colors, dark, radius, spacing } from '@bookdan/ui';

/**
 * The top of every partner screen. It is the same header the guest app uses (ScreenHeader in the shared UI package), so both apps start every page the
 * same way: a deep navy band with rounded lower corners, the hotel or section in small capitals above a large title, and room for a row of numbers
 * underneath. Screens opened from another screen use the compact form with a back button.
 */
export { ScreenHeader as HeaderBand } from '@bookdan/ui';

/** A number on the navy band: how many, and what. A tile with something to do (alert) glows terracotta. */
export function StatTile({ icon, value, label, alert }: { icon: keyof typeof Ionicons.glyphMap; value: number | string; label: string; alert?: boolean }) {
  const lit = !!alert && Number(value) > 0;
  return (
    <View
      style={{
        flex: 1, padding: spacing.md, borderRadius: radius.md, gap: 2,
        backgroundColor: lit ? colors.primary : 'rgba(255,255,255,0.10)',
        borderWidth: 1, borderColor: lit ? colors.primary : 'rgba(255,255,255,0.14)',
      }}
    >
      <Ionicons name={icon} size={18} color={lit ? '#FFFFFF' : dark.accent} />
      <T variant="h1" color="#FFFFFF" style={{ marginTop: spacing.xs }}>{value}</T>
      <T variant="label" color={lit ? '#FFFFFF' : dark.textSoft} numberOfLines={2}>{label}</T>
    </View>
  );
}
