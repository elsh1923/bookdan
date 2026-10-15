import i18next from 'i18next';
import { initReactI18next } from 'react-i18next';
import am from './am';
import en from './en';

// Demo ships English + Amharic (D). Afaan Oromoo and Tigrinya are added for launch (M).
export const SUPPORTED_LANGUAGES = ['en', 'am'] as const;
export type Language = (typeof SUPPORTED_LANGUAGES)[number];

export const LANGUAGE_NAMES: Record<Language, string> = { en: 'English', am: 'አማርኛ' };

export function pickLanguage(deviceTag: string | null | undefined): Language {
  const code = (deviceTag ?? 'en').slice(0, 2).toLowerCase();
  return (SUPPORTED_LANGUAGES as readonly string[]).includes(code) ? (code as Language) : 'en';
}

export function initI18n(lng: Language) {
  if (!i18next.isInitialized) {
    i18next.use(initReactI18next).init({
      resources: { en: { translation: en }, am: { translation: am } },
      lng,
      fallbackLng: 'en',
      interpolation: { escapeValue: false },
      returnNull: false,
    });
  } else {
    i18next.changeLanguage(lng);
  }
  return i18next;
}

export { i18next };
export { useTranslation } from 'react-i18next';
