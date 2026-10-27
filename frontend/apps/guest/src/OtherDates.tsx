import { Ionicons } from '@expo/vector-icons';
import { View } from 'react-native';
import type { AltWindow, SearchItem } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { PressableScale, T, colors, radius, shadow, spacing } from '@bookdan/ui';
import { price, shortDate } from './format';

/**
 * Hotels that are full for the dates the guest asked for, but have rooms on nearby dates or for part of the stay. Each stay is a tap away from
 * the hotel page with those dates. Nothing is held until the guest books, and the booking checks every night again.
 */
export function OtherDates({ hotels, months, onPick }: { hotels: SearchItem[]; months: string[]; onPick: (hotel: SearchItem, w: AltWindow) => void }) {
  const { t } = useTranslation();
  if (hotels.length === 0) return null;
  return (
    <View style={{ marginTop: spacing.xl }}>
      <T variant="displaySm">{t('results.other_dates_title')}</T>
      <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('results.other_dates_hint')}</T>
      {hotels.map((h) => (
        <View key={h.id} style={[{ marginTop: spacing.md, padding: spacing.md, borderRadius: radius.lg, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.line }, shadow.card]}>
          <T variant="h3" numberOfLines={1}>{h.name}</T>
          <T variant="small" color={colors.muted} numberOfLines={1}>{h.area} · {h.city}</T>
          <View style={{ gap: spacing.sm, marginTop: spacing.sm }}>
            {(h.windows ?? []).map((w) => (
              <PressableScale
                key={`${w.check_in}-${w.check_out}`}
                accessibilityRole="button"
                onPress={() => onPick(h, w)}
                scaleTo={0.98}
                style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.fill }}
              >
                <View style={{ flex: 1 }}>
                  <T variant="h3">{shortDate(w.check_in, months)} – {shortDate(w.check_out, months)}</T>
                  <T variant="label" color={colors.muted}>{t('common.night', { count: w.nights })}</T>
                </View>
                <T variant="small" color={colors.inkSoft}>{t('common.from')} {price(w.from_price_santim, w.currency)}</T>
                <Ionicons name="chevron-forward" size={18} color={colors.primaryInk} />
              </PressableScale>
            ))}
          </View>
        </View>
      ))}
    </View>
  );
}

