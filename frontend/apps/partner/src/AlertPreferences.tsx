import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Ionicons } from '@expo/vector-icons';
import { Switch, View } from 'react-native';
import { ALERT_KINDS, type AlertKind, type HotelAlertPrefs } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, radius, spacing } from '@bookdan/ui';
import { api } from './api';
import { useAlertPrefs } from './queries';

function Row({ title, sub, on, disabled, onChange }: { title: string; sub: string; on: boolean; disabled?: boolean; onChange?: (on: boolean) => void }) {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingVertical: spacing.sm }}>
      <View style={{ flex: 1 }}>
        <T variant="h3">{title}</T>
        <T variant="small" color={colors.inkSoft}>{sub}</T>
      </View>
      <Switch
        accessibilityLabel={title}
        value={on}
        disabled={disabled}
        onValueChange={onChange}
        trackColor={{ true: colors.primary, false: colors.line }}
      />
    </View>
  );
}

/**
 * Which alerts this person wants. New booking requests always reach whoever is on duty (the hotel's owner sets shifts), so that row is fixed. The others are
 * the person's own choice, saved per hotel on the server: the server simply leaves them out of the people an alert goes to.
 */
export function AlertPreferences() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const prefs = useAlertPrefs();
  const save = useMutation({
    mutationFn: ({ hotel, muted }: { hotel: HotelAlertPrefs; muted: AlertKind[] }) => api.setAlertPreferences(hotel.property_id, muted),
    // The switch moves at once; a failure puts it back.
    onMutate: async ({ hotel, muted }) => {
      await qc.cancelQueries({ queryKey: ['alert-prefs'] });
      const before = qc.getQueryData<HotelAlertPrefs[]>(['alert-prefs']);
      qc.setQueryData<HotelAlertPrefs[]>(['alert-prefs'], (old) => old?.map((h) => (h.property_id === hotel.property_id ? { ...h, muted_kinds: muted } : h)));
      return { before };
    },
    onError: (_e, _v, ctx) => qc.setQueryData(['alert-prefs'], ctx?.before),
    onSettled: () => qc.invalidateQueries({ queryKey: ['alert-prefs'] }),
  });

  if (!prefs.data || prefs.data.length === 0) return null;
  return (
    <View style={{ marginTop: spacing.xl }}>
      <T variant="label" color={colors.muted}>{t('partner.alert_prefs').toUpperCase()}</T>
      <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.alert_prefs_sub')}</T>
      {prefs.data.map((hotel) => (
        <View key={hotel.property_id} style={{ marginTop: spacing.md, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line }}>
          {prefs.data.length > 1 ? <T variant="h3" style={{ marginBottom: spacing.xs }}>{hotel.hotel}</T> : null}
          <Row title={t('partner.alert_kind_requests')} sub={t('partner.alert_kind_requests_sub')} on disabled />
          {ALERT_KINDS.filter((k) => hotel.available_kinds.includes(k)).map((k) => {
            const on = !hotel.muted_kinds.includes(k);
            return (
              <Row
                key={k}
                title={t(`partner.alert_kind_${k}`)}
                sub={t(`partner.alert_kind_${k}_sub`)}
                on={on}
                onChange={(next) => save.mutate({ hotel, muted: next ? hotel.muted_kinds.filter((m) => m !== k) as AlertKind[] : ([...hotel.muted_kinds, k] as AlertKind[]) })}
              />
            );
          })}
          {!hotel.alerts_on ? (
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm }}>
              <Ionicons name="alert-circle" size={18} color={colors.danger} />
              <T variant="small" color={colors.danger} style={{ flex: 1 }}>{t('partner.alerts_off_by_owner')}</T>
            </View>
          ) : null}
        </View>
      ))}
    </View>
  );
}
