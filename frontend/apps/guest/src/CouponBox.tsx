import { Ionicons } from '@expo/vector-icons';
import { useState } from 'react';
import { Pressable, TextInput, View } from 'react-native';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, fonts, MIN_TOUCH, radius, spacing } from '@bookdan/ui';

/**
 * "Have a coupon?" at checkout. The server decides if a code works: onApply returns an error message to show, or null when the code
 * was accepted. An accepted code shows as a green line with the saving and a way to remove it.
 */
export function CouponBox({ applied, saving, onApply, onRemove }: { applied: string | null; saving: string | null; onApply: (code: string) => Promise<string | null>; onRemove: () => void }) {
  const { t } = useTranslation();
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function apply() {
    if (!code.trim()) return;
    setBusy(true);
    setError(null);
    const problem = await onApply(code.trim().toUpperCase());
    setBusy(false);
    if (problem) setError(problem);
    else setCode('');
  }

  if (applied) {
    return (
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.successSoft }}>
        <Ionicons name="pricetag" size={20} color={colors.success} />
        <View style={{ flex: 1 }}>
          <T variant="h3" color={colors.success}>{t('checkout.coupon_applied', { code: applied })}</T>
          {saving ? <T variant="small" color={colors.success}>{t('checkout.coupon_saved', { amount: saving })}</T> : null}
        </View>
        <Pressable accessibilityRole="button" onPress={onRemove} hitSlop={8} style={{ minHeight: 44, justifyContent: 'center' }}>
          <T variant="small" color={colors.primaryInk}>{t('checkout.coupon_remove')}</T>
        </Pressable>
      </View>
    );
  }
  return (
    <View>
      <T variant="h3">{t('checkout.coupon_title')}</T>
      <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
        <TextInput
          accessibilityLabel={t('checkout.coupon_title')}
          value={code}
          onChangeText={(v) => setCode(v.toUpperCase())}
          autoCapitalize="characters"
          autoCorrect={false}
          placeholder={t('checkout.coupon_placeholder')}
          placeholderTextColor={colors.muted}
          maxLength={20}
          style={{ flex: 1, minHeight: MIN_TOUCH, paddingHorizontal: spacing.lg, borderRadius: radius.md, borderWidth: 1, borderColor: error ? colors.danger : colors.line, backgroundColor: colors.surface, fontFamily: fonts.regular, fontSize: 16, color: colors.ink }}
        />
        <Button title={t('checkout.coupon_apply')} variant="secondary" onPress={apply} loading={busy} disabled={!code.trim()} />
      </View>
      {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.xs }}>✕ {error}</T> : <T variant="small" color={colors.muted} style={{ marginTop: spacing.xs }}>{t('checkout.coupon_online')}</T>}
    </View>
  );
}
