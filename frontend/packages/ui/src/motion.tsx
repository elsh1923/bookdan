import React, { useEffect } from 'react';
import { Pressable, PressableProps, StyleProp, View, ViewStyle } from 'react-native';
import Animated, {
  Easing,
  FadeIn,
  FadeInDown,
  FadeInRight,
  LinearTransition,
  ReduceMotion,
  ZoomIn,
  cancelAnimation,
  useAnimatedStyle,
  useSharedValue,
  withDelay,
  withRepeat,
  withSequence,
  withSpring,
  withTiming,
} from 'react-native-reanimated';
import { colors, radius } from './theme';

/**
 * Motion kit. Rules: motion confirms what the person did or shows where something came from; it is short
 * (under half a second), it never blocks a tap, and it is skipped when the phone's "reduce motion" setting is on.
 */

const ease = Easing.out(Easing.cubic);
export const AnimatedPressable = Animated.createAnimatedComponent(Pressable);
export { Animated, FadeIn, FadeInRight, LinearTransition, ReduceMotion };

/** A pressable that shrinks slightly under the finger and springs back. */
export function PressableScale({
  scaleTo = 0.97,
  style,
  onPressIn,
  onPressOut,
  children,
  ...rest
}: Omit<PressableProps, 'style'> & { scaleTo?: number; style?: StyleProp<ViewStyle> }) {
  const scale = useSharedValue(1);
  const animated = useAnimatedStyle(() => ({ transform: [{ scale: scale.value }] }));
  return (
    <AnimatedPressable
      {...rest}
      onPressIn={(e) => {
        scale.value = withSpring(scaleTo, { damping: 18, stiffness: 320, reduceMotion: ReduceMotion.System });
        onPressIn?.(e);
      }}
      onPressOut={(e) => {
        scale.value = withSpring(1, { damping: 12, stiffness: 260, reduceMotion: ReduceMotion.System });
        onPressOut?.(e);
      }}
      style={[style, animated]}
    >
      {children}
    </AnimatedPressable>
  );
}

/** Fades and rises into place. Give list items their index so they arrive one after another. */
export function Reveal({ index = 0, children, style }: { index?: number; children: React.ReactNode; style?: StyleProp<ViewStyle> }) {
  return (
    <Animated.View
      entering={FadeInDown.delay(Math.min(index, 8) * 70).duration(420).easing(ease).reduceMotion(ReduceMotion.System)}
      style={style}
    >
      {children}
    </Animated.View>
  );
}

/** Content that changes with a step: remount it with a new `stepKey` and it slides in from the right. */
export function StepTransition({ stepKey, children, style }: { stepKey: string | number; children: React.ReactNode; style?: StyleProp<ViewStyle> }) {
  return (
    <Animated.View key={stepKey} entering={FadeInRight.duration(280).easing(ease).reduceMotion(ReduceMotion.System)} style={style}>
      {children}
    </Animated.View>
  );
}

/** Springs in with a little overshoot: for a success tick. */
export function Pop({ children, style, delay = 0 }: { children: React.ReactNode; style?: StyleProp<ViewStyle>; delay?: number }) {
  return (
    <Animated.View entering={ZoomIn.delay(delay).springify().damping(11).reduceMotion(ReduceMotion.System)} style={style}>
      {children}
    </Animated.View>
  );
}

/** Returns a style and a function: call the function (for example when a heart is saved) and the element bounces. */
export function useBounce() {
  const scale = useSharedValue(1);
  const style = useAnimatedStyle(() => ({ transform: [{ scale: scale.value }] }));
  const bounce = () => {
    scale.value = withSequence(
      withTiming(0.8, { duration: 90 }),
      withSpring(1.25, { damping: 6, stiffness: 400 }),
      withSpring(1, { damping: 10, stiffness: 300 }),
    );
  };
  return { style, bounce };
}

/** A grey block that breathes while real content loads. Better than a spinner because the page keeps its shape. */
export function Skeleton({ width, height, radius: r = radius.md, style, dark }: {
  width?: number | `${number}%`; height: number; radius?: number; style?: StyleProp<ViewStyle>; dark?: boolean;
}) {
  const o = useSharedValue(0.55);
  useEffect(() => {
    o.value = withRepeat(withSequence(withTiming(1, { duration: 750 }), withTiming(0.55, { duration: 750 })), -1, false);
    return () => cancelAnimation(o);
  }, [o]);
  const animated = useAnimatedStyle(() => ({ opacity: o.value }));
  return (
    <Animated.View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={[{ width: width ?? '100%', height, borderRadius: r, backgroundColor: dark ? 'rgba(255,255,255,0.10)' : colors.fill }, style, animated]}
    />
  );
}

/** A bar that grows to `progress` (0 to 1) instead of jumping. */
export function ProgressFill({ progress, height = 2, color = colors.primary, track = colors.line }: {
  progress: number; height?: number; color?: string; track?: string;
}) {
  const w = useSharedValue(progress);
  useEffect(() => {
    w.value = withDelay(60, withTiming(progress, { duration: 320, easing: ease, reduceMotion: ReduceMotion.System }));
  }, [progress, w]);
  const animated = useAnimatedStyle(() => ({ width: `${w.value * 100}%` }));
  return (
    <View style={{ height, borderRadius: height, backgroundColor: track, overflow: 'hidden' }}>
      <Animated.View style={[{ height, backgroundColor: color }, animated]} />
    </View>
  );
}

/** A View that gently pulses while `active`: used to draw the eye to something urgent, like a running-out countdown. */
export function Pulse({ active, children, style }: { active: boolean; children: React.ReactNode; style?: StyleProp<ViewStyle> }) {
  const k = useSharedValue(0);
  useEffect(() => {
    if (active) {
      k.value = withRepeat(withSequence(withTiming(1, { duration: 700 }), withTiming(0, { duration: 700 })), -1, false);
    } else {
      cancelAnimation(k);
      k.value = withTiming(0, { duration: 200 });
    }
    return () => cancelAnimation(k);
  }, [active, k]);
  const animated = useAnimatedStyle(() => ({ transform: [{ scale: 1 + k.value * 0.05 }], opacity: 1 - k.value * 0.12 }));
  return <Animated.View style={[style, animated]}>{children}</Animated.View>;
}
