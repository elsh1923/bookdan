import { Ionicons } from '@expo/vector-icons';
import { useState } from 'react';
import { Modal, Pressable, View } from 'react-native';
import { HOTEL_PROBLEM_REASONS, type HotelProblemReason } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, PressableScale, T, colors, radius, spacing } from '@bookdan/ui';

/**
 * Bottom sheet for the two things a hotel can do when it cannot keep a confirmed booking: cancel it (the guest is refunded in full)
 * or report that it cannot host the guest (BookDan support finds another place). A reason is required.
 */
export function HotelProblemSheet({ visible, busy, title, note, confirmLabel, error, onClose, onConfirm }: {
  visible: boolean;
  busy: boolean;
  title: string;
  note: string;
  confirmLabel: string;
  error?: string | null;
  onClose: () => void;
  onConfirm: (reason: HotelProblemReason) => void;
}) {
  const { t } = useTranslation();
  const [reason, setReason] = useState<HotelProblemReason | null>(null);

  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={onClose}>
      <Pressable style={{ flex: 1, backgroundColor: 'rgba(10,20,40,0.45)' }} onPress={busy ? undefined : onClose} />
      <View style={{ backgroundColor: colors.surface, borderTopLeftRadius: radius.lg, borderTopRightRadius: radius.lg, padding: spacing.lg, width: '100%', maxWidth: 640, alignSelf: 'center' }}>
        <T variant="displaySm">{title}</T>
        <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{note}</T>

        <View style={{ marginTop: spacing.md }}>
          {HOTEL_PROBLEM_REASONS.map((r) => {
            const selected = reason === r;
            return (
              <PressableScale
                key={r}
                scaleTo={0.98}
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
                <T variant="h3" color={selected ? colors.primaryInk : colors.ink} style={{ flex: 1 }}>{t(`partner.hc_${r}`)}</T>
              </PressableScale>
            );
          })}
        </View>
        {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.md }}>✕ {error}</T> : null}

        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.lg }}>
          <Button variant="secondary" title={t('common.cancel')} onPress={onClose} disabled={busy} style={{ flex: 1 }} />
          <Button title={confirmLabel} onPress={() => reason && onConfirm(reason)} disabled={!reason} loading={busy} style={{ flex: 2 }} />
        </View>
      </View>
    </Modal>
  );
}
