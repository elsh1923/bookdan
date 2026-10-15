import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter, type Href } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { normalizeEmail, normalizeEthiopianPhone, type Identity, type IdentityType } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Badge, Button, Chip, T, colors, spacing } from '@bookdan/ui';
import { api, isMockApi } from '../src/api';
import { useAuth } from '../src/auth';
import { Field } from '../src/Field';

export default function SignIn() {
  const { t } = useTranslation();
  const router = useRouter();
  const { next } = useLocalSearchParams<{ next?: string }>();
  const signIn = useAuth((s) => s.signIn);

  const [method, setMethod] = useState<IdentityType>('phone');
  const [text, setText] = useState('');
  const [identity, setIdentity] = useState<Identity | null>(null); // set once the code was sent
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const close = () => (router.canGoBack() ? router.back() : router.replace('/'));

  function switchMethod(m: IdentityType) {
    if (m === method) return;
    setMethod(m);
    setText('');
    setError(null);
  }

  async function sendCode() {
    const value = method === 'phone' ? normalizeEthiopianPhone(text) : normalizeEmail(text);
    if (!value) return setError(t(method === 'phone' ? 'auth.invalid_phone' : 'auth.invalid_email'));
    const id: Identity = { type: method, value };
    setError(null);
    setBusy(true);
    try {
      await api.requestOtp(id);
      setIdentity(id);
    } catch (e) {
      setError(e instanceof ApiError && e.code === 'NETWORK' ? t('results.offline_body') : t('results.error_body'));
    } finally {
      setBusy(false);
    }
  }

  async function verify() {
    if (!identity) return;
    setError(null);
    setBusy(true);
    try {
      const res = await api.verifyOtp(identity, code.trim());
      signIn({ token: res.token, refresh_token: res.refresh_token }, res.identity);
      if (next) router.replace(next as Href);
      else close();
    } catch (e) {
      setError(e instanceof ApiError && e.code === 'INVALID_OTP' ? t('auth.invalid_code') : t('results.error_body'));
    } finally {
      setBusy(false);
    }
  }

  const emailMode = (identity?.type ?? method) === 'email';

  return (
    <SafeAreaView style={{ flex: 1 }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={{ padding: spacing.lg }} keyboardShouldPersistTaps="handled">
          <Pressable accessibilityLabel={t('common.back')} onPress={close} style={{ width: 44, height: 44, justifyContent: 'center' }}>
            <Ionicons name="arrow-back" size={24} color={colors.ink} />
          </Pressable>

          <T variant="display" style={{ marginTop: spacing.md }}>{t('auth.title')}</T>
          <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t(emailMode ? 'auth.subtitle_email' : 'auth.subtitle')}</T>

          {identity === null ? (
            <>
              <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.lg }}>
                <Chip label={t('auth.method_phone')} selected={method === 'phone'} onPress={() => switchMethod('phone')} />
                <Chip label={t('auth.method_email')} selected={method === 'email'} onPress={() => switchMethod('email')} />
              </View>

              {method === 'phone' ? (
                <Field
                  key="phone"
                  label={t('auth.phone')}
                  value={text}
                  onChangeText={setText}
                  placeholder={t('auth.phone_placeholder')}
                  keyboardType="phone-pad"
                  autoComplete="tel"
                  textContentType="telephoneNumber"
                  error={error}
                  onSubmitEditing={sendCode}
                />
              ) : (
                <Field
                  key="email"
                  label={t('auth.email')}
                  value={text}
                  onChangeText={setText}
                  placeholder={t('auth.email_placeholder')}
                  keyboardType="email-address"
                  autoCapitalize="none"
                  autoCorrect={false}
                  autoComplete="email"
                  textContentType="emailAddress"
                  error={error}
                  onSubmitEditing={sendCode}
                />
              )}
              <Button title={t('auth.send_code')} onPress={sendCode} loading={busy} style={{ marginTop: spacing.xl }} />
            </>
          ) : (
            <>
              <T color={colors.inkSoft} style={{ marginTop: spacing.xl }}>{t('auth.code_sent', { to: identity.value })}</T>
              <Field
                label={t('auth.code')}
                value={code}
                onChangeText={(v) => setCode(v.replace(/\D/g, '').slice(0, 6))}
                keyboardType="number-pad"
                autoComplete="sms-otp"
                textContentType="oneTimeCode"
                maxLength={6}
                error={error}
                onSubmitEditing={verify}
              />
              <Button title={t('auth.verify')} onPress={verify} loading={busy} disabled={code.length < 6} style={{ marginTop: spacing.xl }} />
              <Button
                variant="ghost"
                title={t(identity.type === 'email' ? 'auth.change_email' : 'auth.change_number')}
                onPress={() => { setIdentity(null); setCode(''); setError(null); }}
                style={{ marginTop: spacing.sm }}
              />
            </>
          )}

          {isMockApi ? (
            <View style={{ marginTop: spacing.xl, gap: spacing.sm }}>
              <Badge tone="warning" icon="⚠" label="SIMULATED" />
              <T variant="small" color={colors.inkSoft}>{t(emailMode ? 'auth.demo_hint_email' : 'auth.demo_hint')}</T>
            </View>
          ) : null}
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
