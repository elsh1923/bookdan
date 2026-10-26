import { Ionicons } from '@expo/vector-icons';
import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import { useEffect, useState } from 'react';
import { View } from 'react-native';
import type { IdSide } from '@bookdan/domain';
import type { ImageSource } from '@bookdan/api-client';
import { useTranslation } from '@bookdan/i18n';
import { Button, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from './api';

/**
 * One side of the guest's ID photo: shows what is saved, and lets them take a new picture or choose one. The picture goes to the server
 * as base64 (the server re-draws it and stores it encrypted); the app never keeps a copy on the phone.
 */
export function IdPhotoSlot({ side, label, exists, onChanged }: { side: IdSide; label: string; exists: boolean; onChanged: () => void }) {
  const { t } = useTranslation();
  const [source, setSource] = useState<ImageSource | null>(null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const [version, setVersion] = useState(0); // changes after an upload so the picture is fetched again
  // Right after adding or removing a photo the saved profile has not been re-read yet (or does not exist yet), so the slot remembers it itself.
  const [added, setAdded] = useState<boolean | null>(null);
  const has = added ?? exists;

  useEffect(() => {
    let live = true;
    if (!has) {
      setSource(null);
      return;
    }
    api.idPhotoSource(side).then((s) => live && setSource(s), () => live && setSource(null));
    return () => { live = false; };
  }, [has, side, version]);

  async function pick(from: 'camera' | 'library') {
    setNote(null);
    const perm = from === 'camera' ? await ImagePicker.requestCameraPermissionsAsync() : await ImagePicker.requestMediaLibraryPermissionsAsync();
    if (!perm.granted) return setNote(t('details.photo_permission'));
    const opts: ImagePicker.ImagePickerOptions = { mediaTypes: ['images'], quality: 0.6, base64: true, exif: false };
    const res = from === 'camera' ? await ImagePicker.launchCameraAsync(opts) : await ImagePicker.launchImageLibraryAsync(opts);
    const asset = !res.canceled ? res.assets[0] : undefined;
    if (!asset?.base64) return;
    setBusy(true);
    try {
      await api.uploadIdPhoto(side, asset.base64);
      setAdded(true);
      setVersion((v) => v + 1);
      onChanged();
    } catch {
      setNote(t('details.photo_failed'));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      await api.deleteIdPhoto(side);
      setAdded(false);
      onChanged();
    } catch {
      setNote(t('details.photo_failed'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <View style={{ marginTop: spacing.md, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
      <View style={{ flexDirection: 'row', gap: spacing.md, alignItems: 'center' }}>
        <View style={{ width: 96, height: 64, borderRadius: radius.sm, overflow: 'hidden', backgroundColor: colors.fill, alignItems: 'center', justifyContent: 'center' }}>
          {has && source ? (
            <Image source={source} cachePolicy="none" contentFit="cover" style={{ width: '100%', height: '100%' }} accessibilityLabel={label} />
          ) : (
            <Ionicons name="id-card-outline" size={28} color={colors.muted} />
          )}
        </View>
        <View style={{ flex: 1 }}>
          <T variant="h3">{label}</T>
          {has ? (
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
              <Ionicons name="checkmark-circle" size={16} color={colors.success} />
              <T variant="small" color={colors.success}>{t('details.photo_added')}</T>
            </View>
          ) : null}
        </View>
      </View>
      <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md }}>
        <Button variant="secondary" icon="camera-outline" title={has ? t('details.photo_replace') : t('details.photo_take')} onPress={() => pick('camera')} loading={busy} style={{ flex: 1, paddingHorizontal: spacing.md }} />
        <Button variant="secondary" icon="images-outline" title={t('details.photo_choose')} onPress={() => pick('library')} disabled={busy} style={{ flex: 1, paddingHorizontal: spacing.md }} />
      </View>
      {has ? <Button variant="ghost" title={t('details.photo_remove')} onPress={remove} disabled={busy} style={{ marginTop: spacing.xs }} /> : null}
      {note ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.sm }}>{note}</T> : null}
    </View>
  );
}
