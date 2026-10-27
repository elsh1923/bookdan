import { Ionicons } from '@expo/vector-icons';
import { useQueryClient } from '@tanstack/react-query';
import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, T, colors, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { useAuth } from '../../src/auth';
import { Field } from '../../src/Field';

/** Start a request: a question or a problem, optionally about one of the guest's bookings (the booking screen passes its id and reference). */
export default function NewSupportRequest() {
  const { t } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const token = useAuth((s) => s.token);
  const p = useLocalSearchParams<{ booking?: string; ref?: string }>();
  const [kind, setKind] = useState<'question' | 'problem'>(p.booking ? 'problem' : 'question');
  const [subject, setSubject] = useState('');
  const [message, setMessage] = useState('');
  const [errors, setErrors] = useState<{ subject?: string; message?: string; form?: string }>({});
  const [busy, setBusy] = useState(false);
  if (!token) return <Redirect href={{ pathname: '/signin', params: { next: '/support/new' } }} />;

  const close = () => (router.canGoBack() ? router.back() : router.replace('/support'));

  async function send() {
    const next: typeof errors = {};
    if (subject.trim().length < 3) next.subject = t('support.subject_required');
    if (message.trim().length < 1) next.message = t('support.message_required');
    setErrors(next);
    if (Object.keys(next).length > 0) return;
    setBusy(true);
    try {
      const { id } = await api.createSupportTicket({ kind, subject: subject.trim(), message: message.trim(), booking_id: p.booking || undefined });
      await qc.invalidateQueries({ queryKey: ['support'] });
      router.replace({ pathname: '/support/[id]', params: { id } });
    } catch (e) {
      const key =
        e instanceof ApiError && e.code === 'TOO_MANY_TICKETS' ? 'support.too_many'
          : e instanceof ApiError && e.code === 'NETWORK' ? 'results.offline_body'
          : 'results.error_body';
      setErrors({ form: t(key) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <View style={{ paddingHorizontal: spacing.lg }}>
          <Pressable accessibilityLabel={t('common.cancel')} onPress={close} style={{ width: 44, height: 44, justifyContent: 'center' }}>
            <Ionicons name="close" size={26} color={colors.ink} />
          </Pressable>
        </View>
        <ScrollView contentContainerStyle={{ padding: spacing.lg, paddingTop: 0, paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
          <T variant="display">{t('support.new_request')}</T>
          {p.booking && p.ref ? <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('support.about_booking', { code: p.ref })}</T> : null}

          <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.lg }}>
            <Chip label={t('support.kind_question')} selected={kind === 'question'} onPress={() => setKind('question')} />
            <Chip label={t('support.kind_problem')} selected={kind === 'problem'} onPress={() => setKind('problem')} />
          </View>

          <Field label={t('support.subject')} hint={t('support.subject_hint')} value={subject} onChangeText={setSubject} maxLength={120} error={errors.subject} />
          <Field
            label={t('support.message')}
            hint={t('support.message_hint')}
            value={message}
            onChangeText={setMessage}
            maxLength={2000}
            multiline
            textAlignVertical="top"
            style={{ minHeight: 140, paddingTop: spacing.md }}
            error={errors.message}
          />
          {errors.form ? <T color={colors.danger} style={{ marginTop: spacing.md }}>✕ {errors.form}</T> : null}
          <Button title={t('support.send')} loading={busy} onPress={send} style={{ marginTop: spacing.xl }} />
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
