import { create } from 'zustand';
import { isDisplayCurrency, type DisplayCurrency, type Rates, type ShownCurrency } from '@bookdan/domain';

/**
 * What the app is showing prices in. The guest picks a currency on the Currency screen; it only takes effect while the server has a fresh rate for it.
 * `version` changes whenever what is shown changes, so the screens can be redrawn (see the root layout).
 */
interface DisplayState {
  /** The currency actually shown: the guest's choice if a fresh rate exists, else ETB. */
  currency: ShownCurrency;
  birrPerUnit: number | null;
  /** The display currencies that have a fresh rate: these are the ones a guest can choose. */
  available: DisplayCurrency[];
  version: number;
  update: (chosen: string, rates: Rates | undefined) => void;
}

export const useDisplay = create<DisplayState>((set, get) => ({
  currency: 'ETB',
  birrPerUnit: null,
  available: [],
  version: 0,
  update: (chosen, rates) => {
    const fresh = (rates?.rates ?? []).filter((r) => !r.stale && isDisplayCurrency(r.currency));
    const pick = chosen === 'ETB' ? undefined : fresh.find((r) => r.currency === chosen);
    const next = {
      currency: (pick ? (pick.currency as DisplayCurrency) : 'ETB') as ShownCurrency,
      birrPerUnit: pick?.birr_per_unit ?? null,
      available: fresh.map((r) => r.currency as DisplayCurrency),
    };
    const cur = get();
    if (cur.currency === next.currency && cur.birrPerUnit === next.birrPerUnit && cur.available.join() === next.available.join()) return;
    set({ ...next, version: cur.version + 1 });
  },
}));
