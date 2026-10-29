import AsyncStorage from '@react-native-async-storage/async-storage';
import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { Language } from '@bookdan/i18n';

// Only small UI state lives here. Server data lives in TanStack Query.
interface PrefsState {
  hydrated: boolean;
  language: Language | null; // null until the staff member picks one; the phone language is proposed
  setLanguage: (l: Language) => void;
  /** Whether the signed-in staff member is a hotel owner (money screens are for owners): null until the server has said. Forgotten at sign-out. */
  owner: boolean | null;
  setOwner: (owner: boolean | null) => void;
}

export const usePrefs = create<PrefsState>()(
  persist(
    (set) => ({
      hydrated: false,
      language: null,
      setLanguage: (language) => set({ language }),
      owner: null,
      setOwner: (owner) => set({ owner }),
    }),
    {
      name: 'partner-prefs',
      storage: createJSONStorage(() => AsyncStorage),
      partialize: (s) => ({ language: s.language, owner: s.owner }),
      onRehydrateStorage: () => () => usePrefs.setState({ hydrated: true }),
    },
  ),
);
