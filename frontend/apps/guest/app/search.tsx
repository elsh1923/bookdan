import { Ionicons } from '@expo/vector-icons';
import { Image } from 'expo-image';
import { useQuery } from '@tanstack/react-query';
import * as Location from 'expo-location';
import { useRouter } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { Alert, Modal, Pressable, ScrollView, Text, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { addDays, nightsBetween, todayAddis, type Suggestion } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, T, colors, fonts, radius, shadow, spacing } from '@bookdan/ui';
import { api } from '../src/api';
import { CurrencyChip } from '../src/CurrencyChip';
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

/** The date row shows the day ("26 Oct '26") and, under it, the weekday (weekdays start on Monday). */
function dayLabel(iso: string, months: string[], weekdays: string[]): { day: string; weekday: string } {
  const [y, m, d] = iso.split('-').map(Number);
  return {
    day: `${d} ${months[m - 1]} '${String(y).slice(2)}`,
    weekday: weekdays[(new Date(Date.UTC(y, m - 1, d)).getUTCDay() + 6) % 7],
  };
}

/** One side of the date row: the day in bold, the weekday under it. */
function DateSide({ iso, months, weekdays, align }: { iso: string; months: string[]; weekdays: string[]; align: 'left' | 'right' }) {
  const { day, weekday } = dayLabel(iso, months, weekdays);
  return (
    <View style={{ flex: 1, alignItems: align === 'left' ? 'flex-start' : 'flex-end' }}>
      <T variant="h3" numberOfLines={1}>{day}</T>
      <T variant="label" color={colors.muted}>{weekday}</T>
    </View>
  );
}

/** Shows the typed part of a place in bold, the way the suggestion list does. */
function Highlight({ text, query }: { text: string; query: string }) {
  const q = query.trim().toLowerCase();
  const i = q ? text.toLowerCase().indexOf(q) : -1;
  if (i < 0) return <T numberOfLines={1}>{text}</T>;
  return (
    <T numberOfLines={1}>
      {text.slice(0, i)}
      <Text style={{ fontFamily: fonts.bold }}>{text.slice(i, i + q.length)}</Text>
      {text.slice(i + q.length)}
    </T>
  );
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

/** One tappable line of the search card: an icon and bold text. It takes the free width, so long text never pushes other buttons off the card. */
function Row({ icon, onPress, label, children }: { icon: keyof typeof Ionicons.glyphMap; onPress: () => void; label: string; children: React.ReactNode }) {
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={label} onPress={onPress} style={{ flex: 1, minWidth: 0, flexDirection: 'row', alignItems: 'center', gap: spacing.md, minHeight: 60, paddingHorizontal: spacing.md }}>
      <Ionicons name={icon} size={22} color={colors.inkSoft} />
      {children}
    </Pressable>
  );
}

function Divider() {
  return <View style={{ height: 1, backgroundColor: colors.line, marginHorizontal: spacing.md }} />;
}

// Places get a map pin, as in the suggestion list of the big travel apps; a hotel gets a building.
const KIND_ICON: Record<Suggestion['kind'], keyof typeof Ionicons.glyphMap> = {
  landmark: 'location-outline',
  area: 'location-outline',
  city: 'location-outline',
  hotel: 'business-outline',
};

export default function SearchSheet() {
  const { t, i18n } = useTranslation();
  const router = useRouter();
  const { recentSearches, addRecent, clearRecent } = usePrefs();
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
  const weekdays = t('weekdays.short', { returnObjects: true }) as unknown as string[];
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
  const [pickPlace, setPickPlace] = useState(false);
  const guests = adults + children;
  const nights = nightsBetween(checkIn, checkOut);

  // Suggestions come from the server as the guest types (about 300 ms after they pause); with nothing typed they are popular places.
  const debouncedQ = useDebounced(q.trim(), 300);
  const suggest = useQuery({ queryKey: ['suggest', debouncedQ], queryFn: () => api.suggest(debouncedQ), staleTime: 5 * 60_000, retry: false });
  const name = (s: Suggestion) => (lang === 'am' && s.text_am ? s.text_am : s.text);

  const stay = { in: checkIn, out: checkOut, guests: String(guests), children: String(children), rooms: String(rooms) };
  const go = (query: string) => {
    addRecent(query);
    setPickPlace(false);
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
  const roomsLine = `${t('common.room', { count: rooms })}, ${t('search.adults', { count: adults })} & ${t('search.children', { count: children })}`;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: colors.canvas }} edges={['top']}>
      {/* Title bar: back, the page name, and the BookDan mark. */}
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.md, paddingVertical: spacing.sm, backgroundColor: colors.surface }}>
        <Pressable accessibilityLabel={t('common.cancel')} onPress={close} hitSlop={8} style={{ width: 40, height: 40, alignItems: 'center', justifyContent: 'center' }}>
          <Ionicons name="arrow-back" size={24} color={colors.ink} />
        </Pressable>
        <T variant="h2" style={{ flex: 1 }}>{t('search.title')}</T>
        <CurrencyChip />
        <Image source={require('../assets/icon.png')} accessibilityLabel="BookDan" style={{ width: 32, height: 32, borderRadius: 8 }} />
      </View>

      <ScrollView contentContainerStyle={{ padding: spacing.md, paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
        <View style={[{ backgroundColor: colors.surface, borderRadius: radius.lg, paddingVertical: spacing.xs, borderWidth: 1, borderColor: colors.line }, shadow.card]}>
          {/* Where: tapping opens the place search. Near Me finds hotels close to the phone. */}
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingRight: spacing.sm }}>
            <Row icon="search" label={t('search.heading')} onPress={() => setPickPlace(true)}>
              <T variant="h3" numberOfLines={1} color={q.trim() ? colors.ink : colors.muted} style={{ flex: 1 }}>{q.trim() || t('search.placeholder')}</T>
            </Row>
            <Pressable onPress={useGps} accessibilityRole="button" style={{ flexDirection: 'row', alignItems: 'center', gap: 4, minHeight: 40, paddingHorizontal: spacing.md, borderRadius: radius.sm, borderWidth: 1, borderColor: colors.line }}>
              <Ionicons name="locate-outline" size={16} color={colors.inkSoft} />
              <T variant="small" color={colors.inkSoft}>{locating ? t('search.locating') : t('search.use_gps')}</T>
            </Pressable>
          </View>
          <Divider />

          {/* When: check-in and check-out with the number of nights between them. */}
          <Row icon="calendar-outline" label={t('search.dates')} onPress={() => setPickDates(true)}>
            <View style={{ flex: 1, flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
              <DateSide iso={checkIn} months={months} weekdays={weekdays} align="left" />
              <View style={{ paddingHorizontal: spacing.sm, paddingVertical: 2, borderRadius: radius.pill, borderWidth: 1, borderColor: colors.line }}>
                <T variant="label" color={colors.inkSoft}>{t('common.night', { count: nights }).toUpperCase()}</T>
              </View>
              <DateSide iso={checkOut} months={months} weekdays={weekdays} align="right" />
            </View>
          </Row>
          {ethiopianLine(checkIn, lang) ? <T variant="label" color={colors.muted} style={{ paddingLeft: 58, marginBottom: spacing.xs }}>{ethiopianLine(checkIn, lang)}</T> : null}
          <Divider />

          {/* Who: rooms, adults and children. */}
          <Row icon="person-outline" label={t('search.travellers')} onPress={() => setPickGuests(true)}>
            <T variant="h3" numberOfLines={1} style={{ flex: 1 }}>{roomsLine}</T>
          </Row>

          <View style={{ padding: spacing.md, paddingTop: spacing.sm }}>
            <Button title={t('search.submit').toUpperCase()} onPress={() => go(q)} />
          </View>
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

      {/* The place search: type, then pick a suggestion or search the text as it is. */}
      <Modal visible={pickPlace} animationType="slide" onRequestClose={() => setPickPlace(false)}>
        <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }} edges={['top']}>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingHorizontal: spacing.md, paddingVertical: spacing.sm, borderBottomWidth: 1, borderBottomColor: colors.line }}>
            <Pressable accessibilityLabel={t('common.cancel')} onPress={() => setPickPlace(false)} hitSlop={8} style={{ width: 40, height: 40, alignItems: 'center', justifyContent: 'center' }}>
              <Ionicons name="arrow-back" size={24} color={colors.ink} />
            </Pressable>
            <View style={{ flex: 1, flexDirection: 'row', alignItems: 'center', height: 48, borderRadius: radius.md, borderWidth: 1, borderColor: colors.primary, paddingHorizontal: spacing.md }}>
              <TextInput
                autoFocus
                value={q}
                onChangeText={setQ}
                placeholder={t('search.placeholder')}
                placeholderTextColor={colors.muted}
                returnKeyType="search"
                onSubmitEditing={() => go(q)}
                style={{ flex: 1, fontFamily: fonts.regular, fontSize: 16, color: colors.ink }}
              />
              {q.length > 0 ? (
                <Pressable accessibilityLabel={t('common.cancel')} onPress={() => setQ('')} hitSlop={8}>
                  <Ionicons name="close-circle" size={20} color={colors.muted} />
                </Pressable>
              ) : null}
            </View>
          </View>

          <ScrollView keyboardShouldPersistTaps="handled">
            {q.trim().length > 0 ? (
              <Pressable accessibilityRole="button" onPress={() => go(q.trim())} style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, minHeight: 56, paddingHorizontal: spacing.lg, borderBottomWidth: 1, borderBottomColor: colors.line }}>
                <Ionicons name="search" size={20} color={colors.inkSoft} />
                <T style={{ flex: 1 }} numberOfLines={1}>{q.trim()}</T>
                <Ionicons name="chevron-forward" size={20} color={colors.primaryInk} />
              </Pressable>
            ) : null}

            {!typed && (suggest.data ?? []).length > 0 ? (
              <T variant="label" color={colors.muted} style={{ paddingHorizontal: spacing.lg, paddingTop: spacing.md }}>{t('search.popular').toUpperCase()}</T>
            ) : null}

            {(suggest.data ?? []).map((s) => (
              <Pressable
                key={`${s.kind}-${s.text}-${s.property_id ?? ''}`}
                accessibilityRole="button"
                onPress={() => pick(s)}
                style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.md, minHeight: 64, paddingHorizontal: spacing.lg, borderBottomWidth: 1, borderBottomColor: colors.hairline }}
              >
                <Ionicons name={KIND_ICON[s.kind]} size={22} color={colors.inkSoft} />
                <View style={{ flex: 1 }}>
                  <Highlight text={name(s)} query={q} />
                  <T variant="small" color={colors.muted} numberOfLines={1}>
                    {s.kind === 'hotel' ? `${t('search.suggest_hotel')} · ${s.subtitle}` : s.subtitle}
                  </T>
                </View>
              </Pressable>
            ))}
          </ScrollView>
        </SafeAreaView>
      </Modal>

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
