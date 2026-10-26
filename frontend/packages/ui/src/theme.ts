// Design tokens. Navy and slate carry the structure; warm terracotta is the action colour (buttons, selected states).
// Amber is reserved for star ratings and red for "saved".
export const colors = {
  primary: '#E05A47', // terracotta: button fills
  primaryInk: '#B8432F', // terracotta for text and icons on white (AA contrast)
  onPrimary: '#FFFFFF', // text on terracotta fills
  primaryDark: '#9E3424',
  primarySoft: '#FBEAE6', // tinted backgrounds, selected chips
  accent: '#0F6B4F', // deep green, secondary
  accentSoft: '#E3F2EC',
  ink: '#0B192C', // deep navy for headings and text
  charcoal: '#1E293B', // warm slate for secondary headings
  inkSoft: '#475569', // slate-600 for body copy
  muted: '#64748B', // slate-500 for labels
  line: '#E2E8F0', // slate-200 borders
  hairline: '#F1F5F9', // slate-100 faint separators inside cards
  surface: '#FFFFFF',
  background: '#F8FAFC', // off-white page background
  canvas: '#F8FAFC',
  fill: '#F1F5F9', // inputs inside cards
  deal: '#E05A47', // offers and deals share the action colour
  dealInk: '#B8432F',
  dealSoft: '#FBEAE6',
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
  // Noto Sans Ethiopic is bundled by the app so Amharic renders the same on every phone (UI text and labels).
  regular: 'NotoSansEthiopic_400Regular',
  medium: 'NotoSansEthiopic_500Medium',
  bold: 'NotoSansEthiopic_700Bold',
  // Cormorant for hero titles and section headings (Latin only). Amharic keeps the Ethiopic font via DisplayFontContext.
  display: 'CormorantGaramond_700Bold',
  displayMedium: 'CormorantGaramond_600SemiBold',
};

export const shadow = {
  // Ultra-soft and diffused: the card lifts a little, without an outline.
  card: {
    shadowColor: '#0B192C',
    shadowOpacity: 0.06,
    shadowRadius: 20,
    shadowOffset: { width: 0, height: 8 },
    elevation: 2,
  },
  // A soft terracotta glow under primary buttons.
  button: {
    shadowColor: '#E05A47',
    shadowOpacity: 0.25,
    shadowRadius: 12,
    shadowOffset: { width: 0, height: 6 },
    elevation: 4,
  },
};

// Deep navy surfaces used by the home screen and the tab bar.
export const dark = {
  bg: '#0B192C',
  surface: '#14233B',
  surfaceRaised: '#1E293B',
  line: 'rgba(148,163,184,0.25)',
  text: '#FFFFFF',
  textSoft: 'rgba(255,255,255,0.78)',
  muted: 'rgba(255,255,255,0.56)',
  accent: '#F4A090', // soft terracotta for icons and highlights on navy
};

// Terracotta buttons; white text on every stop meets 4.5:1.
export const gradients = {
  primary: ['#E4604C', '#D24F3C', '#BD4331'] as const,
  // Soft navy mesh behind the home search card: deep navy into slate, with a faint warm glow.
  header: ['#0B192C', '#15243D', '#24304A'] as const,
  // Offers strip: a gentle terracotta blend instead of a harsh solid colour.
  deal: ['#EE8C74', '#E05A47'] as const,
};
