import { TextInput, View, type TextInputProps } from 'react-native';
import { T, colors, fonts, MIN_TOUCH, radius, spacing } from '@bookdan/ui';

export function Field({
  label,
  hint,
  error,
  style,
  ...input
}: TextInputProps & { label: string; hint?: string; error?: string | null }) {
  return (
    <View style={{ marginTop: spacing.lg }}>
      <T variant="label" color={colors.muted}>{label.toUpperCase()}</T>
      <TextInput
        accessibilityLabel={label}
        placeholderTextColor={colors.muted}
        {...input}
        style={[{
          minHeight: MIN_TOUCH,
          marginTop: spacing.xs,
          paddingHorizontal: spacing.lg,
          borderRadius: radius.md,
          borderWidth: 1,
          borderColor: error ? colors.danger : colors.line,
          backgroundColor: colors.surface,
          fontFamily: fonts.regular,
          fontSize: 16,
          color: colors.ink,
        }, style]}
      />
      {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.xs }}>✕ {error}</T> : null}
      {!error && hint ? <T variant="small" color={colors.muted} style={{ marginTop: spacing.xs }}>{hint}</T> : null}
    </View>
  );
}
