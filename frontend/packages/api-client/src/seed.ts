import type { PropertyDetail } from '@bookdan/domain';

type Seed = PropertyDetail['property'] & { rooms: PropertyDetail['room_types']; reviews: PropertyDetail['reviews'] };

const rules = ['Check-in from 12:00, check-out until 11:00', 'Valid ID required at check-in', 'No smoking inside rooms'];

function rooms(prefix: string, prices: [string, number, number, number][]): PropertyDetail['room_types'] {
  return prices.map(([name, birr, capacity, left], i) => ({
    id: `${prefix}-r${i + 1}`,
    name,
    capacity,
    price_santim: birr * 100,
    currency: 'ETB' as const,
    amenities: ['Private bathroom', 'TV'],
    rooms_left: left,
  }));
}

function reviews(prefix: string): PropertyDetail['reviews'] {
  return [
    { id: `${prefix}-v1`, author: 'Hanna T.', rating: 5, text: 'Spotless rooms and a very helpful reception.', date: '2026-09-02' },
    { id: `${prefix}-v2`, author: 'Daniel M.', rating: 4, text: 'Good value. Hot water was reliable.', date: '2026-08-14' },
  ];
}

const base = (s: Partial<Seed> & Pick<Seed, 'id' | 'name' | 'area' | 'city' | 'type' | 'lat' | 'lng' | 'accent'> & { rooms: Seed['rooms'] }): Seed => ({
  badge: 'verified',
  from_price_santim: Math.min(...s.rooms.map((r) => r.price_santim)),
  currency: 'ETB',
  available: true,
  amenities: ['wifi', 'hot_water'],
  rating: 4.4,
  review_count: 38,
  photo_url: null,
  description: 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.',
  phone: '+251 11 000 0000',
  address: `${s.area}, ${s.city}`,
  house_rules: rules,
  photos: [],
  reviews: reviews(s.id),
  ...s,
});

export const SEED: Seed[] = [
  base({
    id: 'p-addis-1', name: 'Meskel Square Palace Hotel', area: 'Meskel Square', city: 'Addis Ababa', type: 'hotel',
    lat: 9.0108, lng: 38.7613, accent: '#1E3A8A', amenities: ['generator', 'hot_water', 'wifi', 'parking'],
    rating: 4.6, review_count: 124,
    rooms: rooms('p-addis-1', [['Standard Double', 3200, 2, 5], ['Deluxe King', 4800, 2, 2], ['Family Suite', 7200, 4, 1]]),
  }),
  base({
    id: 'p-addis-2', name: 'Bole Garden Guesthouse', area: 'Bole', city: 'Addis Ababa', type: 'guesthouse',
    lat: 8.9962, lng: 38.7889, accent: '#2563EB', amenities: ['wifi', 'hot_water', 'pay_at_hotel'],
    rating: 4.3, review_count: 61,
    rooms: rooms('p-addis-2', [['Single Room', 1400, 1, 4], ['Twin Room', 2100, 2, 3]]),
  }),
  base({
    id: 'p-addis-3', name: 'Piassa Heritage Inn', area: 'Piassa', city: 'Addis Ababa', type: 'hotel',
    lat: 9.0346, lng: 38.7469, accent: '#334E8C', amenities: ['hot_water', 'wifi'],
    rating: null, review_count: 0, badge: 'none', available: false,
    rooms: rooms('p-addis-3', [['Standard Room', 1800, 2, 0]]),
  }),
  base({
    id: 'p-bahirdar-1', name: 'Lake Tana View Lodge', area: 'Lake Tana shore', city: 'Bahir Dar', type: 'lodge',
    lat: 11.5936, lng: 37.3876, accent: '#0E7490', amenities: ['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel'],
    rating: 4.8, review_count: 207,
    rooms: rooms('p-bahirdar-1', [['Garden Room', 2900, 2, 6], ['Lake View Room', 3900, 2, 3], ['Family Chalet', 6100, 5, 2]]),
  }),
  base({
    id: 'p-lalibela-1', name: 'Rock Church Guesthouse', area: 'Near Bete Giyorgis', city: 'Lalibela', type: 'guesthouse',
    lat: 12.0319, lng: 39.0473, accent: '#3B5BA9', amenities: ['generator', 'wifi', 'pay_at_hotel'],
    rating: 4.5, review_count: 88,
    rooms: rooms('p-lalibela-1', [['Standard Room', 1700, 2, 4], ['Triple Room', 2400, 3, 2]]),
  }),
  base({
    id: 'p-gondar-1', name: 'Fasil Castle View Hotel', area: 'Fasil Ghebbi', city: 'Gondar', type: 'hotel',
    lat: 12.6074, lng: 37.4666, accent: '#4F46A5', amenities: ['generator', 'hot_water', 'wifi', 'parking'],
    rating: 4.2, review_count: 52,
    rooms: rooms('p-gondar-1', [['Standard Double', 2300, 2, 7], ['Executive Room', 3300, 2, 2]]),
  }),
  base({
    id: 'p-hawassa-1', name: 'Hawassa Lakeside Resort', area: 'Lake Hawassa', city: 'Hawassa', type: 'resort',
    lat: 7.0621, lng: 38.4763, accent: '#0F766E', amenities: ['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel'],
    rating: 4.7, review_count: 143,
    rooms: rooms('p-hawassa-1', [['Lake View Room', 4200, 2, 5], ['Bungalow', 6800, 4, 3]]),
  }),
  base({
    id: 'p-adama-1', name: 'Adama Crossroads Hotel', area: 'Adama centre', city: 'Adama', type: 'hotel',
    lat: 8.5414, lng: 39.2689, accent: '#1D4E89', amenities: ['hot_water', 'wifi', 'parking'],
    rating: 4.0, review_count: 29,
    rooms: rooms('p-adama-1', [['Standard Room', 1600, 2, 6]]),
  }),
];
