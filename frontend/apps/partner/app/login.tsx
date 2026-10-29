import { useState } from 'react';
import { Image, KeyboardAvoidingView, Linking, Platform, ScrollView, View } from 'react-native';
import { LinearGradient } from 'expo-linear-gradient';
import { StatusBar } from 'expo-status-bar';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { normalizeEmail, normalizeEthiopianPhone, type Identity, type IdentityType } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Badge, Button, Chip, Pop, Reveal, StepTransition, T, colors, dark, gradients, radius, spacing } from '@bookdan/ui';
import { api, apiUrl, isMockApi } from '../src/api';
import { useAuth } from '../src/auth';
import { Field } from '../src/Field';

/** Staff sign-in: a code by SMS or email. Only staff the platform knows can get in (the server decides). */
export default function Login() {
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const signIn = useAuth((s) => s.signIn);

  const [method, setMethod] = useState<IdentityType>('phone');
  const [text, setText] = useState('');
  const [identity, setIdentity] = useState<Identity | null>(null); // set once the code was sent
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

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
      setError(e instanceof ApiError && e.code === 'NETWORK' ? t('results.offline_body') : e instanceof ApiError && e.code === 'PHONE_SIGNIN_OFF' ? t('auth.phone_off') : e instanceof ApiError && e.code === 'SEND_FAILED' ? t('auth.send_failed') : e instanceof ApiError && e.code === 'TOO_MANY_REQUESTS' ? t('auth.too_many') : t('results.error_body'));
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
      signIn({ token: res.token, refresh_token: res.refresh_token }, res.identity); // the navigator then shows the inbox
    } catch (e) {
      setError(e instanceof ApiError && e.code === 'INVALID_OTP' ? t('auth.invalid_code') : e instanceof ApiError && e.code === 'NOT_STAFF' ? t('auth.not_staff') : t('results.error_body'));
      setBusy(false);
    }
  }

  const emailMode = (identity?.type ?? method) === 'email';

  return (
    <View style={{ flex: 1 }}>
      <StatusBar style="light" />
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={{ paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
          {/* The same navy band as the rest of the app: logo, title and what the sign-in is for. */}
          <LinearGradient
            colors={gradients.header}
            start={{ x: 0, y: 0 }}
            end={{ x: 1, y: 1 }}
            style={{ paddingTop: insets.top + spacing.lg, paddingBottom: spacing.xl, paddingHorizontal: spacing.lg, borderBottomLeftRadius: 28, borderBottomRightRadius: 28 }}
          >
            <Pop><Image source={require('../assets/icon.png')} accessibilityLabel="BookDan" style={{ width: 64, height: 64, borderRadius: radius.md }} /></Pop>
            <Reveal index={1}>
              <T variant="display" color="#FFFFFF" style={{ marginTop: spacing.lg }}>{t('partner.login_title')}</T>
              <T color={dark.textSoft} style={{ marginTop: spacing.xs }}>{t('partner.login_subtitle')}</T>
            </Reveal>
          </LinearGradient>
          <View style={{ padding: spacing.lg }}>

          <StepTransition stepKey={identity === null ? 'enter' : 'code'}>
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
              {!isMockApi && apiUrl ? <Button variant="ghost" icon="business-outline" title={t('partner.apply')} onPress={() => Linking.openURL(`${apiUrl}/apply`)} style={{ marginTop: spacing.sm }} /> : null}
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
          </StepTransition>

          {isMockApi ? (
            <View style={{ marginTop: spacing.xl, gap: spacing.sm }}>
              <Badge tone="warning" icon="⚠" label="SIMULATED" />
              <T variant="small" color={colors.inkSoft}>{t(emailMode ? 'auth.demo_hint_email' : 'auth.demo_hint')}</T>
            </View>
          ) : null}
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </View>
  );
}
