import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { useFocusEffect } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useCallback, useState, type ReactNode } from 'react';
import { Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Reveal, T, colors, dark, gradients, radius, spacing } from '@bookdan/ui';

/**
 * The top of every partner screen: a deep navy band with rounded lower corners (the same look as the guest app's home), the hotel or section in small
 * capitals above a large title, and room for a row of numbers underneath. Screens opened from another screen use the compact form with a back button.
 */
export function HeaderBand({ eyebrow, title, onBack, right, children, compact }: {
  eyebrow?: string;
  title: string;
  onBack?: () => void;
  right?: ReactNode;
  children?: ReactNode;
  compact?: boolean;
}) {
  const insets = useSafeAreaInsets();
  // The status bar text is light only while this screen is the one in front.
  const [focused, setFocused] = useState(true);
  useFocusEffect(useCallback(() => {
    setFocused(true);
    return () => setFocused(false);
  }, []));

  return (
    <LinearGradient
      colors={gradients.header}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={{ paddingTop: insets.top + spacing.sm, paddingBottom: compact ? spacing.md : spacing.lg, paddingHorizontal: spacing.lg, borderBottomLeftRadius: 28, borderBottomRightRadius: 28 }}
    >
      {focused ? <StatusBar style="light" /> : null}
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, minHeight: 44 }}>
        {onBack ? (
          <Pressable
            accessibilityRole="button"
            accessibilityLabel="Back"
            onPress={onBack}
            style={{ width: 40, height: 40, borderRadius: 20, backgroundColor: 'rgba(255,255,255,0.14)', alignItems: 'center', justifyContent: 'center' }}
          >
            <Ionicons name="arrow-back" size={22} color="#FFFFFF" />
          </Pressable>
        ) : null}
        <View style={{ flex: 1 }}>
          {eyebrow ? <T variant="label" color={dark.accent} numberOfLines={1}>{eyebrow.toUpperCase()}</T> : null}
          {compact ? <T variant="h2" color="#FFFFFF" numberOfLines={1}>{title}</T> : null}
        </View>
        {right}
      </View>
      {compact ? null : (
        <Reveal>
          <T variant="display" color="#FFFFFF" style={{ marginTop: spacing.xs }}>{title}</T>
        </Reveal>
      )}
      {children}
    </LinearGradient>
  );
}

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
