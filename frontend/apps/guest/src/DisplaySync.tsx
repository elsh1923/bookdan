import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { api } from './api';
import { useDisplay } from './display';
import { usePrefs } from './store';

/**
 * Keeps what the app shows prices in up to date: reads the display exchange rates from the server (public, refreshed hourly) and applies
 * the guest's choice when there is a fresh rate for it. If the rates cannot be read the app simply shows ETB.
 */
export function DisplaySync() {
  const chosen = usePrefs((s) => s.currency);
  const rates = useQuery({ queryKey: ['rates'], queryFn: () => api.getRates(), staleTime: 60 * 60_000, retry: 1 });
  useEffect(() => {
    useDisplay.getState().update(chosen, rates.data);
  }, [chosen, rates.data]);
  return null;
}
