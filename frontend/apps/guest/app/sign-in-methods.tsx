import { Ionicons } from '@expo/vector-icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { normalizeEmail, normalizeEthiopianPhone, type Identity, type IdentityType } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { Field } from '../src/Field';

/**
 * Sign-in methods. A guest can add a phone number to an email account (or the other way round), so their bookings follow them whichever they use. When the
 * number or address already has its own account, the guest is asked before the two are merged.
 */
export default function SignInMethods() {
  const { t } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const methods = useQuery({ queryKey: ['identities'], queryFn: () => api.getIdentities() });

  const [adding, setAdding] = useState<IdentityType | null>(null);
  const [text, setText] = useState('');
  const [sent, setSent] = useState<Identity | null>(null); // set once the code was sent
  const [code, setCode] = useState('');
  const [mergeAsk, setMergeAsk] = useState<string | null>(null); // the server's description of what would be merged
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const close = () => (router.canGoBack() ? router.back() : router.replace('/'));
  function reset() {
    setAdding(null); setText(''); setSent(null); setCode(''); setMergeAsk(null); setError(null);
  }
  function describe(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.code === 'NETWORK') return t('results.offline_body');
      if (e.code === 'INVALID_OTP') return t('auth.invalid_code');
      if (e.code === 'INVALID_PHONE') return t('auth.invalid_phone');
      if (e.code === 'INVALID_EMAIL') return t('auth.invalid_email');
      if (e.code === 'PHONE_SIGNIN_OFF') return t('link.phone_off');
      if (e.code === 'SEND_FAILED') return t('auth.send_failed');
      if (e.code === 'TOO_MANY_REQUESTS') return t('auth.too_many');
      if (e.code === 'ALREADY_HAS_IDENTITY') return t('link.already');
      if (e.code === 'IDENTITY_NOT_MERGEABLE') return t('link.not_mergeable');
      if (e.code === 'INVALID_STATE') return t('link.busy');
    }
    return t('results.error_body');
  }

  async function sendCode() {
    if (!adding) return;
    const value = adding === 'phone' ? normalizeEthiopianPhone(text) : normalizeEmail(text);
    if (!value) return setError(t(adding === 'phone' ? 'auth.invalid_phone' : 'auth.invalid_email'));
    const id: Identity = { type: adding, value };
    setError(null);
    setBusy(true);
    try {
      await api.sendLinkCode(id);
      setSent(id);
    } catch (e) {
      setError(describe(e));
    } finally {
      setBusy(false);
    }
  }

  async function confirm(merge: boolean) {
    if (!sent) return;
    setError(null);
    setBusy(true);
    try {
      const res = await api.confirmLink(sent, code.trim(), merge);
      // Another account's bookings may have arrived: everything cached about this account is out of date.
      await qc.invalidateQueries();
      qc.setQueryData(['identities'], res);
      setNote(t(res.merged ? 'link.merged' : 'link.done'));
      reset();
    } catch (e) {
      if (e instanceof ApiError && e.code === 'IDENTITY_IN_USE') setMergeAsk(e.message);
      else setError(describe(e));
    } finally {
      setBusy(false);
    }
  }

  function Row({ label, value, type }: { label: string; value: string | null; type: IdentityType }) {
    const blocked = type === 'phone' && methods.data && !methods.data.phone_available;
    return (
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, minHeight: 64, paddingHorizontal: spacing.md, marginTop: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
        <Ionicons name={type === 'phone' ? 'call-outline' : 'mail-outline'} size={22} color={colors.primaryInk} />
        <View style={{ flex: 1 }}>
          <T variant="label" color={colors.muted}>{label.toUpperCase()}</T>
          <T variant="h3">{value ?? t('link.not_added')}</T>
        </View>
        {value ? <Ionicons name="checkmark-circle" size={22} color={colors.success} /> : blocked ? null : <Button variant="secondary" title={t('link.add')} onPress={() => { reset(); setNote(null); setAdding(type); }} />}
      </View>
    );
  }

  return (
    <SafeAreaView style={{ flex: 1 }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={{ padding: spacing.lg }} keyboardShouldPersistTaps="handled">
          <Pressable accessibilityLabel={t('common.back')} onPress={close} style={{ width: 44, height: 44, justifyContent: 'center' }}>
            <Ionicons name="arrow-back" size={24} color={colors.ink} />
          </Pressable>
          <T variant="display" style={{ marginTop: spacing.md }}>{t('link.title')}</T>
          <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('link.intro')}</T>

          {methods.data ? (
            <>
              <Row label={t('auth.email')} value={methods.data.email} type="email" />
              <Row label={t('auth.phone')} value={methods.data.phone} type="phone" />
              {!methods.data.phone_available && !methods.data.phone ? <T variant="small" color={colors.muted} style={{ marginTop: spacing.sm }}>{t('link.phone_off')}</T> : null}
            </>
          ) : (
            <T color={colors.muted} style={{ marginTop: spacing.lg }}>{methods.isError ? t('results.error_body') : '…'}</T>
          )}

          {note ? <T color={colors.success} style={{ marginTop: spacing.lg }}>✓ {note}</T> : null}

          {adding && !sent ? (
            <>
              {adding === 'phone' ? (
                <Field key="phone" label={t('auth.phone')} value={text} onChangeText={setText} placeholder={t('auth.phone_placeholder')} keyboardType="phone-pad" autoComplete="tel" textContentType="telephoneNumber" error={error} onSubmitEditing={sendCode} />
              ) : (
                <Field key="email" label={t('auth.email')} value={text} onChangeText={setText} placeholder={t('auth.email_placeholder')} keyboardType="email-address" autoCapitalize="none" autoCorrect={false} autoComplete="email" textContentType="emailAddress" error={error} onSubmitEditing={sendCode} />
              )}
              <Button title={t('auth.send_code')} onPress={sendCode} loading={busy} style={{ marginTop: spacing.xl }} />
              <Button variant="ghost" title={t('common.cancel')} onPress={reset} style={{ marginTop: spacing.sm }} />
            </>
          ) : null}

          {sent && !mergeAsk ? (
            <>
              <T color={colors.inkSoft} style={{ marginTop: spacing.xl }}>{t('auth.code_sent', { to: sent.value })}</T>
              <Field label={t('auth.code')} value={code} onChangeText={(v) => setCode(v.replace(/\D/g, '').slice(0, 6))} keyboardType="number-pad" autoComplete="sms-otp" textContentType="oneTimeCode" maxLength={6} error={error} onSubmitEditing={() => confirm(false)} />
              <Button title={t('auth.verify')} onPress={() => confirm(false)} loading={busy} disabled={code.length < 6} style={{ marginTop: spacing.xl }} />
              <Button variant="ghost" title={t('common.cancel')} onPress={reset} style={{ marginTop: spacing.sm }} />
            </>
          ) : null}

          {mergeAsk ? (
            <View style={{ marginTop: spacing.xl, padding: spacing.lg, borderRadius: radius.md, borderWidth: 1, borderColor: colors.primary, backgroundColor: colors.primarySoft }}>
              <T variant="h3">{t('link.merge_title')}</T>
              <T color={colors.inkSoft} style={{ marginTop: spacing.sm }}>{mergeAsk}</T>
              {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>✕ {error}</T> : null}
              <Button title={t('link.merge_yes')} onPress={() => confirm(true)} loading={busy} style={{ marginTop: spacing.lg }} />
              <Button variant="ghost" title={t('common.cancel')} onPress={reset} style={{ marginTop: spacing.sm }} />
            </View>
          ) : null}
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
