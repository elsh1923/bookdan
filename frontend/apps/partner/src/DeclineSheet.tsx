import { Ionicons } from '@expo/vector-icons';
import { useState } from 'react';
import { Modal, Pressable, View } from 'react-native';
import { DECLINE_REASONS, type DeclineReason } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';

/** Bottom sheet asking for the reason when a hotel declines. A reason is required. */
export function DeclineSheet({ visible, busy, onClose, onConfirm }: {
  visible: boolean;
  busy: boolean;
  onClose: () => void;
  onConfirm: (reason: DeclineReason) => void;
}) {
  const { t } = useTranslation();
  const [reason, setReason] = useState<DeclineReason | null>(null);

  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={onClose}>
      <Pressable style={{ flex: 1, backgroundColor: 'rgba(10,20,40,0.45)' }} onPress={busy ? undefined : onClose} />
      <View style={{ backgroundColor: colors.surface, borderTopLeftRadius: radius.lg, borderTopRightRadius: radius.lg, padding: spacing.lg, width: '100%', maxWidth: 640, alignSelf: 'center' }}>
        <T variant="displaySm">{t('partner.decline_title')}</T>
        <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.decline_note')}</T>

        <View style={{ marginTop: spacing.md }}>
          {DECLINE_REASONS.map((r) => {
            const selected = reason === r;
            return (
              <Pressable
                key={r}
                accessibilityRole="radio"
                accessibilityState={{ selected }}
                onPress={() => setReason(r)}
                style={{
                  minHeight: 56, flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.md,
                  marginTop: spacing.sm, borderRadius: radius.md, borderWidth: 1.5,
                  borderColor: selected ? colors.primary : colors.line, backgroundColor: selected ? colors.primarySoft : colors.surface,
                }}
              >
                <Ionicons name={selected ? 'radio-button-on' : 'radio-button-off'} size={22} color={selected ? colors.primary : colors.muted} />
                <T variant="h3" color={selected ? colors.primaryInk : colors.ink} style={{ flex: 1 }}>{t(`partner.reason_${r}`)}</T>
              </Pressable>
            );
          })}
        </View>

        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.lg }}>
          <Button variant="secondary" title={t('common.cancel')} onPress={onClose} disabled={busy} style={{ flex: 1 }} />
          <Button title={t('partner.decline_confirm')} onPress={() => reason && onConfirm(reason)} disabled={!reason} loading={busy} style={{ flex: 2 }} />
        </View>
      </View>
    </Modal>
  );
}
