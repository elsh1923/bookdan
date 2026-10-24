import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { isStillChanging, type Amenity, type PropertyType, type SearchSort } from '@bookdan/domain';
import type { CreateQuoteParams } from '@bookdan/api-client';
import { api } from './api';

export interface SearchCriteria {
  q: string;
  check_in: string;
  check_out: string;
  guests: number;
  rooms?: number;
  types: PropertyType[];
  amenities: Amenity[];
  sort?: SearchSort;
  min_price?: number;
  max_price?: number;
  min_rating?: number;
  /** The guest's position, for "Near me": hotels know how far they are and can be sorted by it. */
  lat?: number;
  lng?: number;
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
    // Changing the number of rooms asks for a new quote; keep showing the old one until it arrives.
    placeholderData: keepPreviousData,
    retry: false, // SOLD_OUT and auth errors are answers, not glitches
  });
}

/** The guest's saved name and ID (the ID masked to its last four characters), or null if they have not saved any. */
export function useProfile(enabled: boolean) {
  return useQuery({ queryKey: ['profile'], queryFn: () => api.getProfile(), enabled, staleTime: 60_000 });
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
