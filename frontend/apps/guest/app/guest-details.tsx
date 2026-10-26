import { Ionicons } from '@expo/vector-icons';
import { useQueryClient } from '@tanstack/react-query';
import { Redirect, useRouter } from 'expo-router';
import { useEffect, useRef, useState } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { ID_TYPES, maskId, normalizeIdNumber, type IdType } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { useAuth } from '../src/auth';
import { Field } from '../src/Field';
import { useProfile } from '../src/queries';

/**
 * The guest's name, ID and nationality. They enter it once; when they request a room, the hotel receives a copy and
 * sees it in the partner app. The app never gets the full number back after saving: only the last four characters,
 * so changing the ID means typing it again.
 */
export default function GuestDetails() {
  const { t } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const token = useAuth((s) => s.token);
  const profile = useProfile(!!token);

  const [fullName, setFullName] = useState('');
  const [idType, setIdType] = useState<IdType>('national_id');
  const [idNumber, setIdNumber] = useState('');
  const [nationality, setNationality] = useState('Ethiopian');
  const [errors, setErrors] = useState<{ name?: string; id?: string; nationality?: string; form?: string }>({});
  const [busy, setBusy] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);

  // Fill in what the guest saved before (the ID number is never sent back, so that field stays empty).
  const filled = useRef(false);
  useEffect(() => {
    if (filled.current || !profile.data) return;
    filled.current = true;
    setFullName(profile.data.full_name);
    setIdType(profile.data.id_type);
    setNationality(profile.data.nationality);
  }, [profile.data]);

  if (!token) return <Redirect href={{ pathname: '/signin', params: { next: '/guest-details' } }} />;

  const close = () => (router.canGoBack() ? router.back() : router.replace('/'));

  async function save() {
    const next: typeof errors = {};
    if (fullName.trim().length < 2) next.name = t('details.name_required');
    const normalized = normalizeIdNumber(idNumber);
    if (!normalized) next.id = t('details.invalid_id');
    if (nationality.trim().length < 2) next.nationality = t('details.nationality_required');
    setErrors(next);
    if (Object.keys(next).length > 0 || !normalized) return;

    setBusy(true);
    try {
      await api.saveProfile({ full_name: fullName.trim(), id_type: idType, id_number: normalized, nationality: nationality.trim() });
      setIdNumber(''); // gone from this screen as soon as it is saved
      await qc.invalidateQueries({ queryKey: ['profile'] });
      close();
    } catch (e) {
      const key =
        e instanceof ApiError && e.code === 'NETWORK' ? 'results.offline_body'
          : e instanceof ApiError && e.code === 'INVALID_ID' ? 'details.invalid_id'
          : 'results.error_body';
      setErrors({ form: t(key) });
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!confirmRemove) return setConfirmRemove(true); // a second tap confirms
    setBusy(true);
    try {
      await api.deleteProfile();
      await qc.invalidateQueries({ queryKey: ['profile'] });
      close();
    } catch {
      setErrors({ form: t('results.error_body') });
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
          <T variant="display">{t('details.title')}</T>
          <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('details.subtitle')}</T>

          <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.lg, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.primarySoft }}>
            <Ionicons name="lock-closed-outline" size={20} color={colors.primaryInk} />
            <T variant="small" color={colors.primaryInk} style={{ flex: 1 }}>{t('details.privacy')}</T>
          </View>

          {profile.data ? (
            <View style={{ flexDirection: 'row', gap: spacing.sm, alignItems: 'center', marginTop: spacing.md }}>
              <Ionicons name="checkmark-circle" size={20} color={colors.success} />
              <T variant="small" color={colors.success} style={{ flex: 1 }}>
                {t('details.saved_as', { type: t(`idType.${profile.data.id_type}`), masked: maskId(profile.data.id_last4) })}
              </T>
            </View>
          ) : null}

          <Field label={t('details.full_name')} hint={t('details.full_name_hint')} value={fullName} onChangeText={setFullName} autoCapitalize="words" autoComplete="name" textContentType="name" error={errors.name} />

          <T variant="label" color={colors.muted} style={{ marginTop: spacing.lg }}>{t('details.id_type').toUpperCase()}</T>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
            {ID_TYPES.map((it) => (
              <Chip key={it} label={t(`idType.${it}`)} selected={idType === it} onPress={() => setIdType(it)} />
            ))}
          </View>

          <Field
            label={t('details.id_number')}
            hint={profile.data ? t('details.id_number_again') : t('details.id_number_hint')}
            value={idNumber}
            onChangeText={setIdNumber}
            autoCapitalize="characters"
            autoCorrect={false}
            autoComplete="off"
            spellCheck={false}
            importantForAutofill="no"
            error={errors.id}
          />

          <Field label={t('details.nationality')} value={nationality} onChangeText={setNationality} autoComplete="off" error={errors.nationality} />

          {errors.form ? (
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md, padding: spacing.md, borderRadius: radius.md, backgroundColor: colors.dangerSoft }}>
              <Ionicons name="alert-circle-outline" size={20} color={colors.danger} />
              <T variant="small" color={colors.danger} style={{ flex: 1 }}>{errors.form}</T>
            </View>
          ) : null}

          <Button title={t('details.save')} icon="shield-checkmark-outline" onPress={save} loading={busy} style={{ marginTop: spacing.xl }} />
          {profile.data ? (
            <Button variant="ghost" title={confirmRemove ? t('details.remove_confirm') : t('details.remove')} onPress={remove} disabled={busy} style={{ marginTop: spacing.sm }} />
          ) : null}
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
