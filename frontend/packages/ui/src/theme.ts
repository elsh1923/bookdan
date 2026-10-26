// Design tokens. Blue is the brand colour; amber is reserved for star ratings and red for "saved".
export const colors = {
  primary: '#2563EB', // brand blue (fills)
  primaryInk: '#1D4ED8', // blue for text and icons on white (AA contrast)
  onPrimary: '#FFFFFF', // text on blue fills
  primaryDark: '#1E40AF',
  primarySoft: '#EAF1FF', // tinted backgrounds, selected chips
  accent: '#0F6B4F', // deep green, secondary
  accentSoft: '#E3F2EC',
  ink: '#0F1B33', // near-black navy for text
  charcoal: '#0F1B33',
  inkSoft: '#4B5B76',
  muted: '#7B8AA3',
  line: '#E6EBF4',
  hairline: '#EDF1F8', // faint separators inside cards
  surface: '#FFFFFF',
  background: '#FFFFFF',
  canvas: '#F5F7FC', // soft page background that lifts white cards
  fill: '#F3F6FB', // cool grey-blue for inputs inside cards
  deal: '#F97316', // saffron for offers and deals
  dealInk: '#C2410C',
  dealSoft: '#FFF3E8',
  gold: '#F5A524', // premium / featured accents
  star: '#F5A524', // ratings
  favorite: '#F43F5E', // saved hearts
  danger: '#C0392B',
  dangerSoft: '#FBE9E7',
  warning: '#B26A00',
  warningSoft: '#FFF1D6',
  success: '#1E7F4F',
  successSoft: '#E1F4EA',
};

// 8pt grid
export const spacing = { xs: 4, sm: 8, md: 16, lg: 24, xl: 32, xxl: 48 };

export const radius = { sm: 8, md: 16, lg: 24, pill: 999 };

// Touch targets stay large for quick use at a busy front desk.
export const MIN_TOUCH = 48;

export const fonts = {
  // Noto Sans Ethiopic is bundled by the app so Amharic renders the same on every phone.
  regular: 'NotoSansEthiopic_400Regular',
  medium: 'NotoSansEthiopic_500Medium',
  bold: 'NotoSansEthiopic_700Bold',
  // Serif for headings (Latin only). Amharic keeps the Ethiopic font via DisplayFontContext.
  display: 'PlayfairDisplay_700Bold',
  displayMedium: 'PlayfairDisplay_500Medium',
};

export const shadow = {
  card: {
    shadowColor: '#0F1B33',
    shadowOpacity: 0.08,
    shadowRadius: 16,
    shadowOffset: { width: 0, height: 6 },
    elevation: 3,
  },
  // A soft blue glow under primary buttons.
  button: {
    shadowColor: '#2563EB',
    shadowOpacity: 0.28,
    shadowRadius: 12,
    shadowOffset: { width: 0, height: 6 },
    elevation: 4,
  },
};

// Deep navy surfaces used by the home screen and the tab bar.
export const dark = {
  bg: '#0A1428',
  surface: '#10203F',
  surfaceRaised: '#17294D',
  line: 'rgba(125,177,255,0.28)',
  text: '#FFFFFF',
  textSoft: 'rgba(255,255,255,0.76)',
  muted: 'rgba(255,255,255,0.54)',
  accent: '#7DB1FF', // light blue for icons and highlights on navy
};

// Brand blue, for primary buttons. White text on every stop meets 4.5:1.
export const gradients = {
  primary: ['#2F6FEB', '#2563EB', '#1D4ED8'] as const,
  // Deep navy-to-blue band behind the home search card (the MakeMyTrip-style hero).
  header: ['#0A1B4E', '#15316F', '#1E4AB8'] as const,
  // Saffron, for the offers / deals strip.
  deal: ['#FFA14A', '#F97316'] as const,
};
