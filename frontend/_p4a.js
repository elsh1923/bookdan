const fs = require('fs');
function edit(p, pairs) {
  let s = fs.readFileSync(p, 'utf8');
  for (const [a, b] of pairs) { if (!s.includes(a)) throw new Error(p + ' missing ' + a.slice(0, 80)); s = s.replace(a, () => b); }
  fs.writeFileSync(p, s);
}

// ================= domain
edit('packages/domain/src/schemas.ts', [
  ["'front_desk_24h']);", "'front_desk_24h', 'water_tank']);"],
  ['  landmark: PlaceRefSchema.nullable().default(null),\n});', "  landmark: PlaceRefSchema.nullable().default(null),\n  /** Some room here can be booked with breakfast / a cheaper non-refundable rate is offered. */\n  breakfast: z.boolean().default(false),\n  nonrefundable: z.boolean().default(false),\n  /** How many minutes the hotel usually takes to answer a request (null until it has answered a few). */\n  reply_minutes: z.number().int().nullable().default(null),\n});\n\n/** One place suggestion as the guest types: a landmark, area or city to search for, or a hotel to open. */\nexport const SuggestionSchema = z.object({\n  kind: z.enum(['landmark', 'area', 'city', 'hotel']),\n  text: z.string(),\n  text_am: z.string().optional(),\n  subtitle: z.string().default(''),\n  property_id: z.string().optional(),\n});\nexport type Suggestion = z.infer<typeof SuggestionSchema>;\nexport const SuggestResponseSchema = z.object({ items: z.array(SuggestionSchema), server_time: z.string() });"],
  ["    check_in_time: z.string().default('14:00'),\n    check_out_time: z.string().default('12:00'),\n  }),\n  room_types:", "    check_in_time: z.string().default('14:00'),\n    check_out_time: z.string().default('12:00'),\n    /** The description in Amharic, and how to find the hotel in words (English and Amharic). Empty when the hotel wrote none. */\n    description_am: z.string().default(''),\n    directions: z.string().default(''),\n    directions_am: z.string().default(''),\n  }),\n  room_types:"],
  ["  list_total_santim: z.number().int().default(0),\n  server_time: z.string(),\n});", "  list_total_santim: z.number().int().default(0),\n  /** How many of the guests are children (the rest are adults). */\n  children: z.number().int().default(0),\n  server_time: z.string(),\n});"],
  ["  discount_santim: z.number().int().default(0),\n  server_time: z.string(),\n});\n\n/** The optional sub-ratings", "  discount_santim: z.number().int().default(0),\n  children: z.number().int().default(0),\n  /** What the guest asked the hotel for, and whether the person staying booked it for someone else (with a phone number to reach them). */\n  special_requests: z.string().nullable().default(null),\n  booked_for_other: z.boolean().default(false),\n  guest_phone: z.string().nullable().default(null),\n  server_time: z.string(),\n});\n\n/** The optional sub-ratings"],
]);
edit('packages/domain/src/partner.ts', [
  ["  refundable: z.boolean().default(true),\n  server_time", "  refundable: z.boolean().default(true),\n  /** Who is travelling and what they asked for. For a booking made for someone else, guest_name is the person staying and guest_phone reaches them. */\n  children: z.number().int().default(0),\n  special_requests: z.string().default(''),\n  booked_for_other: z.boolean().default(false),\n  guest_phone: z.string().default(''),\n  server_time"],
]);

// ================= api client
edit('packages/api-client/src/types.ts', [
  ["  min_rating?: number;\n  /** The guest", "  min_rating?: number;\n  /** Only hotels that offer breakfast / a cheaper non-refundable rate. */\n  breakfast?: boolean;\n  deals?: boolean;\n  /** The guest"],
  ["  coupon_code?: string;\n}", "  coupon_code?: string;\n  /** How many of the guests are children (each room still needs an adult). */\n  children?: number;\n}"],
  ["  createHold(params: { quote_id: string; guest_name: string; idempotency_key: string }): Promise<Booking>;", "  createHold(params: {\n    quote_id: string;\n    /** The name of the person who will stay. */\n    guest_name: string;\n    idempotency_key: string;\n    /** Up to 400 characters for the hotel (\"late arrival\", \"high floor\"). */\n    special_requests?: string;\n    /** Booking for someone else: the hotel is told, and gets the guest's phone number to reach them. */\n    booked_for_other?: boolean;\n    guest_phone?: string;\n  }): Promise<Booking>;"],
  ["  getProperty(params: GetPropertyParams): Promise<PropertyDetail>;\n", "  getProperty(params: GetPropertyParams): Promise<PropertyDetail>;\n  /** Landmarks, areas, cities and hotels matching what the guest has typed (with nothing typed: popular landmarks). */\n  suggest(q: string): Promise<Suggestion[]>;\n"],
  ["  ReviewInput,\n  SearchSort,", "  ReviewInput,\n  SearchSort,\n  Suggestion,"],
]);
edit('packages/api-client/src/http.ts', [
  ["  CheckinLookupSchema,\n", "  CheckinLookupSchema,\n  SuggestResponseSchema,\n"],
  ["  if (p.min_rating) q.set('min_rating', String(p.min_rating));\n", "  if (p.min_rating) q.set('min_rating', String(p.min_rating));\n  if (p.breakfast) q.set('breakfast', '1');\n  if (p.deals) q.set('deals', '1');\n"],
  ["    createHold: (p) =>\n      request('POST', '/bookings', BookingSchema, { quote_id: p.quote_id, guest_name: p.guest_name }, { 'Idempotency-Key': p.idempotency_key }),",
   "    suggest: async (q) => (await request('GET', `/suggest?${new URLSearchParams({ q })}`, SuggestResponseSchema)).items,\n    createHold: (p) =>\n      request(\n        'POST', '/bookings', BookingSchema,\n        { quote_id: p.quote_id, guest_name: p.guest_name, special_requests: p.special_requests, booked_for_other: p.booked_for_other, guest_phone: p.guest_phone },\n        { 'Idempotency-Key': p.idempotency_key },\n      ),"],
]);
