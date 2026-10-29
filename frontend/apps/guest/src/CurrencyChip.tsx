import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { Pressable } from 'react-native';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, spacing } from '@bookdan/ui';
import { useDisplay } from './display';

/** The currency prices are shown in, as a small button for a screen's top bar: it opens the Currency screen. `onDark` for the navy home header. */
export function CurrencyChip({ onDark }: { onDark?: boolean }) {
  const { t } = useTranslation();
  const router = useRouter();
  const shown = useDisplay((s) => s.currency);
  const ink = onDark ? '#FFFFFF' : colors.primaryInk;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('currency.title')}
      hitSlop={8}
      onPress={() => router.push('/currency')}
      style={{ flexDirection: 'row', alignItems: 'center', gap: 2, paddingHorizontal: spacing.sm, height: 36, borderRadius: 18, backgroundColor: onDark ? 'rgba(255,255,255,0.14)' : colors.primarySoft }}
    >
      <T variant="label" color={ink} style={{ fontWeight: '700' }}>{shown}</T>
      <Ionicons name="chevron-down" size={14} color={ink} />
    </Pressable>
  );
}
