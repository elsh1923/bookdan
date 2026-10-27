import { create } from 'zustand';
import type { Currency, DisplayCurrency, Rates } from '@bookdan/domain';

/**
 * What the app is showing prices in. The guest picks USD or EUR in their profile; it only takes effect while the server has a fresh rate
 * for it. `version` changes whenever what is shown changes, so the screens can be redrawn (see the root layout).
 */
interface DisplayState {
  /** The currency actually shown: the guest's choice if a fresh rate exists, else ETB. */
  currency: Currency;
  birrPerUnit: number | null;
  /** The display currencies that have a fresh rate: these are the ones a guest can choose. */
  available: DisplayCurrency[];
  version: number;
  update: (chosen: Currency, rates: Rates | undefined) => void;
}

export const useDisplay = create<DisplayState>((set, get) => ({
  currency: 'ETB',
  birrPerUnit: null,
  available: [],
  version: 0,
  update: (chosen, rates) => {
    const fresh = (rates?.rates ?? []).filter((r) => !r.stale);
    const pick = chosen === 'ETB' ? undefined : fresh.find((r) => r.currency === chosen);
    const next = { currency: (pick ? pick.currency : 'ETB') as Currency, birrPerUnit: pick?.birr_per_unit ?? null, available: fresh.map((r) => r.currency) };
    const cur = get();
    if (cur.currency === next.currency && cur.birrPerUnit === next.birrPerUnit && cur.available.join() === next.available.join()) return;
    set({ ...next, version: cur.version + 1 });
  },
}));
