import { Ionicons } from '@expo/vector-icons';
import { Pressable, View } from 'react-native';
import type { Currency, RoomOption } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, radius, spacing } from '@bookdan/ui';
import { price } from './format';
import { useDisplay } from './display';

export const optionKey = (o: Pick<RoomOption, 'meal' | 'refundable'>) => `${o.meal}/${o.refundable}`;

/**
 * The ways to book one room, as radio rows: "Room only" or "Room with breakfast", each with free cancellation or non-refundable
 * (cheaper). Every row shows its price per night; the chosen one is highlighted.
 */
export function RoomOptionPicker({ options, value, currency, onChange }: { options: RoomOption[]; value: string; currency: Currency; onChange: (o: RoomOption) => void }) {
  useDisplay((st) => st.version); // redraw when the display currency changes
  const { t } = useTranslation();
  return (
    <View style={{ marginTop: spacing.md, gap: spacing.sm }}>
      {options.map((o) => {
        const selected = optionKey(o) === value;
        return (
          <Pressable
            key={optionKey(o)}
            accessibilityRole="radio"
            accessibilityState={{ selected }}
            onPress={() => onChange(o)}
            style={{
              minHeight: 64, flexDirection: 'row', alignItems: 'center', gap: spacing.md, padding: spacing.md, borderRadius: radius.md, borderWidth: 1.5,
              borderColor: selected ? colors.primary : colors.line, backgroundColor: selected ? colors.primarySoft : colors.surface,
            }}
          >
            <Ionicons name={selected ? 'radio-button-on' : 'radio-button-off'} size={22} color={selected ? colors.primary : colors.muted} />
            <View style={{ flex: 1 }}>
              <T variant="h3" color={selected ? colors.primaryInk : colors.ink}>{t(o.meal === 'breakfast' ? 'property.option_breakfast' : 'property.option_room_only')}</T>
              {o.refundable
                ? <T variant="small" color={colors.success}>✓ {t('property.option_free_cancel')}</T>
                : <T variant="small" color={colors.danger}>{t('property.option_nonrefundable')} · {t('property.option_nonrefundable_note')}</T>}
            </View>
            <View style={{ alignItems: 'flex-end' }}>
              <T variant="h3" color={colors.primaryInk}>{price(o.price_santim, currency)}</T>
              <T variant="label" color={colors.muted}>{t('common.per_night')}</T>
            </View>
          </Pressable>
        );
      })}
    </View>
  );
}

/** What was booked and what happens on cancelling, in words, for the checkout and the booking. */
export function PolicyCard({ meal, refundable }: { meal: 'room_only' | 'breakfast'; refundable: boolean }) {
  const { t } = useTranslation();
  return (
    <View style={{ padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: refundable ? colors.line : colors.danger, backgroundColor: refundable ? colors.surface : colors.dangerSoft }}>
      <T variant="h3">{t(meal === 'breakfast' ? 'checkout.meal_breakfast' : 'checkout.meal_room_only')}</T>
      <T variant="label" color={colors.muted} style={{ marginTop: spacing.sm }}>{t('checkout.policy_title').toUpperCase()}</T>
      <T variant="small" color={refundable ? colors.inkSoft : colors.danger}>{t(refundable ? 'checkout.policy_refundable' : 'checkout.policy_nonrefundable')}</T>
    </View>
  );
}
