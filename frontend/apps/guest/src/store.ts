import AsyncStorage from '@react-native-async-storage/async-storage';
import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { Currency } from '@bookdan/domain';
import type { Language } from '@bookdan/i18n';

// Only small UI state lives here. Server data lives in TanStack Query.
interface PrefsState {
  hydrated: boolean;
  onboarded: boolean;
  language: Language | null; // null until the guest has been offered a choice
  currency: Currency;
  recentSearches: string[];
  saved: string[]; // wishlist: property ids
  draftGuestName: string; // typed at checkout; survives a session expiry
  setLanguage: (l: Language) => void;
  setCurrency: (c: Currency) => void;
  finishOnboarding: () => void;
  addRecent: (q: string) => void;
  clearRecent: () => void;
  toggleSaved: (id: string) => void;
  setDraftGuestName: (name: string) => void;
}

export const usePrefs = create<PrefsState>()(
  persist(
    (set) => ({
      hydrated: false,
      onboarded: false,
      language: null,
      currency: 'ETB',
      recentSearches: [],
      saved: [],
      draftGuestName: '',
      setLanguage: (language) => set({ language }),
      setCurrency: (currency) => set({ currency }),
      finishOnboarding: () => set({ onboarded: true }),
      addRecent: (q) =>
        set((s) => {
          const t = q.trim();
          if (!t) return s;
          return { recentSearches: [t, ...s.recentSearches.filter((x) => x !== t)].slice(0, 6) };
        }),
      clearRecent: () => set({ recentSearches: [] }),
      setDraftGuestName: (draftGuestName) => set({ draftGuestName }),
      toggleSaved: (id) => set((st) => ({ saved: st.saved.includes(id) ? st.saved.filter((x) => x !== id) : [id, ...st.saved] })),
    }),
    {
      name: 'guest-prefs',
      storage: createJSONStorage(() => AsyncStorage),
      partialize: (s) => ({
        onboarded: s.onboarded,
        language: s.language,
        currency: s.currency,
        recentSearches: s.recentSearches,
        saved: s.saved,
        draftGuestName: s.draftGuestName,
      }),
      onRehydrateStorage: () => () => usePrefs.setState({ hydrated: true }),
    },
  ),
);
