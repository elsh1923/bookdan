import { Ionicons } from '@expo/vector-icons';
import { useQueryClient } from '@tanstack/react-query';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { useRef, useState } from 'react';
import { KeyboardAvoidingView, Platform, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { judgeVoucher, todayAddis, type CheckinLookup, type CheckinQuery, type VoucherPayload } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Pop, Reveal, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { StayLine } from '../../src/BookingCards';
import { checkinErrorKey } from '../../src/checkin';
import { Field } from '../../src/Field';
import { HeaderBand } from '../../src/HeaderBand';
import { price } from '../../src/format';
import { useOffline } from '../../src/offline';

/** Check in a guest: scan the QR code on their voucher, or type the booking reference and the PIN from the same screen. */
export default function CheckinScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [permission, requestPermission] = useCameraPermissions();
  const [scanning, setScanning] = useState(false);
  const [reference, setReference] = useState('');
  const [pin, setPin] = useState('');
  const [found, setFound] = useState<CheckinLookup | null>(null);
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const handled = useRef(false); // the camera reports the same code many times a second
  // No internet: a genuine voucher can still be accepted on the phone, and the check-in is sent later.
  const [offlineFound, setOfflineFound] = useState<{ payload: VoucherPayload; token: string } | null>(null);
  const [savedOffline, setSavedOffline] = useState(false);
  const queue = useOffline((s) => s.queue);
  const problems = useOffline((s) => s.problems);

  const reset = () => {
    setFound(null);
    setOfflineFound(null);
    setSavedOffline(false);
    setDone(false);
    setError(null);
    setReference('');
    setPin('');
  };

  async function lookup(query: CheckinQuery) {
    setBusy(true);
    setError(null);
    try {
      setFound(await api.lookupCheckin(query));
    } catch (e) {
      if (e instanceof ApiError && e.code === 'NETWORK' && 'token' in query) {
        const off = useOffline.getState();
        if (!off.publicKey) {
          setError(t('partner.offline_no_key'));
        } else {
          const verdict = judgeVoucher(query.token, off.publicKey, off.hotelIds, Date.now(), todayAddis());
          if (verdict.ok) setOfflineFound({ payload: verdict.payload, token: query.token });
          else setError(t(`partner.offline_reason_${verdict.reason}`));
        }
      } else {
        setError(t(checkinErrorKey(e)));
      }
    } finally {
      setBusy(false);
    }
  }

  function saveOffline() {
    if (!offlineFound) return;
    useOffline.getState().enqueue({ token: offlineFound.token, reference: offlineFound.payload.code, at: new Date().toISOString() });
    setSavedOffline(true);
  }

  async function confirm() {
    if (!found) return;
    setBusy(true);
    setError(null);
    try {
      await api.checkInGuest(found.booking.id);
      setDone(true);
      qc.invalidateQueries({ queryKey: ['inbox'] });
    } catch (e) {
      setError(t(checkinErrorKey(e)));
    } finally {
      setBusy(false);
    }
  }

  async function startScan() {
    if (!permission?.granted && !(await requestPermission()).granted) return;
    handled.current = false;
    setError(null);
    setScanning(true);
  }

  if (scanning) {
    return (
      <SafeAreaView style={{ flex: 1, backgroundColor: '#000' }}>
        <CameraView
          style={{ flex: 1 }}
          facing="back"
          barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
          onBarcodeScanned={({ data }) => {
            if (handled.current) return;
            handled.current = true;
            setScanning(false);
            lookup({ token: data });
          }}
        />
        <View style={{ padding: spacing.lg, backgroundColor: colors.background, gap: spacing.md }}>
          <T style={{ textAlign: 'center' }}>{t('partner.checkin_aim')}</T>
          <Button variant="secondary" title={t('partner.checkin_stop_scan')} onPress={() => setScanning(false)} />
        </View>
      </SafeAreaView>
    );
  }

  const b = found?.booking;
  const state = found?.state;
  const cameraBlocked = permission && !permission.granted && !permission.canAskAgain;

  return (
    <View style={{ flex: 1 }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
        <ScrollView contentContainerStyle={{ paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
          <HeaderBand eyebrow={t('partner.tab_checkin')} title={t('partner.checkin_title')} />
          <View style={{ padding: spacing.lg }}>

          {queue.length > 0 ? (
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.warningSoft }}>
              <Ionicons name="cloud-upload-outline" size={20} color={colors.warning} />
              <T style={{ flex: 1 }}>{t('partner.offline_waiting', { count: queue.length })}</T>
            </View>
          ) : null}
          {problems.map((p) => (
            <View key={p.reference} style={{ marginTop: spacing.sm, padding: spacing.md, borderRadius: radius.md, borderWidth: 1.5, borderColor: colors.danger, backgroundColor: colors.dangerSoft }}>
              <T variant="h3" color={colors.danger}>{p.reference}</T>
              <T color={colors.ink}>{t(`partner.offline_result_${p.result}`)}</T>
              <Button variant="ghost" title={t('partner.offline_dismiss')} onPress={() => useOffline.getState().dismiss(p.reference)} />
            </View>
          ))}

          {offlineFound ? (
            <Reveal style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.lg, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
              <T variant="h2">{offlineFound.payload.code}</T>
              <T color={colors.inkSoft}>{offlineFound.payload.in} – {offlineFound.payload.out}</T>
              <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md }}>
                <Ionicons name={savedOffline ? 'checkmark-circle' : 'cloud-offline-outline'} size={20} color={savedOffline ? colors.success : colors.warning} />
                <T style={{ flex: 1 }} color={savedOffline ? colors.success : colors.ink}>{t(savedOffline ? 'partner.offline_saved' : 'partner.offline_body')}</T>
              </View>
              {savedOffline ? (
                <Button title={t('partner.checkin_next')} onPress={reset} style={{ marginTop: spacing.lg }} />
              ) : (
                <>
                  <Button title={t('partner.offline_save')} icon="log-in-outline" onPress={saveOffline} style={{ marginTop: spacing.lg }} />
                  <Button variant="ghost" title={t('common.back')} onPress={reset} style={{ marginTop: spacing.sm }} />
                </>
              )}
            </Reveal>
          ) : done && b ? (
            <Pop style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.lg, backgroundColor: colors.primarySoft, alignItems: 'center' }}>
              <Ionicons name="checkmark-circle" size={56} color={colors.success} />
              <T variant="h2" style={{ marginTop: spacing.sm }}>{t('partner.checkin_done')}</T>
              <T color={colors.inkSoft} style={{ textAlign: 'center' }}>{t('partner.checkin_done_body', { name: b.guest_name })}</T>
              {b.balance_due_santim > 0 ? (
                <T color={colors.warning} style={{ textAlign: 'center', marginTop: spacing.sm }}>{t('partner.checkin_collect', { amount: price(b.balance_due_santim, b.currency) })}</T>
              ) : null}
              <Button title={t('partner.checkin_next')} onPress={reset} style={{ marginTop: spacing.lg, alignSelf: 'stretch' }} />
            </Pop>
          ) : b && state ? (
            <Reveal style={{ marginTop: spacing.lg, padding: spacing.lg, borderRadius: radius.lg, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
              <T variant="h2">{b.guest_name}</T>
              <StayLine b={b} />
              {state === 'ready' || state === 'checked_in' ? (
                b.balance_due_santim > 0 ? (
                  <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.primarySoft }}>
                    <Ionicons name="cash-outline" size={20} color={colors.primaryInk} />
                    <T style={{ flex: 1 }}>{t('partner.checkin_collect', { amount: price(b.balance_due_santim, b.currency) })}</T>
                  </View>
                ) : null
              ) : null}
              {state === 'ready' ? (
                <Button title={t('partner.checkin_confirm')} icon="log-in-outline" onPress={confirm} loading={busy} style={{ marginTop: spacing.lg }} />
              ) : (
                <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md }}>
                  <Ionicons name={state === 'checked_in' ? 'checkmark-circle' : 'alert-circle-outline'} size={20} color={state === 'checked_in' ? colors.success : colors.danger} />
                  <T style={{ flex: 1 }} color={state === 'checked_in' ? colors.success : colors.danger}>
                    {t(state === 'checked_in' ? 'partner.checkin_already' : `partner.checkin_state_${state}`)}
                  </T>
                </View>
              )}
              <Button variant="ghost" title={t('common.back')} onPress={reset} style={{ marginTop: spacing.sm }} />
            </Reveal>
          ) : (
            <>
              <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.checkin_subtitle')}</T>
              <Button title={t('partner.checkin_scan')} icon="qr-code-outline" onPress={startScan} style={{ marginTop: spacing.lg }} />
              {cameraBlocked ? (
                <View style={{ marginTop: spacing.sm }}>
                  <T variant="small" color={colors.danger}>{t('partner.checkin_camera_title')}. {t('partner.checkin_camera_body')}</T>
                </View>
              ) : null}

              <T variant="label" color={colors.muted} style={{ marginTop: spacing.xl, textAlign: 'center' }}>{t('partner.checkin_or').toUpperCase()}</T>
              <Field
                label={t('partner.checkin_reference')}
                value={reference}
                onChangeText={(v) => setReference(v.toUpperCase())}
                autoCapitalize="characters"
                autoCorrect={false}
                placeholder="BD-XXXXXX"
                maxLength={9}
              />
              <Field label={t('partner.checkin_pin')} value={pin} onChangeText={(v) => setPin(v.replace(/[^0-9]/g, ''))} keyboardType="number-pad" maxLength={4} secureTextEntry />
              <Button
                title={t('partner.checkin_find')}
                variant="secondary"
                onPress={() => lookup({ reference, pin })}
                loading={busy}
                disabled={reference.trim().length < 5 || pin.length !== 4}
                style={{ marginTop: spacing.lg }}
              />
            </>
          )}

          {error ? (
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md }} accessibilityRole="alert">
              <Ionicons name="alert-circle-outline" size={18} color={colors.danger} />
              <T variant="small" color={colors.danger} style={{ flex: 1 }}>{error}</T>
            </View>
          ) : null}
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </View>
  );
}
