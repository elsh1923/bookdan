import { Ionicons } from '@expo/vector-icons';
import * as Location from 'expo-location';
import { useRouter } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { Alert, Pressable, ScrollView, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { addDays, nightsBetween, todayAddis } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, T, colors, fonts, radius, shadow, spacing } from '@bookdan/ui';
import { ethiopianLine, shortDate } from '../src/format';
import { usePrefs } from '../src/store';

const POPULAR = ['Meskel Square', 'Bole', 'Lake Tana', 'Bete Giyorgis', 'Fasil', 'Hawassa'];

function useDebounced<V>(value: V, ms: number): V {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

function Stepper({ label, value, onChange, min, max, sub }: {
  label: string; value: string; onChange: (d: number) => void; min?: boolean; max?: boolean; sub?: string | null;
}) {
  return (
    <View style={{ flex: 1, minWidth: 140, backgroundColor: colors.fill, borderRadius: radius.md, paddingVertical: spacing.sm, paddingHorizontal: spacing.md }}>
      <T variant="label" color={colors.muted}>{label}</T>
      <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: spacing.xs }}>
        <Pressable accessibilityLabel={`${label} -`} disabled={min} onPress={() => onChange(-1)} style={{ width: 44, height: 44, alignItems: 'center', justifyContent: 'center', opacity: min ? 0.3 : 1 }}>
          <T variant="h2" color={colors.primaryInk}>−</T>
        </Pressable>
        <View style={{ alignItems: 'center' }}>
          <T variant="h3">{value}</T>
          {sub ? <T variant="label" color={colors.muted}>{sub}</T> : null}
        </View>
        <Pressable accessibilityLabel={`${label} +`} disabled={max} onPress={() => onChange(1)} style={{ width: 44, height: 44, alignItems: 'center', justifyContent: 'center', opacity: max ? 0.3 : 1 }}>
          <T variant="h2" color={colors.primaryInk}>+</T>
        </Pressable>
      </View>
    </View>
  );
}

export default function SearchSheet() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const { recentSearches, addRecent, clearRecent } = usePrefs();
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
  const lang = i18n.language === 'am' ? 'am' : 'en';

  // Defaults: tonight plus one, one guest.
  const today = useMemo(() => todayAddis(), []);
  const [q, setQ] = useState('');
  const [checkIn, setCheckIn] = useState(today);
  const [nights, setNights] = useState(1);
  const [guests, setGuests] = useState(1);
  const [rooms, setRooms] = useState(1);
  const checkOut = addDays(checkIn, nights);

  // Suggestions are debounced about 300 ms so typing never janks.
  const debouncedQ = useDebounced(q, 300);
  const suggestions = debouncedQ.trim().length > 0
    ? POPULAR.filter((p) => p.toLowerCase().includes(debouncedQ.trim().toLowerCase()) && p.toLowerCase() !== debouncedQ.trim().toLowerCase())
    : [];

  const go = (query: string) => {
    addRecent(query);
    router.replace({ pathname: '/results', params: { q: query, in: checkIn, out: checkOut, guests: String(guests), rooms: String(rooms) } });
  };

  // GPS permission is only requested when this button is tapped. The hotels come back nearest first, each with its distance.
  const [locating, setLocating] = useState(false);
  const useGps = async () => {
    setLocating(true);
    try {
      const { status } = await Location.requestForegroundPermissionsAsync();
      if (status !== 'granted') {
        Alert.alert(t('search.use_gps'), t('search.gps_denied'));
        return;
      }
      const here = (await Location.getLastKnownPositionAsync()) ?? (await Location.getCurrentPositionAsync({ accuracy: Location.Accuracy.Balanced }));
      router.replace({ pathname: '/results', params: { q: '', lat: String(here.coords.latitude), lng: String(here.coords.longitude), in: checkIn, out: checkOut, guests: String(guests), rooms: String(rooms) } });
    } catch {
      Alert.alert(t('search.use_gps'), t('search.gps_failed'));
    } finally {
      setLocating(false);
    }
  };

  const close = () => (router.canGoBack() ? router.back() : router.replace('/'));

  return (
    <SafeAreaView style={{ flex: 1 }}>
      <ScrollView contentContainerStyle={{ padding: spacing.lg, paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
        <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' }}>
          <T variant="h1" style={{ flex: 1 }}>{t('search.heading')}</T>
          <Pressable accessibilityLabel={t('common.cancel')} onPress={close} style={{ width: 44, height: 44, alignItems: 'center', justifyContent: 'center', borderRadius: 22, borderWidth: 1, borderColor: colors.line }}>
            <Ionicons name="close" size={22} color={colors.ink} />
          </Pressable>
        </View>

        <View style={[{ marginTop: spacing.lg, backgroundColor: colors.surface, borderRadius: radius.lg, padding: spacing.md }, shadow.card]}>
          <View style={{ flexDirection: 'row', alignItems: 'center', backgroundColor: colors.fill, borderRadius: radius.md, paddingHorizontal: spacing.md }}>
            <Ionicons name="search" size={18} color={colors.ink} />
            <TextInput
              autoFocus
              value={q}
              onChangeText={setQ}
              placeholder={t('search.placeholder')}
              placeholderTextColor={colors.muted}
              returnKeyType="search"
              onSubmitEditing={() => go(q)}
              style={{ flex: 1, minHeight: 52, paddingHorizontal: spacing.sm, fontFamily: fonts.regular, fontSize: 15, color: colors.ink }}
            />
          </View>
          {suggestions.map((s) => (
            <Pressable key={s} onPress={() => { setQ(s); go(s); }} style={{ paddingVertical: spacing.md, paddingHorizontal: spacing.sm }}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                <Ionicons name="location-outline" size={18} color={colors.inkSoft} />
                <T>{s}</T>
              </View>
            </Pressable>
          ))}

          <Pressable onPress={useGps} accessibilityRole="button" style={{ minHeight: 44, justifyContent: 'center', paddingHorizontal: spacing.sm }}>
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
              <Ionicons name="navigate-outline" size={16} color={colors.primaryInk} />
              <T color={colors.primaryInk} variant="small">{locating ? t('search.locating') : t('search.use_gps')}</T>
            </View>
          </Pressable>

          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm }}>
            <Stepper
              label={t('search.check_in')}
              value={shortDate(checkIn, months)}
              sub={ethiopianLine(checkIn, lang)}
              min={checkIn <= today}
              onChange={(d) => setCheckIn(addDays(checkIn, d))}
            />
            <Stepper
              label={t('search.check_out')}
              value={shortDate(checkOut, months)}
              sub={t('common.night', { count: nightsBetween(checkIn, checkOut) })}
              min={nights <= 1}
              max={nights >= 30}
              onChange={(d) => setNights(nights + d)}
            />
            <Stepper
              label={t('search.guests')}
              value={t('common.guest', { count: guests })}
              min={guests <= rooms}
              max={guests >= 10}
              onChange={(d) => setGuests(guests + d)}
            />
            <Stepper
              label={t('search.rooms')}
              value={t('common.room', { count: rooms })}
              min={rooms <= 1}
              max={rooms >= 9}
              onChange={(d) => {
                const next = rooms + d;
                setRooms(next);
                if (guests < next) setGuests(next); // every room has at least one guest
              }}
            />
          </View>

          <Button title={t('search.submit')} onPress={() => go(q)} style={{ marginTop: spacing.md }} />
        </View>

        {recentSearches.length > 0 ? (
          <View style={{ marginTop: spacing.xl }}>
            <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' }}>
              <T variant="h3">{t('search.recent')}</T>
              <Pressable onPress={clearRecent} hitSlop={12}><T variant="small" color={colors.primaryInk}>{t('search.clear_recent')}</T></Pressable>
            </View>
            <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
              {recentSearches.map((r) => <Chip key={r} label={r} onPress={() => go(r)} />)}
            </View>
          </View>
        ) : null}

        <View style={{ marginTop: spacing.xl }}>
          <T variant="h3">{t('search.popular')}</T>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
            {POPULAR.map((p) => <Chip key={p} label={p} onPress={() => go(p)} />)}
          </View>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}
