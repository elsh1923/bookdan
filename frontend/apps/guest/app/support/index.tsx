import { Ionicons } from '@expo/vector-icons';
import { useQuery } from '@tanstack/react-query';
import { Redirect, useRouter } from 'expo-router';
import { ActivityIndicator, FlatList, Pressable, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useTranslation } from '@bookdan/i18n';
import { Badge, Button, EmptyState, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { useAuth } from '../../src/auth';
import { statusTone } from '../../src/support';

/** The guest's requests to support, newest activity first. */
export default function SupportList() {
  const { t } = useTranslation();
  const router = useRouter();
  const token = useAuth((s) => s.token);
  const list = useQuery({ queryKey: ['support'], queryFn: () => api.listSupportTickets(), enabled: !!token, refetchOnMount: 'always' });
  if (!token) return <Redirect href={{ pathname: '/signin', params: { next: '/support' } }} />;

  return (
    <SafeAreaView style={{ flex: 1 }}>
      <View style={{ flexDirection: 'row', alignItems: 'center', paddingHorizontal: spacing.lg, gap: spacing.sm }}>
        <Pressable accessibilityLabel={t('common.back')} onPress={() => (router.canGoBack() ? router.back() : router.replace('/'))} style={{ width: 44, height: 44, justifyContent: 'center' }}>
          <Ionicons name="arrow-back" size={24} color={colors.ink} />
        </Pressable>
        <T variant="h2" style={{ flex: 1 }}>{t('support.title')}</T>
      </View>
      {list.isPending ? (
        <ActivityIndicator style={{ marginTop: spacing.xxl }} color={colors.primary} />
      ) : (
        <FlatList
          data={list.data ?? []}
          keyExtractor={(x) => x.id}
          contentContainerStyle={{ padding: spacing.lg, gap: spacing.md }}
          ListHeaderComponent={<Button icon="add" title={t('support.new_request')} onPress={() => router.push('/support/new')} />}
          ListEmptyComponent={<EmptyState icon="💬" title={t('support.title')} body={t('support.empty')} />}
          renderItem={({ item }) => (
            <Pressable
              accessibilityRole="button"
              onPress={() => router.push({ pathname: '/support/[id]', params: { id: item.id } })}
              style={{ padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface, gap: spacing.xs }}
            >
              <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', gap: spacing.sm }}>
                <T variant="h3" style={{ flex: 1 }} numberOfLines={1}>{item.subject}</T>
                <Badge tone={statusTone(item.status)} label={t(`support.status_${item.status}`)} />
              </View>
              {item.booking_code ? <T variant="small" color={colors.muted}>{t('support.about_booking', { code: item.booking_code })}</T> : null}
              {item.preview ? <T variant="small" color={colors.inkSoft} numberOfLines={2}>{item.preview}</T> : null}
            </Pressable>
          )}
        />
      )}
    </SafeAreaView>
  );
}
