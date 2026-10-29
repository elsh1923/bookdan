import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { Alert, Pressable, ScrollView, View } from 'react-native';
import { ApiError } from '@bookdan/api-client';
import { canMarkNoShow, isAwaitingAnswer, todayAddis, type DeclineReason, type HotelProblemReason } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, EmptyState, Reveal, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { HeaderBand } from '../../src/HeaderBand';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../src/api';
import { checkinErrorKey } from '../../src/checkin';
import { AwaitingCard, BookingRow } from '../../src/BookingCards';
import { DeclineSheet } from '../../src/DeclineSheet';
import { DisputePanel } from '../../src/DisputePanel';
import { HotelProblemSheet } from '../../src/HotelProblemSheet';
import { GuestProfileCard } from '../../src/GuestProfileCard';
import { useInbox } from '../../src/queries';
import { useAnswers } from '../../src/useAnswers';

/** One booking, full screen. A tapped new-booking alert opens this, so the hotel can answer straight away. */
export default function BookingScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const inbox = useInbox();
  const { busyId, errors, answer, refresh } = useAnswers();
  const [declining, setDeclining] = useState(false);
  const qc = useQueryClient();
  const [checkingIn, setCheckingIn] = useState(false);
  const [markingNoShow, setMarkingNoShow] = useState(false);
  const [checkinError, setCheckinError] = useState<string | null>(null);
  // Cancelling a confirmed booking, or reporting that the hotel cannot host the guest.
  const [problem, setProblem] = useState<'cancel' | 'host' | null>(null);
  const [problemBusy, setProblemBusy] = useState(false);
  const [problemError, setProblemError] = useState<string | null>(null);
  async function reportProblem(bookingId: string, reason: HotelProblemReason) {
    setProblemBusy(true);
    setProblemError(null);
    try {
      if (problem === 'cancel') await api.cancelBooking(bookingId, reason);
      else await api.relocateBooking(bookingId, reason);
      await qc.invalidateQueries({ queryKey: ['inbox'] });
      qc.invalidateQueries({ queryKey: ['rooms-chart'] });
      setProblem(null);
    } catch (e) {
      setProblemError(
        e instanceof ApiError && e.code === 'FORBIDDEN' ? t('partner.owner_only')
          : e instanceof ApiError && e.code === 'NETWORK' ? t('results.offline_body')
          : t('results.error_body'),
      );
    } finally {
      setProblemBusy(false);
    }
  }

  // The guest never came: the hotel keeps its share and the nights still to come go back on sale.
  function noShow(bookingId: string) {
    Alert.alert(t('partner.no_show_title'), t('partner.no_show_body'), [
      { text: t('common.cancel'), style: 'cancel' },
      {
        text: t('partner.no_show_button'),
        style: 'destructive',
        onPress: async () => {
          setMarkingNoShow(true);
          setCheckinError(null);
          try {
            await api.markNoShow(bookingId);
            await qc.invalidateQueries({ queryKey: ['inbox'] });
            qc.invalidateQueries({ queryKey: ['rooms-chart'] });
          } catch (e) {
            setCheckinError(t(checkinErrorKey(e)));
          } finally {
            setMarkingNoShow(false);
          }
        },
      },
    ]);
  }

  // For a guest who arrives without the voucher: the receptionist checks them in from the booking. The server decides if it is allowed.
  async function checkIn(bookingId: string) {
    setCheckingIn(true);
    setCheckinError(null);
    try {
      await api.checkInGuest(bookingId);
      await qc.invalidateQueries({ queryKey: ['inbox'] });
    } catch (e) {
      setCheckinError(t(checkinErrorKey(e)));
    } finally {
      setCheckingIn(false);
    }
  }

  const booking = inbox.data?.items.find((b) => b.id === id);
  const back = () => (router.canGoBack() ? router.back() : router.replace('/'));

  const header = <HeaderBand compact title={t('partner.booking_title')} eyebrow={booking?.reference} onBack={back} />;

  let body;
  if (inbox.isPending) {
    body = <View style={{ gap: spacing.md }}><Skeleton height={176} radius={radius.lg} /><Skeleton height={220} radius={radius.lg} /></View>;
  } else if (inbox.isError) {
    body = (
      <EmptyState
        icon="⚠️"
        title={t(inbox.error instanceof ApiError && inbox.error.code === 'NETWORK' ? 'results.offline_title' : 'results.error_title')}
        body={t(inbox.error instanceof ApiError && inbox.error.code === 'NETWORK' ? 'results.offline_body' : 'results.error_body')}
        actionLabel={t('common.retry')}
        onAction={() => inbox.refetch()}
      />
    );
  } else if (!booking) {
    body = <EmptyState icon="🔎" title={t('results.error_title')} body={t('partner.already_answered')} actionLabel={t('common.back')} onAction={back} />;
  } else if (isAwaitingAnswer(booking)) {
    body = (
      <AwaitingCard
        b={booking}
        receivedAt={inbox.dataUpdatedAt}
        busy={busyId === booking.id}
        error={errors[booking.id] || null}
        onAccept={() => answer(booking, () => api.acceptBooking(booking.id))}
        onDecline={() => setDeclining(true)}
        onTimeUp={refresh}
      />
    );
  } else {
    body = (
      <View>
        <BookingRow b={booking} />
        {booking.status === 'CONFIRMED' ? <Button title={t('partner.checkin_confirm')} icon="log-in-outline" onPress={() => checkIn(booking.id)} loading={checkingIn} /> : null}
        {canMarkNoShow(booking, todayAddis()) ? <Button variant="ghost" title={t('partner.no_show_button')} onPress={() => noShow(booking.id)} loading={markingNoShow} style={{ marginTop: spacing.sm }} /> : null}
        {booking.checked_in_at ? <T variant="small" color={colors.success}>{t('partner.checkin_done')}</T> : null}
        {booking.status === 'CONFIRMED' ? (
          <View style={{ marginTop: spacing.md }}>
            <Button variant="secondary" icon="swap-horizontal-outline" title={t('partner.cannot_host')} onPress={() => { setProblemError(null); setProblem('host'); }} />
            <Button variant="ghost" icon="close-circle-outline" title={t('partner.cancel_booking')} onPress={() => { setProblemError(null); setProblem('cancel'); }} style={{ marginTop: spacing.xs }} />
          </View>
        ) : null}
        {booking.status === 'NEEDS_RELOCATION' ? (
          <View style={{ marginTop: spacing.md }}>
            <T variant="small" color={colors.inkSoft}>{t('partner.relocation_waiting')}</T>
            <Button variant="ghost" icon="close-circle-outline" title={t('partner.cancel_booking')} onPress={() => { setProblemError(null); setProblem('cancel'); }} style={{ marginTop: spacing.xs }} />
          </View>
        ) : null}
        {booking.status === 'DISPUTED' ? <DisputePanel bookingId={booking.id} /> : null}
        <Button variant="ghost" icon="chatbubbles-outline" title={t('support.get_help')} onPress={() => router.push({ pathname: '/support/new', params: { booking: booking.id, ref: booking.reference } })} style={{ marginTop: spacing.sm }} />
        {checkinError ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>{checkinError}</T> : null}
      </View>
    );
  }

  return (
    <View style={{ flex: 1 }}>
      {header}
      <ScrollView contentContainerStyle={{ padding: spacing.lg }}>
        <Reveal>{body}</Reveal>
        {booking ? <Reveal index={2} style={{ marginTop: spacing.lg }}><GuestProfileCard booking={booking} /></Reveal> : null}
      </ScrollView>
      <HotelProblemSheet
        visible={problem !== null && !!booking}
        busy={problemBusy}
        error={problemError}
        title={problem === 'cancel' ? t('partner.cancel_title') : t('partner.cannot_host_title')}
        note={problem === 'cancel' ? t('partner.cancel_note') : t('partner.cannot_host_note')}
        confirmLabel={problem === 'cancel' ? t('partner.cancel_confirm') : t('partner.cannot_host_confirm')}
        onClose={() => setProblem(null)}
        onConfirm={(reason) => booking && reportProblem(booking.id, reason)}
      />
      <DeclineSheet
        visible={declining && !!booking}
        busy={!!booking && busyId === booking.id}
        onClose={() => setDeclining(false)}
        onConfirm={async (reason: DeclineReason) => {
          if (!booking) return;
          const outcome = await answer(booking, () => api.declineBooking(booking.id, reason));
          if (outcome !== 'offline') setDeclining(false);
        }}
      />
    </View>
  );
}
