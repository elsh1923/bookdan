import { useRouter } from 'expo-router';
import { useMemo, useState } from 'react';
import { RefreshControl, ScrollView, View } from 'react-native';
import Animated, { FadeInDown, FadeOut, LinearTransition } from 'react-native-reanimated';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { isAwaitingAnswer, sortInbox, type DeclineReason, type PartnerBooking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { AwaitingCard, BookingRow } from '../../src/BookingCards';
import { DeclineSheet } from '../../src/DeclineSheet';
import { useInbox } from '../../src/queries';
import { useAnswers } from '../../src/useAnswers';

export default function Inbox() {
  const { t } = useTranslation();
  const router = useRouter();
  const inbox = useInbox();
  const { busyId, errors, answer, refresh } = useAnswers();
  const [declining, setDeclining] = useState<PartnerBooking | null>(null);

  const items = useMemo(() => sortInbox(inbox.data?.items ?? []), [inbox.data]);
  const waiting = items.filter(isAwaitingAnswer);
  const others = items.filter((b) => !isAwaitingAnswer(b));

  return (
    <SafeAreaView style={{ flex: 1 }} edges={['top']}>
      <ScrollView
        contentContainerStyle={{ padding: spacing.lg, paddingBottom: spacing.xxl }}
        refreshControl={<RefreshControl refreshing={inbox.isRefetching && !busyId} onRefresh={refresh} tintColor={colors.primary} />}
      >
        <T variant="label" color={colors.muted}>{(inbox.data?.property.name ?? '').toUpperCase()}</T>
        <T variant="display">{t('partner.tab_bookings')}</T>

        {inbox.isPending ? (
          <View style={{ marginTop: spacing.lg, gap: spacing.md }}>
            <Skeleton height={176} radius={radius.lg} />
            <Skeleton height={96} radius={radius.md} />
            <Skeleton height={96} radius={radius.md} />
          </View>
        ) : inbox.isError ? (
          <EmptyState
            icon="⚠️"
            title={t(inbox.error instanceof ApiError && inbox.error.code === 'NETWORK' ? 'results.offline_title' : 'results.error_title')}
            body={t(inbox.error instanceof ApiError && inbox.error.code === 'NETWORK' ? 'results.offline_body' : 'results.error_body')}
            actionLabel={t('common.retry')}
            onAction={() => inbox.refetch()}
          />
        ) : items.length === 0 ? (
          <EmptyState icon="📭" title={t('partner.empty_title')} body={t('partner.empty_body')} />
        ) : (
          <>
            {waiting.length > 0 ? (
              <View style={{ marginTop: spacing.lg }}>
                <T variant="h3" style={{ marginBottom: spacing.sm }}>{t('partner.waiting_title')} ({waiting.length})</T>
                {waiting.map((b, i) => (
                  <Animated.View key={b.id} layout={LinearTransition.duration(260)} entering={FadeInDown.delay(Math.min(i, 6) * 70).duration(380)} exiting={FadeOut.duration(180)}>
                  <AwaitingCard
                    b={b}
                    receivedAt={inbox.dataUpdatedAt}
                    busy={busyId === b.id}
                    error={errors[b.id] || null}
                    onAccept={() => answer(b, () => api.acceptBooking(b.id))}
                    onDecline={() => setDeclining(b)}
                    onTimeUp={refresh}
                    onOpen={() => router.push({ pathname: '/booking/[id]', params: { id: b.id } })}
                  />
                  </Animated.View>
                ))}
              </View>
            ) : null}

            {others.length > 0 ? (
              <View style={{ marginTop: spacing.lg }}>
                <T variant="h3" style={{ marginBottom: spacing.sm }}>{t('partner.upcoming_title')}</T>
                {others.map((b, i) => (
                  <Animated.View key={b.id} layout={LinearTransition.duration(260)} entering={FadeInDown.delay(Math.min(i, 6) * 70).duration(380)}>
                    <BookingRow b={b} onOpen={() => router.push({ pathname: '/booking/[id]', params: { id: b.id } })} />
                  </Animated.View>
                ))}
              </View>
            ) : null}
          </>
        )}
      </ScrollView>

      <DeclineSheet
        visible={!!declining}
        busy={!!declining && busyId === declining.id}
        onClose={() => setDeclining(null)}
        onConfirm={async (reason: DeclineReason) => {
          if (!declining) return;
          const outcome = await answer(declining, () => api.declineBooking(declining.id, reason));
          if (outcome !== 'offline') setDeclining(null); // offline: keep the sheet open so the hotel can try again
        }}
      />
    </SafeAreaView>
  );
}
