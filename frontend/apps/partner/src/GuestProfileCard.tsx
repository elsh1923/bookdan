import { Ionicons } from '@expo/vector-icons';
import { useEffect, useState } from 'react';
import { View } from 'react-native';
import { ApiError } from '@bookdan/api-client';
import { maskId, type PartnerBooking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Pop, T, colors, fonts, radius, spacing } from '@bookdan/ui';
import { api } from './api';

function Line({ icon, label, value }: { icon: keyof typeof Ionicons.glyphMap; label: string; value: string }) {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingVertical: spacing.xs }}>
      <Ionicons name={icon} size={20} color={colors.inkSoft} />
      <View style={{ flex: 1 }}>
        <T variant="label" color={colors.muted}>{label.toUpperCase()}</T>
        <T>{value}</T>
      </View>
    </View>
  );
}

/**
 * The guest's profile as they saved it in the guest app: name, nationality, how to reach them, and their ID. The ID shows
 * masked; "Show ID number" asks the server for the full number (which writes an audit entry). The number lives only in
 * this component's state: it is never cached, and it is gone when the staff member hides it or leaves the screen.
 */
export function GuestProfileCard({ booking }: { booking: PartnerBooking }) {
  const { t } = useTranslation();
  const g = booking.guest;
  const [number, setNumber] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // A different booking, or leaving the screen, always clears the number.
  useEffect(() => () => setNumber(null), [booking.id]);

  async function reveal() {
    setBusy(true);
    setError(null);
    try {
      setNumber((await api.revealGuestId(booking.id)).id_number);
    } catch (e) {
      setError(e instanceof ApiError && e.code === 'GONE' ? t('partner.guest_erased_note') : e instanceof ApiError && e.code === 'NETWORK' ? t('partner.offline_answer') : t('partner.answer_failed'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <View style={{ padding: spacing.lg, borderRadius: radius.lg, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
      <T variant="displaySm">{t('partner.guest_profile')}</T>
      <T variant="h3" style={{ marginTop: spacing.sm }}>{g.full_name}</T>

      <View style={{ marginTop: spacing.sm }}>
        <Line icon="call-outline" label={t('partner.guest_contact')} value={g.contact} />
        <Line icon="flag-outline" label={t('partner.guest_nationality')} value={g.nationality} />
        <Line icon="id-card-outline" label={t(`partner.id_type_${g.id_type}`)} value={number ?? maskId(g.id_last4)} />
      </View>

      {number ? (
        <>
          <Pop style={{ marginTop: spacing.sm, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.fill, alignItems: 'center' }}>
            <T style={{ fontFamily: fonts.bold, fontSize: 24, lineHeight: 34, letterSpacing: 2 }}>{number}</T>
          </Pop>
          <T variant="small" color={colors.warning} style={{ marginTop: spacing.xs }}>{t('partner.guest_id_note')}</T>
          <Button variant="secondary" icon="eye-off-outline" title={t('partner.guest_hide_id')} onPress={() => setNumber(null)} style={{ marginTop: spacing.sm }} />
        </>
      ) : g.id_purged ? (
        <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{t('partner.guest_erased_note')}</T>
      ) : (
        <Button variant="secondary" icon="eye-outline" title={t('partner.guest_show_id')} onPress={reveal} loading={busy} style={{ marginTop: spacing.sm }} />
      )}
      {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {error}</T> : null}
    </View>
  );
}
