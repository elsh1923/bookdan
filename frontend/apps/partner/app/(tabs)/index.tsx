import { useMemo, useState } from 'react';
import { ActivityIndicator, RefreshControl, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { isAwaitingAnswer, sortInbox, type DeclineReason, type PartnerBooking } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { EmptyState, T, colors, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { AwaitingCard, BookingRow } from '../../src/BookingCards';
import { DeclineSheet } from '../../src/DeclineSheet';
import { useInbox } from '../../src/queries';
import { useAnswers } from '../../src/useAnswers';

export default function Inbox() {
  const { t } = useTranslation();
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
          <ActivityIndicator color={colors.primary} style={{ marginTop: spacing.xxl }} />
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
                {waiting.map((b) => (
                  <AwaitingCard
                    key={b.id}
                    b={b}
                    receivedAt={inbox.dataUpdatedAt}
                    busy={busyId === b.id}
                    error={errors[b.id] || null}
                    onAccept={() => answer(b, () => api.acceptBooking(b.id))}
                    onDecline={() => setDeclining(b)}
                    onTimeUp={refresh}
                  />
                ))}
              </View>
            ) : null}

            {others.length > 0 ? (
              <View style={{ marginTop: spacing.lg }}>
                <T variant="h3" style={{ marginBottom: spacing.sm }}>{t('partner.upcoming_title')}</T>
                {others.map((b) => <BookingRow key={b.id} b={b} />)}
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
