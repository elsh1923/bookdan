import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import React, { createContext, useContext } from 'react';
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  TextProps,
  View,
  useWindowDimensions,
  ViewStyle,
  StyleProp,
} from 'react-native';
import { colors, fonts, gradients, MIN_TOUCH, radius, shadow, spacing } from './theme';

type Variant = 'display' | 'displaySm' | 'h1' | 'h2' | 'h3' | 'body' | 'small' | 'label';

/** Font used by the serif display variants. The app switches it to the Ethiopic font for Amharic. */
export const DisplayFontContext = createContext<string>(fonts.display);

const textStyles: Record<Variant, { fontSize: number; lineHeight: number; fontFamily: string; letterSpacing?: number }> = {
  display: { fontSize: 32, lineHeight: 42, fontFamily: fonts.display },
  displaySm: { fontSize: 24, lineHeight: 32, fontFamily: fonts.display },
  h1: { fontSize: 26, lineHeight: 36, fontFamily: fonts.bold },
  h2: { fontSize: 20, lineHeight: 30, fontFamily: fonts.bold },
  h3: { fontSize: 16, lineHeight: 24, fontFamily: fonts.medium },
  body: { fontSize: 15, lineHeight: 24, fontFamily: fonts.regular },
  small: { fontSize: 13, lineHeight: 20, fontFamily: fonts.regular },
  label: { fontSize: 12, lineHeight: 18, fontFamily: fonts.medium, letterSpacing: 0.3 },
};

export function T({
  variant = 'body',
  color = colors.ink,
  style,
  ...rest
}: TextProps & { variant?: Variant; color?: string }) {
  // Phones narrower than ~360dp (small Androids) get slightly smaller type.
  const { width } = useWindowDimensions();
  const k = width < 360 ? 0.92 : 1;
  const displayFont = useContext(DisplayFontContext);
  const base = textStyles[variant];
  const family = variant === 'display' || variant === 'displaySm' ? displayFont : base.fontFamily;
  return (
    <Text
      // Respect the system text size, but cap it so layouts do not break at the largest setting.
      maxFontSizeMultiplier={1.3}
      {...rest}
      style={[{ ...base, fontFamily: family, fontSize: base.fontSize * k, lineHeight: base.lineHeight * k }, { color }, style]}
    />
  );
}

export function Button({
  title,
  onPress,
  variant = 'primary',
  loading,
  disabled,
  icon,
  style,
}: {
  title: string;
  onPress: () => void;
  variant?: 'primary' | 'secondary' | 'ghost';
  loading?: boolean;
  disabled?: boolean;
  icon?: keyof typeof Ionicons.glyphMap;
  style?: StyleProp<ViewStyle>;
}) {
  const primary = variant === 'primary';
  const fg = primary ? colors.onPrimary : colors.primaryInk;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: !!disabled || !!loading }}
      onPress={onPress}
      disabled={disabled || loading}
      style={({ pressed }) => [
        styles.button,
        variant === 'secondary' && { backgroundColor: colors.surface, borderWidth: 1.5, borderColor: colors.primary },
        primary && !disabled && shadow.button,
        { opacity: disabled ? 0.5 : pressed ? 0.88 : 1 },
        style,
      ]}
    >
      {primary ? (
        <LinearGradient
          colors={gradients.primary}
          start={{ x: 0, y: 0 }}
          end={{ x: 1, y: 1 }}
          style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, borderRadius: radius.md }}
        />
      ) : null}
      {loading ? (
        <ActivityIndicator color={fg} />
      ) : (
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
          {icon ? <Ionicons name={icon} size={20} color={fg} /> : null}
          <T variant="h3" color={fg} style={{ textAlign: 'center' }}>{title}</T>
        </View>
      )}
    </Pressable>
  );
}

export function Chip({
  label,
  selected,
  onPress,
}: {
  label: string;
  selected?: boolean;
  onPress?: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected: !!selected }}
      onPress={onPress}
      style={[
        styles.chip,
        selected && { backgroundColor: colors.primarySoft, borderColor: colors.primary },
      ]}
    >
      <T variant="small" color={selected ? colors.primaryInk : colors.inkSoft} style={selected ? { fontFamily: fonts.medium } : undefined}>
        {selected ? '✓ ' : ''}{label}
      </T>
    </Pressable>
  );
}

/** Status is never colour only: always icon/label plus colour. */
export function Badge({
  label,
  tone = 'success',
  icon,
}: {
  label: string;
  tone?: 'success' | 'warning' | 'danger' | 'neutral';
  icon?: string;
}) {
  const map = {
    success: [colors.successSoft, colors.success],
    warning: [colors.warningSoft, colors.warning],
    danger: [colors.dangerSoft, colors.danger],
    neutral: [colors.line, colors.inkSoft],
  } as const;
  const [bg, fg] = map[tone];
  return (
    <View style={[styles.badge, { backgroundColor: bg }]}>
      <T variant="label" color={fg}>{icon ? `${icon} ` : ''}{label}</T>
    </View>
  );
}

export function EmptyState({
  icon,
  title,
  body,
  actionLabel,
  onAction,
}: {
  icon: string;
  title: string;
  body?: string;
  actionLabel?: string;
  onAction?: () => void;
}) {
  return (
    <View style={styles.empty}>
      <T style={{ fontSize: 44, lineHeight: 56 }}>{icon}</T>
      <T variant="h2" style={{ textAlign: 'center', marginTop: spacing.md }}>{title}</T>
      {body ? <T color={colors.inkSoft} style={{ textAlign: 'center', marginTop: spacing.xs }}>{body}</T> : null}
      {actionLabel && onAction ? (
        <Button title={actionLabel} onPress={onAction} style={{ marginTop: spacing.xl, alignSelf: 'stretch' }} />
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  button: {
    minHeight: MIN_TOUCH,
    paddingHorizontal: spacing.xl,
    borderRadius: radius.md,
    alignItems: 'center',
    justifyContent: 'center',
  },
  chip: {
    minHeight: 40,
    paddingHorizontal: spacing.lg,
    borderRadius: radius.pill,
    borderWidth: 1,
    borderColor: colors.line,
    backgroundColor: colors.surface,
    alignItems: 'center',
    justifyContent: 'center',
  },
  badge: {
    alignSelf: 'flex-start',
    paddingHorizontal: spacing.sm,
    paddingVertical: 2,
    borderRadius: radius.sm,
  },
  empty: { alignItems: 'center', padding: spacing.xxl, paddingTop: 64 },
});
