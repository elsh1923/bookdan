import { Ionicons } from '@expo/vector-icons';
import { LinearGradient } from 'expo-linear-gradient';
import { useFocusEffect } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useCallback, useState, type ReactNode } from 'react';
import { Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Reveal } from './motion';
import { T } from './components';
import { dark, gradients, radius, spacing } from './theme';

/**
 * The top of every screen in both apps: a deep navy band with rounded lower corners, the section in small capitals above a large title, and room for
 * more underneath (a row of numbers, a search box). A screen opened from another screen uses the compact form, which has a round back button and a
 * one-line title. Use this instead of drawing a back arrow and a title by hand, so every page starts the same way.
 */
export function ScreenHeader({ eyebrow, title, onBack, backLabel = 'Back', right, children, compact }: {
  eyebrow?: string;
  title: string;
  onBack?: () => void;
  /** Spoken name of the back button (pass the translated word). */
  backLabel?: string;
  right?: ReactNode;
  children?: ReactNode;
  compact?: boolean;
}) {
  const insets = useSafeAreaInsets();
  // The status bar text is light only while this screen is the one in front.
  const [focused, setFocused] = useState(true);
  useFocusEffect(useCallback(() => {
    setFocused(true);
    return () => setFocused(false);
  }, []));

  return (
    <LinearGradient
      colors={gradients.header}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={{ paddingTop: insets.top + spacing.sm, paddingBottom: compact ? spacing.md : spacing.lg, paddingHorizontal: spacing.lg, borderBottomLeftRadius: 28, borderBottomRightRadius: 28 }}
    >
      {focused ? <StatusBar style="light" /> : null}
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: spacing.sm, minHeight: 44 }}>
        {onBack ? (
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={backLabel}
            onPress={onBack}
            hitSlop={6}
            style={{ width: 40, height: 40, borderRadius: radius.pill, backgroundColor: 'rgba(255,255,255,0.14)', alignItems: 'center', justifyContent: 'center' }}
          >
            <Ionicons name="arrow-back" size={22} color="#FFFFFF" />
          </Pressable>
        ) : null}
        <View style={{ flex: 1 }}>
          {eyebrow ? <T variant="label" color={dark.accent} numberOfLines={1}>{eyebrow.toUpperCase()}</T> : null}
          {compact ? <T variant="h2" color="#FFFFFF" numberOfLines={1}>{title}</T> : null}
        </View>
        {right}
      </View>
      {compact ? null : (
        <Reveal>
          <T variant="display" color="#FFFFFF" style={{ marginTop: spacing.xs }}>{title}</T>
        </Reveal>
      )}
      {children}
    </LinearGradient>
  );
}
