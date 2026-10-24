import { Ionicons } from '@expo/vector-icons';
import { useEffect, useRef } from 'react';
import { View } from 'react-native';
import { formatCountdown, maskId, type PartnerBooking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Badge, Button, Pop, PressableScale, Pulse, T, colors, fonts, radius, shadow, spacing } from '@bookdan/ui';
import { URGENT_MS, useRemaining } from './Countdown';
import { price, shortDate } from './format';

function useMonths(): string[] {
  const { i18n } = useTranslation();
  return i18n.t('months.short', { returnObjects: true }) as unknown as string[];
}

/** Who the guest is, from the profile they saved in the guest app. The ID is masked here; the booking screen can show it all. */
function GuestLine({ b, onOpen }: { b: PartnerBooking; onOpen?: () => void }) {
  const { t } = useTranslation();
  const Wrap: any = onOpen ? PressableScale : View;
  return (
    <Wrap onPress={onOpen} scaleTo={0.98} accessibilityRole={onOpen ? 'button' : undefined} style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.xs, marginTop: spacing.xs, minHeight: onOpen ? 44 : undefined }}>
      <Ionicons name="id-card-outline" size={16} color={colors.inkSoft} />
      <T variant="small" color={colors.inkSoft} style={{ flex: 1 }} numberOfLines={1}>
        {t(`partner.id_type_${b.guest.id_type}`)} {maskId(b.guest.id_last4)} · {b.guest.nationality}
      </T>
      {onOpen ? <Ionicons name="chevron-forward" size={18} color={colors.muted} /> : null}
    </Wrap>
  );
}

export function StayLine({ b }: { b: PartnerBooking }) {
  const { t } = useTranslation();
  const months = useMonths();
  return (
    <>
      {b.property_name ? <T variant="label" color={colors.primaryInk}>{b.property_name.toUpperCase()}</T> : null}
      <T color={colors.inkSoft}>
        {b.rooms > 1 ? `${b.rooms} × ` : ''}{b.room_name} · {shortDate(b.check_in, months)} – {shortDate(b.check_out, months)} · {t('common.night', { count: b.nights })}
      </T>
      <T variant="small" color={colors.muted}>{t('common.guest', { count: b.guests })} · {b.reference}</T>
      {b.meal === 'breakfast' || !b.refundable ? (
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.xs, marginTop: spacing.xs }}>
          {b.meal === 'breakfast' ? <Badge tone="success" icon="☕" label={t('partner.meal_breakfast')} /> : null}
          {!b.refundable ? <Badge tone="warning" icon="⚠" label={t('partner.nonrefundable')} /> : null}
        </View>
      ) : null}
    </>
  );
}

/** A booking the hotel still has to answer: countdown, and large Accept / Decline buttons. */
export function AwaitingCard({ b, receivedAt, busy, error, onAccept, onDecline, onTimeUp, onOpen }: {
  b: PartnerBooking;
  receivedAt: number;
  busy: boolean;
  error: string | null;
  onAccept: () => void;
  onDecline: () => void;
  onTimeUp: () => void;
  /** Opens the booking screen with the guest's full profile. */
  onOpen?: () => void;
}) {
  const { t } = useTranslation();
  const remaining = useRemaining(b.hotel_deadline_at, b.server_time, receivedAt);
  const over = remaining !== null && remaining <= 0;
  const urgent = remaining !== null && remaining < URGENT_MS;

  // When the time runs out, ask the server: it has already refunded the guest and closed the booking.
  const told = useRef(false);
  useEffect(() => {
    if (over && !told.current) {
      told.current = true;
      onTimeUp();
    }
  }, [over, onTimeUp]);

  const accent = over ? colors.muted : urgent ? colors.danger : colors.primary;
  return (
    <View style={[{ flexDirection: 'row', backgroundColor: colors.surface, borderRadius: radius.lg, overflow: 'hidden', borderWidth: 1, borderColor: colors.line, marginBottom: spacing.md }, shadow.card]}>
      <View style={{ width: 6, backgroundColor: accent }} />
      <View style={{ flex: 1, padding: spacing.md }}>
        <View style={{ flexDirection: 'row', alignItems: 'flex-start', justifyContent: 'space-between', gap: spacing.sm }}>
          <View style={{ flex: 1 }}>
            <T variant="h2" numberOfLines={2}>{b.guest_name}</T>
          </View>
          {remaining !== null ? (
            <Pulse active={urgent && !over} style={{ alignItems: 'center', paddingHorizontal: spacing.md, paddingVertical: 6, borderRadius: radius.md, backgroundColor: urgent || over ? colors.dangerSoft : colors.primarySoft }}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
                <Ionicons name="timer-outline" size={16} color={accent} />
                <T style={{ fontFamily: fonts.bold, fontVariant: ['tabular-nums'] }} color={over ? colors.danger : accent}>
                  {over ? t('partner.time_up') : formatCountdown(remaining)}
                </T>
              </View>
              {!over ? <T variant="label" color={colors.inkSoft}>{t('partner.time_to_answer')}</T> : null}
            </Pulse>
          ) : null}
        </View>

        <View style={{ marginTop: spacing.xs }}><StayLine b={b} /></View>
        <GuestLine b={b} onOpen={onOpen} />
        <T variant="h3" color={colors.primaryInk} style={{ marginTop: spacing.sm }}>
          {t('partner.paid')} · {price(b.total_santim, b.currency)}
        </T>

        {error ? (
          <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md, padding: spacing.sm, borderRadius: radius.sm, backgroundColor: colors.dangerSoft }}>
            <Ionicons name="alert-circle-outline" size={18} color={colors.danger} />
            <T variant="small" color={colors.danger} style={{ flex: 1 }}>{error}</T>
          </View>
        ) : null}

        <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md }}>
          <Button variant="secondary" title={t('partner.decline')} onPress={onDecline} disabled={busy || over} style={{ flex: 1 }} />
          <Button title={t('partner.accept')} icon="checkmark" onPress={onAccept} loading={busy} disabled={over} style={{ flex: 2 }} />
        </View>
      </View>
    </View>
  );
}

/** A booking that is already answered (or answered for us when the time ran out). */
export function BookingRow({ b, onOpen }: { b: PartnerBooking; onOpen?: () => void }) {
  const { t } = useTranslation();
  const confirmed = b.status === 'CONFIRMED' || b.status === 'CHECKED_IN' || b.status === 'COMPLETED';
  const reason = b.decline_reason ? t(`partner.reason_${b.decline_reason}`) : null;
  return (
    <View style={{ padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface, marginBottom: spacing.sm }}>
      <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'flex-start', gap: spacing.sm }}>
        <T variant="h3" style={{ flex: 1 }} numberOfLines={1}>{b.guest_name}</T>
        <Pop key={b.status}><Badge tone={confirmed ? 'success' : 'neutral'} icon={confirmed ? '✓' : '✕'} label={t(`status.${b.status}`)} /></Pop>
      </View>
      <StayLine b={b} />
      {reason ? <T variant="small" color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{reason}</T> : null}
      <GuestLine b={b} onOpen={onOpen} />
    </View>
  );
}
