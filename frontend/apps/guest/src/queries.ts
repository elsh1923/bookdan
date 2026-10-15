import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { isStillChanging, type Amenity, type PropertyType } from '@bookdan/domain';
import type { CreateQuoteParams } from '@bookdan/api-client';
import { api } from './api';

export interface SearchCriteria {
  q: string;
  check_in: string;
  check_out: string;
  guests: number;
  types: PropertyType[];
  amenities: Amenity[];
}

export function useSearch(c: SearchCriteria) {
  return useInfiniteQuery({
    queryKey: ['search', c],
    queryFn: ({ pageParam }) => api.search({ ...c, cursor: pageParam }),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.next_cursor,
  });
}

export function useProperty(id: string, check_in: string, check_out: string) {
  return useQuery({
    queryKey: ['property', id, check_in, check_out],
    queryFn: () => api.getProperty({ id, check_in, check_out }),
    staleTime: 60 * 60_000, // property details: 1 hour
  });
}

export function useQuote(p: CreateQuoteParams, enabled: boolean) {
  return useQuery({
    queryKey: ['quote', p],
    queryFn: () => api.createQuote(p),
    enabled,
    staleTime: 0,
    gcTime: 0,
    retry: false, // SOLD_OUT and auth errors are answers, not glitches
  });
}

/** Polls while the booking can still change; stops on a final status. */
export function useBooking(id: string) {
  return useQuery({
    queryKey: ['booking', id],
    queryFn: () => api.getBooking(id),
    staleTime: 0,
    refetchInterval: (q) => (q.state.data && !isStillChanging(q.state.data) ? false : 2000),
  });
}

export function useBookings(enabled: boolean) {
  return useQuery({ queryKey: ['bookings'], queryFn: () => api.listBookings(), enabled, staleTime: 0 });
}

/** Every property for the given stay, fetched page by page. Used by the home carousels and wishlists. */
export function useAllProperties(c: Omit<SearchCriteria, 'q' | 'amenities'>) {
  const query = useSearch({ q: '', amenities: [], ...c });
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = query;
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);
  const items = query.data?.pages.flatMap((pg) => pg.items) ?? [];
  return { items, isPending: query.isPending, isError: query.isError, refetch: query.refetch };
}
