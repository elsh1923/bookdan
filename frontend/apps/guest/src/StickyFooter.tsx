import type { ReactNode } from 'react';
import { View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { colors, spacing } from '@bookdan/ui';

/** White action bar pinned to the bottom of a screen, above the gesture bar / home indicator. */
export function StickyFooter({ children }: { children: ReactNode }) {
  const insets = useSafeAreaInsets();
  return (
    <View
      style={{
        // Floats above the page: rounded, lifted by a soft shadow, with a small gap from the screen edges.
        position: 'absolute', left: spacing.md, right: spacing.md, bottom: Math.max(insets.bottom, spacing.sm),
        paddingHorizontal: spacing.lg, paddingTop: spacing.md, paddingBottom: spacing.md,
        borderRadius: 22, backgroundColor: colors.surface, borderWidth: 1, borderColor: colors.hairline,
        shadowColor: '#0B192C', shadowOpacity: 0.12, shadowRadius: 18, shadowOffset: { width: 0, height: 8 }, elevation: 10,
      }}
    >
      {children}
    </View>
  );
}

/** Space to leave at the end of a ScrollView so the last content clears the footer. */
export const FOOTER_CLEARANCE = 112;
