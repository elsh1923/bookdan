import { Image } from 'expo-image';
import { LinearGradient } from 'expo-linear-gradient';
import { useEffect, useRef } from 'react';
import { Animated, Easing, View } from 'react-native';
import { T, colors, dark, gradients, spacing } from '@bookdan/ui';

/**
 * Shown while the app loads its fonts and saved data: the BookDan logo on the deep navy background, with a gentle pulse
 * and a thin terracotta progress line. It replaces the plain spinner so the first screen feels like the brand.
 */
export function LaunchScreen() {
  const pulse = useRef(new Animated.Value(0)).current;
  const bar = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    const breathe = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, { toValue: 1, duration: 900, easing: Easing.inOut(Easing.ease), useNativeDriver: true }),
        Animated.timing(pulse, { toValue: 0, duration: 900, easing: Easing.inOut(Easing.ease), useNativeDriver: true }),
      ]),
    );
    const sweep = Animated.loop(
      Animated.timing(bar, { toValue: 1, duration: 1400, easing: Easing.linear, useNativeDriver: true }),
    );
    breathe.start();
    sweep.start();
    return () => {
      breathe.stop();
      sweep.stop();
    };
  }, [pulse, bar]);

  const scale = pulse.interpolate({ inputRange: [0, 1], outputRange: [1, 1.05] });
  const barX = bar.interpolate({ inputRange: [0, 1], outputRange: [-160, 160] });

  return (
    <LinearGradient colors={gradients.header} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={{ flex: 1, alignItems: 'center', justifyContent: 'center' }}>
      <Animated.View style={{ transform: [{ scale }], alignItems: 'center' }}>
        <Image
          source={require('../assets/icon.png')}
          accessibilityLabel="BookDan"
          style={{ width: 120, height: 120, borderRadius: 28 }}
        />
        <T variant="displaySm" color={dark.text} style={{ marginTop: spacing.md, letterSpacing: 0.5 }}>Bookdan</T>
      </Animated.View>
      <View style={{ position: 'absolute', bottom: 96, width: 160, height: 3, borderRadius: 2, overflow: 'hidden', backgroundColor: 'rgba(255,255,255,0.14)' }}>
        <Animated.View style={{ position: 'absolute', top: 0, bottom: 0, width: 160, backgroundColor: colors.primary, transform: [{ translateX: barX }] }} />
      </View>
    </LinearGradient>
  );
}
