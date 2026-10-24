import { Pressable, ScrollView, View } from 'react-native';
import { addDays, nightsBetween } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { T, colors, radius, spacing } from '@bookdan/ui';
import { ethiopianLine } from './format';

const pad = (n: number) => String(n).padStart(2, '0');

interface Cell { iso: string; day: number }

/** The weeks of one month, Monday first, with empty slots (null) before the 1st. */
function weeksOf(year: number, month: number): (Cell | null)[][] {
  const first = (new Date(Date.UTC(year, month - 1, 1)).getUTCDay() + 6) % 7; // 0 = Monday
  const days = new Date(Date.UTC(year, month, 0)).getUTCDate();
  const cells: (Cell | null)[] = Array(first).fill(null);
  for (let d = 1; d <= days; d++) cells.push({ iso: `${year}-${pad(month)}-${pad(d)}`, day: d });
  while (cells.length % 7) cells.push(null);
  const weeks: (Cell | null)[][] = [];
  for (let i = 0; i < cells.length; i += 7) weeks.push(cells.slice(i, i + 7));
  return weeks;
}

/**
 * A calendar to pick a stay: tap the check-in day, then the check-out day. A tap on a day before (or on) the check-in starts again.
 * Days that have passed cannot be chosen, and a stay is at most maxNights. In Amharic each day also shows its Ethiopian date.
 */
export function DateRangePicker({ start, end, today, maxNights = 30, onChange, onTooLong }: {
  start: string | null; end: string | null; today: string; maxNights?: number;
  onChange: (start: string, end: string | null) => void; onTooLong?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const lang = i18n.language === 'am' ? 'am' : 'en';
  const months = t('months.short', { returnObjects: true }) as unknown as string[];
  const weekdays = t('weekdays.short', { returnObjects: true }) as unknown as string[];
  const [ty, tm] = today.split('-').map(Number);

  const tap = (iso: string) => {
    if (iso < today) return;
    if (!start || end || iso <= start) return onChange(iso, null);
    if (nightsBetween(start, iso) > maxNights) {
      onTooLong?.();
      return onChange(start, addDays(start, maxNights));
    }
    onChange(start, iso);
  };

  const list = Array.from({ length: 13 }, (_, i) => {
    const m0 = tm - 1 + i;
    return { year: ty + Math.floor(m0 / 12), month: (m0 % 12) + 1 };
  });

  return (
    <ScrollView contentContainerStyle={{ paddingBottom: spacing.xl }}>
      <View style={{ flexDirection: 'row', paddingHorizontal: spacing.lg, paddingTop: spacing.sm }}>
        {weekdays.map((w) => <T key={w} variant="label" color={colors.muted} style={{ flex: 1, textAlign: 'center' }}>{w}</T>)}
      </View>
      {list.map(({ year, month }) => (
        <View key={`${year}-${month}`} style={{ marginTop: spacing.lg }}>
          <T variant="h3" style={{ paddingHorizontal: spacing.lg }}>{months[month - 1]} {year}</T>
          {weeksOf(year, month).map((week, wi) => (
            <View key={wi} style={{ flexDirection: 'row', paddingHorizontal: spacing.lg, marginTop: 4 }}>
              {week.map((c, ci) => {
                if (!c) return <View key={ci} style={{ flex: 1, height: 48 }} />;
                const past = c.iso < today;
                const isStart = c.iso === start;
                const isEnd = c.iso === end;
                const inside = !!start && !!end && c.iso > start && c.iso < end;
                const eth = ethiopianLine(c.iso, lang);
                return (
                  <Pressable
                    key={ci}
                    disabled={past}
                    onPress={() => tap(c.iso)}
                    accessibilityRole="button"
                    accessibilityLabel={`${c.day} ${months[month - 1]} ${year}`}
                    accessibilityState={{ selected: isStart || isEnd, disabled: past }}
                    style={{ flex: 1, height: 48, alignItems: 'center', justifyContent: 'center', backgroundColor: inside ? colors.primarySoft : 'transparent', borderRadius: isStart || isEnd ? radius.md : 0, ...(isStart || isEnd ? { backgroundColor: colors.primary } : null) }}
                  >
                    <T variant="body" color={isStart || isEnd ? colors.onPrimary : past ? colors.line : colors.ink} style={{ fontWeight: c.iso === today ? '700' : '400' }}>{c.day}</T>
                    {eth ? <T variant="label" color={isStart || isEnd ? colors.onPrimary : colors.muted} style={{ fontSize: 9, lineHeight: 11 }} numberOfLines={1}>{eth.split(' ').slice(0, 2).join(' ').replace(',', '')}</T> : null}
                  </Pressable>
                );
              })}
            </View>
          ))}
        </View>
      ))}
    </ScrollView>
  );
}
