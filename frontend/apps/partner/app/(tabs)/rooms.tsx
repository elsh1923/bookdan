import { Ionicons } from '@expo/vector-icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Alert, Modal, Pressable, RefreshControl, ScrollView, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { ApiError } from '@bookdan/api-client';
import { addDays, nightsBetween, todayAddis, type ChartNight, type ChartRoom, type NewRoomBlock, type RoomBlock } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Button, Chip, EmptyState, Skeleton, T, colors, radius, spacing } from '@bookdan/ui';
import { api } from '../../src/api';
import { Field } from '../../src/Field';
import { shortDate } from '../../src/format';

const CELL = 76;

/** The weekday letters and day number of a date, Monday first. */
function dayLabel(iso: string, weekdays: string[]): { wd: string; d: number } {
  const dt = new Date(`${iso}T00:00:00Z`);
  return { wd: weekdays[(dt.getUTCDay() + 6) % 7] ?? '', d: dt.getUTCDate() };
}

function Counter({ label, value, onChange, min, max }: { label: string; value: string; onChange: (d: number) => void; min?: boolean; max?: boolean }) {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', minHeight: 52, borderBottomWidth: 1, borderBottomColor: colors.line }}>
      <T variant="h3" style={{ flex: 1 }}>{label}</T>
      <Pressable accessibilityLabel={`${label} -`} disabled={min} onPress={() => onChange(-1)} style={{ width: 48, height: 48, alignItems: 'center', justifyContent: 'center', opacity: min ? 0.3 : 1 }}>
        <T variant="h2" color={colors.primaryInk}>−</T>
      </Pressable>
      <T variant="h3" style={{ minWidth: 80, textAlign: 'center' }}>{value}</T>
      <Pressable accessibilityLabel={`${label} +`} disabled={max} onPress={() => onChange(1)} style={{ width: 48, height: 48, alignItems: 'center', justifyContent: 'center', opacity: max ? 0.3 : 1 }}>
        <T variant="h2" color={colors.primaryInk}>+</T>
      </Pressable>
    </View>
  );
}

function Cell({ night, weekdays, onPress }: { night: ChartNight; weekdays: string[]; onPress: () => void }) {
  const { wd, d } = dayLabel(night.date, weekdays);
  const full = night.free <= 0;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${night.date}: ${night.free}`}
      onPress={onPress}
      style={{ width: CELL, paddingVertical: spacing.sm, alignItems: 'center', borderRadius: radius.md, backgroundColor: full ? colors.dangerSoft : colors.successSoft }}
    >
      <T variant="label" color={colors.inkSoft}>{wd} {d}</T>
      <T variant="h2" color={full ? colors.danger : colors.success}>{night.free}</T>
      <T variant="label" color={colors.muted}>/{night.capacity}</T>
    </Pressable>
  );
}

/** The rooms chart: how many rooms are free each night, a walk-in guest or blocked rooms taken out of sale, and giving them back. */
export default function RoomsScreen() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const months = i18n.t('months.short', { returnObjects: true }) as unknown as string[];
  const weekdays = i18n.t('weekdays.short', { returnObjects: true }) as unknown as string[];
  const [property, setProperty] = useState<string | undefined>();
  const [picked, setPicked] = useState<{ room: ChartRoom; night: ChartNight } | null>(null);
  const [form, setForm] = useState<{ kind: NewRoomBlock['kind']; room: string; date: string } | null>(null);

  const chart = useQuery({ queryKey: ['rooms-chart', property], queryFn: () => api.getRoomsChart({ property_id: property, days: 14 }), refetchInterval: 30_000 });
  const hotelId = property ?? chart.data?.property.id;
  const blocks = useQuery({ queryKey: ['rooms-blocks', hotelId], queryFn: () => api.listRoomBlocks(hotelId), enabled: !!hotelId });
  const refresh = () => Promise.all([qc.invalidateQueries({ queryKey: ['rooms-chart'] }), qc.invalidateQueries({ queryKey: ['rooms-blocks'] })]);

  const release = (b: RoomBlock) =>
    Alert.alert(t('partner.give_back_title'), t('partner.give_back_body'), [
      { text: t('common.cancel'), style: 'cancel' },
      { text: t('partner.give_back'), onPress: async () => { try { await api.releaseRoomBlock(b.id); } finally { refresh(); } } },
    ]);

  return (
    <SafeAreaView style={{ flex: 1 }} edges={['top']}>
      <ScrollView
        contentContainerStyle={{ padding: spacing.lg, paddingBottom: spacing.xxl }}
        refreshControl={<RefreshControl refreshing={chart.isRefetching} onRefresh={refresh} tintColor={colors.primary} />}
      >
        <T variant="display">{t('partner.rooms_title')}</T>
        <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.rooms_hint')}</T>

        {chart.data && chart.data.hotels.length > 1 ? (
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: spacing.sm, paddingTop: spacing.md }}>
            {chart.data.hotels.map((h) => <Chip key={h.id} label={h.name} selected={h.id === chart.data!.property.id} onPress={() => setProperty(h.id)} />)}
          </ScrollView>
        ) : null}

        {chart.isPending ? (
          <View style={{ marginTop: spacing.lg, gap: spacing.md }}><Skeleton height={120} radius={radius.md} /><Skeleton height={120} radius={radius.md} /></View>
        ) : chart.isError ? (
          <EmptyState icon="⚠️" title={t(chart.error instanceof ApiError && chart.error.code === 'NETWORK' ? 'results.offline_title' : 'results.error_title')} body={t(chart.error instanceof ApiError && chart.error.code === 'NETWORK' ? 'results.offline_body' : 'results.error_body')} actionLabel={t('common.retry')} onAction={() => chart.refetch()} />
        ) : (
          <>
            {chart.data.rooms.map((room) => (
              <View key={room.id} style={{ marginTop: spacing.xl }}>
                <View style={{ flexDirection: 'row', alignItems: 'baseline', justifyContent: 'space-between' }}>
                  <T variant="h3">{room.name}</T>
                  <T variant="small" color={colors.muted}>{t('partner.rooms_of', { n: room.total_rooms })}</T>
                </View>
                <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: spacing.xs, paddingTop: spacing.sm }}>
                  {room.nights.map((n) => <Cell key={n.date} night={n} weekdays={weekdays} onPress={() => setPicked({ room, night: n })} />)}
                </ScrollView>
              </View>
            ))}

            <T variant="h3" style={{ marginTop: spacing.xl }}>{t('partner.blocks_title')}</T>
            {blocks.isError ? <T color={colors.danger} style={{ marginTop: spacing.xs }}>{t('results.error_body')} ({String((blocks.error as Error)?.message)})</T> : null}
            {(blocks.data ?? []).length === 0 ? (
              <T color={colors.inkSoft} style={{ marginTop: spacing.xs }}>{t('partner.blocks_none')}</T>
            ) : (
              (blocks.data ?? []).map((b) => (
                <View key={b.id} style={{ marginTop: spacing.sm, padding: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.surface }}>
                  <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm }}>
                    <Ionicons name={b.kind === 'walk_in' ? 'person-add-outline' : 'lock-closed-outline'} size={18} color={colors.primaryInk} />
                    <T variant="h3" style={{ flex: 1 }} numberOfLines={1}>{b.kind === 'walk_in' ? b.guest_name : t('partner.block_rooms')}</T>
                  </View>
                  <T variant="small" color={colors.inkSoft}>{b.rooms} × {b.room_name} · {shortDate(b.check_in, months)} – {shortDate(b.check_out, months)}</T>
                  {b.guest_phone ? <T variant="small" color={colors.inkSoft}>{b.guest_phone}</T> : null}
                  {b.note ? <T variant="small" color={colors.muted}>{b.note}</T> : null}
                  <Button variant="ghost" title={t('partner.give_back')} onPress={() => release(b)} style={{ marginTop: spacing.xs }} />
                </View>
              ))
            )}
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.lg }}>
              <Button variant="secondary" icon="person-add-outline" title={t('partner.walk_in')} style={{ flex: 1 }} onPress={() => setForm({ kind: 'walk_in', room: chart.data.rooms[0]?.id ?? '', date: todayAddis() })} />
              <Button variant="secondary" icon="lock-closed-outline" title={t('partner.block_rooms')} style={{ flex: 1 }} onPress={() => setForm({ kind: 'block', room: chart.data.rooms[0]?.id ?? '', date: todayAddis() })} />
            </View>
          </>
        )}
      </ScrollView>

      {/* One night: what is taken and by whom, and the two things to do about it. */}
      <Modal visible={!!picked} transparent animationType="slide" onRequestClose={() => setPicked(null)}>
        <Pressable style={{ flex: 1, backgroundColor: 'rgba(0,0,0,0.4)' }} onPress={() => setPicked(null)} />
        {picked ? (
          <View style={{ backgroundColor: colors.surface, borderTopLeftRadius: radius.lg, borderTopRightRadius: radius.lg, padding: spacing.xl, width: '100%', maxWidth: 640, alignSelf: 'center' }}>
            <T variant="h2">{picked.room.name}</T>
            <T color={colors.inkSoft}>{shortDate(picked.night.date, months)}</T>
            <View style={{ marginTop: spacing.md, gap: 4 }}>
              <T variant="h3" color={picked.night.free > 0 ? colors.success : colors.danger}>{t('partner.rooms_free', { n: picked.night.free })} · {t('partner.rooms_of', { n: picked.night.capacity })}</T>
              {picked.night.booked > 0 ? <T color={colors.inkSoft}>{t('partner.rooms_booked', { n: picked.night.booked })}</T> : null}
              {picked.night.held > 0 ? <T color={colors.inkSoft}>{t('partner.rooms_held', { n: picked.night.held })}</T> : null}
              {picked.night.blocked > 0 ? <T color={colors.inkSoft}>{t('partner.rooms_blocked', { n: picked.night.blocked })}</T> : null}
            </View>
            <View style={{ flexDirection: 'row', gap: spacing.sm, marginTop: spacing.lg }}>
              <Button variant="secondary" icon="person-add-outline" title={t('partner.walk_in')} style={{ flex: 1 }} onPress={() => { setForm({ kind: 'walk_in', room: picked.room.id, date: picked.night.date }); setPicked(null); }} />
              <Button variant="secondary" icon="lock-closed-outline" title={t('partner.block_rooms')} style={{ flex: 1 }} onPress={() => { setForm({ kind: 'block', room: picked.room.id, date: picked.night.date }); setPicked(null); }} />
            </View>
          </View>
        ) : null}
      </Modal>

      {form && chart.data ? (
        <BlockForm
          key={`${form.kind}-${form.room}-${form.date}`}
          initial={form}
          rooms={chart.data.rooms}
          propertyId={chart.data.property.id}
          months={months}
          onClose={() => setForm(null)}
          onSaved={() => { setForm(null); refresh(); }}
        />
      ) : null}
    </SafeAreaView>
  );
}

/** Add a walk-in guest or block rooms: which room, from which night, how many nights and rooms. */
function BlockForm({ initial, rooms, propertyId, months, onClose, onSaved }: {
  initial: { kind: NewRoomBlock['kind']; room: string; date: string }; rooms: ChartRoom[]; propertyId: string; months: string[]; onClose: () => void; onSaved: () => void;
}) {
  const { t } = useTranslation();
  const today = todayAddis();
  const [kind, setKind] = useState(initial.kind);
  const [room, setRoom] = useState(initial.room);
  const [date, setDate] = useState(initial.date < today ? today : initial.date);
  const [nights, setNights] = useState(1);
  const [count, setCount] = useState(1);
  const [name, setName] = useState('');
  const [phone, setPhone] = useState('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const total = rooms.find((r) => r.id === room)?.total_rooms ?? 1;
  const out = addDays(date, nights);

  async function save() {
    if (kind === 'walk_in' && name.trim().length < 2) return setError(t('partner.err_name'));
    setBusy(true);
    setError(null);
    try {
      await api.createRoomBlock({ property_id: propertyId, room_type_id: room, check_in: date, check_out: out, rooms: count, kind, guest_name: name.trim() || undefined, guest_phone: phone.trim() || undefined, note: note.trim() || undefined });
      onSaved();
    } catch (e) {
      setError(t(e instanceof ApiError && e.code === 'SOLD_OUT' ? 'partner.err_sold_out' : 'partner.err_generic'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal visible animationType="slide" onRequestClose={onClose}>
      <SafeAreaView style={{ flex: 1, backgroundColor: colors.surface }}>
        <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingHorizontal: spacing.lg }}>
          <T variant="h2">{t(kind === 'walk_in' ? 'partner.walk_in' : 'partner.block_rooms')}</T>
          <Pressable accessibilityLabel={t('common.cancel')} onPress={onClose} style={{ width: 48, height: 48, alignItems: 'center', justifyContent: 'center' }}>
            <Ionicons name="close" size={24} color={colors.ink} />
          </Pressable>
        </View>
        <ScrollView contentContainerStyle={{ padding: spacing.lg, paddingBottom: spacing.xxl }} keyboardShouldPersistTaps="handled">
          <View style={{ flexDirection: 'row', gap: spacing.sm }}>
            <Chip label={t('partner.walk_in')} selected={kind === 'walk_in'} onPress={() => setKind('walk_in')} />
            <Chip label={t('partner.block_rooms')} selected={kind === 'block'} onPress={() => setKind('block')} />
          </View>
          <T variant="label" color={colors.muted} style={{ marginTop: spacing.lg }}>{t('partner.form_room').toUpperCase()}</T>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.xs }}>
            {rooms.map((r) => <Chip key={r.id} label={r.name} selected={r.id === room} onPress={() => { setRoom(r.id); setCount(1); }} />)}
          </View>
          <Counter label={t('partner.form_first_night')} value={shortDate(date, months)} min={date <= today} onChange={(d) => setDate(addDays(date, d))} />
          <Counter label={t('partner.form_nights')} value={`${nights} (${shortDate(date, months)} – ${shortDate(out, months)})`} min={nights <= 1} max={nights >= 60} onChange={(d) => setNights(nights + d)} />
          <Counter label={t('partner.form_rooms')} value={String(count)} min={count <= 1} max={count >= total} onChange={(d) => setCount(count + d)} />
          {kind === 'walk_in' ? (
            <>
              <Field label={t('partner.form_guest')} value={name} onChangeText={setName} autoCapitalize="words" />
              <Field label={t('partner.form_phone')} value={phone} onChangeText={setPhone} keyboardType="phone-pad" />
            </>
          ) : null}
          <Field label={t('partner.form_note')} value={note} onChangeText={setNote} maxLength={300} />
          {error ? <T variant="small" color={colors.danger} style={{ marginTop: spacing.md }}>✕ {error}</T> : null}
          <Button title={t(kind === 'walk_in' ? 'partner.save_walk_in' : 'partner.save_block')} onPress={save} loading={busy} style={{ marginTop: spacing.lg }} />
        </ScrollView>
      </SafeAreaView>
    </Modal>
  );
}
