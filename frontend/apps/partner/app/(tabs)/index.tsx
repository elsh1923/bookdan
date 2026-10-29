import { useRouter } from 'expo-router';
import { useMemo, useState } from 'react';
import { RefreshControl, ScrollView, View } from 'react-native';
import Animated, { FadeInDown, FadeOut, LinearTransition } from 'react-native-reanimated';
import { ApiError } from '@bookdan/api-client';
import { isAwaitingAnswer, sortInbox, todayAddis, type DeclineReason, type PartnerBooking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { AwaitingCard, BookingRow } from '../../src/BookingCards';
import { DeclineSheet } from '../../src/DeclineSheet';
import { HeaderBand, StatTile } from '../../src/HeaderBand';
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
  const today = todayAddis();
  const arriving = items.filter((b) => b.status === 'CONFIRMED' && b.check_in === today).length;
  const inHouse = items.filter((b) => b.status === 'CHECKED_IN').length;

  return (
    <View style={{ flex: 1 }}>
      <ScrollView
        contentContainerStyle={{ paddingBottom: spacing.xxl }}
        refreshControl={<RefreshControl refreshing={inbox.isRefetching && !busyId} onRefresh={refresh} tintColor={colors.primary} />}
      >
        <HeaderBand eyebrow={inbox.data?.property.name ?? ''} title={t('partner.tab_bookings')}>
          {inbox.data ? (
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md }}>
              <StatTile icon="timer-outline" value={waiting.length} label={t('partner.stat_waiting')} alert />
              <StatTile icon="log-in-outline" value={arriving} label={t('partner.stat_arriving')} />
              <StatTile icon="bed-outline" value={inHouse} label={t('partner.stat_inhouse')} />
            </View>
          ) : null}
        </HeaderBand>
        <View style={{ padding: spacing.lg }}>

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
        </View>
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
    </View>
  );
}
