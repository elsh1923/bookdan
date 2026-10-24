import { Ionicons } from '@expo/vector-icons';
import { useQuery } from '@tanstack/react-query';
import * as Location from 'expo-location';
import { useRouter } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { Alert, Modal, Pressable, ScrollView, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { addDays, nightsBetween, todayAddis, type Suggestion } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, T, colors, fonts, radius, shadow, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { DateRangePicker } from '../src/DateRangePicker';
import { ethiopianLine, shortDate } from '../src/format';
import { usePrefs } from '../src/store';

// Shown if the server cannot be reached.
const POPULAR = ['Meskel Square', 'Bole', 'Lake Tana', 'Bete Giyorgis', 'Fasil', 'Hawassa'];

function useDebounced<V>(value: V, ms: number): V {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

function Counter({ label, value, onChange, min, max }: { label: string; value: string; onChange: (d: number) => void; min?: boolean; max?: boolean }) {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', minHeight: 56, borderBottomWidth: 1, borderBottomColor: colors.line }}>
      <T variant="h3">{label}</T>
      <View style={{ flexDirection: 'row', alignItems: 'center' }}>
        <Pressable accessibilityLabel={`${label} -`} disabled={min} onPress={() => onChange(-1)} style={{ width: 48, height: 48, alignItems: 'center', justifyContent: 'center', opacity: min ? 0.3 : 1 }}>
          <T variant="h2" color={colors.primaryInk}>−</T>
        </Pressable>
        <T variant="h3" style={{ minWidth: 36, textAlign: 'center' }}>{value}</T>
        <Pressable accessibilityLabel={`${label} +`} disabled={max} onPress={() => onChange(1)} style={{ width: 48, height: 48, alignItems: 'center', justifyContent: 'center', opacity: max ? 0.3 : 1 }}>
          <T variant="h2" color={colors.primaryInk}>+</T>
        </Pressable>
      </View>
    </View>
  );
}

function Tile({ icon, label, value, sub, onPress }: { icon: keyof typeof Ionicons.glyphMap; label: string; value: string; sub?: string | null; onPress: () => void }) {
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={`${label}: ${value}`} onPress={onPress} style={{ flex: 1, minWidth: 150, backgroundColor: colors.fill, borderRadius: radius.md, padding: spacing.md }}>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.xs }}>
        <Ionicons name={icon} size={16} color={colors.inkSoft} />
        <T variant="label" color={colors.muted}>{label}</T>
      </View>
      <T variant="h3" style={{ marginTop: 2 }} numberOfLines={2}>{value}</T>
      {sub ? <T variant="label" color={colors.muted}>{sub}</T> : null}
    </Pressable>
  );
}

const KIND_ICON: Record<Suggestion['kind'], keyof typeof Ionicons.glyphMap> = {
  landmark: 'flag-outline',
  area: 'location-outline',
  city: 'business-outline',
  hotel: 'bed-outline',
};

export default function SearchSheet() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const { recentSearches, addRecent, clearRecent } = usePrefs();
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
  const lang = i18n.language === 'am' ? 'am' : 'en';

  // Defaults: tonight plus one, two adults, one room.
  const today = useMemo(() => todayAddis(), []);
  const [q, setQ] = useState('');
  const [checkIn, setCheckIn] = useState(today);
  const [checkOut, setCheckOut] = useState(addDays(today, 1));
  const [adults, setAdults] = useState(2);
  const [children, setChildren] = useState(0);
  const [rooms, setRooms] = useState(1);
  const [pickDates, setPickDates] = useState(false);
  const [pickGuests, setPickGuests] = useState(false);
  const guests = adults + children;
  const nights = nightsBetween(checkIn, checkOut);

  // Suggestions come from the server as the guest types (about 300 ms after they pause); with nothing typed they are popular places.
  const debouncedQ = useDebounced(q.trim(), 300);
  const suggest = useQuery({ queryKey: ['suggest', debouncedQ], queryFn: () => api.suggest(debouncedQ), staleTime: 5 * 60_000, retry: false });
  const name = (s: Suggestion) => (lang === 'am' && s.text_am ? s.text_am : s.text);

  const stay = { in: checkIn, out: checkOut, guests: String(guests), children: String(children), rooms: String(rooms) };
  const go = (query: string) => {
    addRecent(query);
    router.replace({ pathname: '/results', params: { q: query, ...stay } });
  };
  const pick = (s: Suggestion) => {
    if (s.kind === 'hotel' && s.property_id) {
      addRecent(s.text);
      router.replace({ pathname: '/property/[id]', params: { id: s.property_id, ...stay } });
    } else {
      setQ(s.text);
      go(s.text);
    }
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
      router.replace({ pathname: '/results', params: { q: '', lat: String(here.coords.latitude), lng: String(here.coords.longitude), ...stay } });
    } catch {
      Alert.alert(t('search.use_gps'), t('search.gps_failed'));
    } finally {
      setLocating(false);
    }
  };

  const close = () => (router.canGoBack() ? router.back() : router.replace('/'));
  const typed = debouncedQ.length > 0;
  const popular = !typed ? (suggest.data ?? []).map(name) : [];
  const dateSummary = `${shortDate(checkIn, months)} – ${shortDate(checkOut, months)}`;

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
          {typed ? (suggest.data ?? []).map((s) => (
            <Pressable key={`${s.kind}-${s.text}-${s.property_id ?? ''}`} accessibilityRole="button" onPress={() => pick(s)} style={{ minHeight: 52, justifyContent: 'center', paddingHorizontal: spacing.sm }}>
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                <Ionicons name={KIND_ICON[s.kind]} size={18} color={colors.inkSoft} />
                <View style={{ flex: 1 }}>
                  <T numberOfLines={1}>{name(s)}</T>
                  <T variant="label" color={colors.muted} numberOfLines={1}>{s.kind === 'hotel' ? `${t('search.suggest_hotel')} · ${s.subtitle}` : s.subtitle}</T>
                </View>
              </View>
            </Pressable>
          )) : null}

          <Pressable onPress={useGps} accessibilityRole="button" style={{ minHeight: 44, justifyContent: 'center', paddingHorizontal: spacing.sm }}>
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
              <Ionicons name="navigate-outline" size={16} color={colors.primaryInk} />
              <T color={colors.primaryInk} variant="small">{locating ? t('search.locating') : t('search.use_gps')}</T>
            </View>
          </Pressable>

          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm }}>
            <Tile icon="calendar-outline" label={t('search.dates')} value={dateSummary} sub={`${t('common.night', { count: nights })}${ethiopianLine(checkIn, lang) ? ` · ${ethiopianLine(checkIn, lang)}` : ''}`} onPress={() => setPickDates(true)} />
            <Tile
              icon="people-outline"
              label={t('search.travellers')}
              value={`${t('common.room', { count: rooms })} · ${t('search.adults', { count: adults })}`}
              sub={children > 0 ? t('search.children', { count: children }) : null}
              onPress={() => setPickGuests(true)}
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
          <T variant="h3">{t('search.popular_places')}</T>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.sm }}>
            {(popular.length ? popular : POPULAR).map((p) => <Chip key={p} label={p} onPress={() => go(p)} />)}
          </View>
        </View>
      </ScrollView>

      {/* The calendar: tap the check-in day, then the check-out day. */}
      <Modal visible={pickDates} animationType="slide" onRequestClose={() => setPickDates(false)}>
        <DatesSheet
          today={today}
          start={checkIn}
          end={checkOut}
          onClose={() => setPickDates(false)}
          onDone={(s, e) => { setCheckIn(s); setCheckOut(e); setPickDates(false); }}
        />
      </Modal>

      {/* Rooms, adults and children. */}
      <Modal visible={pickGuests} animationType="slide" transparent onRequestClose={() => setPickGuests(false)}>
        <Pressable style={{ flex: 1, backgroundColor: 'rgba(0,0,0,0.4)' }} onPress={() => setPickGuests(false)} />
        <View style={{ backgroundColor: colors.surface, borderTopLeftRadius: radius.lg, borderTopRightRadius: radius.lg, padding: spacing.xl, width: '100%', maxWidth: 640, alignSelf: 'center' }}>
          <T variant="h2">{t('search.travellers')}</T>
          <Counter label={t('search.rooms')} value={String(rooms)} min={rooms <= 1} max={rooms >= 9} onChange={(d) => { const n = rooms + d; setRooms(n); if (adults < n) setAdults(n); }} />
          <Counter label={t('search.adults_label')} value={String(adults)} min={adults <= rooms} max={adults >= 20} onChange={(d) => setAdults(adults + d)} />
          <Counter label={t('search.children_label')} value={String(children)} min={children <= 0} max={children >= 10} onChange={(d) => setChildren(children + d)} />
          <T variant="small" color={colors.muted} style={{ marginTop: spacing.sm }}>{t('search.children_hint')}</T>
          <Button title={t('search.date_done')} onPress={() => setPickGuests(false)} style={{ marginTop: spacing.lg }} />
        </View>
      </Modal>
    </SafeAreaView>
  );
}

/** The calendar screen: what to tap next, the nights chosen, and Done once both days are set. */
function DatesSheet({ today, start, end, onClose, onDone }: { today: string; start: string; end: string; onClose: () => void; onDone: (s: string, e: string) => void }) {
  const { t } = useTranslation();
  const [s, setS] = useState<string | null>(start);
  const [e, setE] = useState<string | null>(end);
  const [long, setLong] = useState(false);
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
  const hint = !s || e ? t('search.date_pick_start') : t('search.date_pick_end');
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }}>
      <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.lg }}>
        <T variant="h2">{t('search.date_title')}</T>
        <Pressable accessibilityLabel={t('common.cancel')} onPress={onClose} style={{ width: 44, height: 44, alignItems: 'center', justifyContent: 'center' }}>
          <Ionicons name="close" size={24} color={colors.ink} />
        </Pressable>
      </View>
      <T variant="small" color={long ? colors.danger : colors.inkSoft} style={{ paddingHorizontal: spacing.lg }}>{long ? t('search.date_max') : hint}</T>
      <View style={{ flex: 1 }}>
        <DateRangePicker start={s} end={e} today={today} onChange={(a, b) => { setS(a); setE(b); setLong(false); }} onTooLong={() => setLong(true)} />
      </View>
      <View style={{ padding: spacing.lg, borderTopWidth: 1, borderTopColor: colors.line }}>
        <Button
          title={s && e ? `${t('search.date_done')} · ${shortDate(s, months)} – ${shortDate(e, months)} · ${t('common.night', { count: nightsBetween(s, e) })}` : t('search.date_done')}
          disabled={!s || !e}
          onPress={() => s && e && onDone(s, e)}
        />
      </View>
    </SafeAreaView>
  );
}
