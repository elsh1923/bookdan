import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { isAwaitingAnswer, type DeclineReason } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, Reveal, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { AwaitingCard, BookingRow } from '../../src/BookingCards';
import { DeclineSheet } from '../../src/DeclineSheet';
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

  const booking = inbox.data?.items.find((b) => b.id === id);
  const back = () => (router.canGoBack() ? router.back() : router.replace('/'));

  const header = (
    <Pressable accessibilityLabel={t('common.back')} onPress={back} style={{ width: 44, height: 44, justifyContent: 'center', marginLeft: spacing.lg }}>
      <Ionicons name="arrow-back" size={24} color={colors.ink} />
    </Pressable>
  );

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
    body = <BookingRow b={booking} />;
  }

  return (
    <SafeAreaView style={{ flex: 1 }}>
      {header}
      <ScrollView contentContainerStyle={{ padding: spacing.lg }}>
        <T variant="display" style={{ marginBottom: spacing.md }}>{t('partner.booking_title')}</T>
        <Reveal>{body}</Reveal>
        {booking ? <Reveal index={2} style={{ marginTop: spacing.lg }}><GuestProfileCard booking={booking} /></Reveal> : null}
      </ScrollView>
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
    </SafeAreaView>
  );
}
