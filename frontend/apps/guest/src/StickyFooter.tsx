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
        position: 'absolute', left: 0, right: 0, bottom: 0,
        paddingHorizontal: spacing.lg, paddingTop: spacing.md, paddingBottom: Math.max(insets.bottom, spacing.md),
        backgroundColor: colors.surface, borderTopWidth: 1, borderTopColor: colors.line,
        shadowColor: '#000', shadowOpacity: 0.08, shadowRadius: 16, shadowOffset: { width: 0, height: -4 }, elevation: 8,
      }}
    >
      {children}
    </View>
  );
}

/** Space to leave at the end of a ScrollView so the last content clears the footer. */
export const FOOTER_CLEARANCE = 112;
