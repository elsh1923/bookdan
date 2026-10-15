import { Image } from 'expo-image';
import { StyleProp, View, ViewStyle } from 'react-native';
import { T, colors } from '@bookdan/ui';

// Diamond lattice, a nod to Aksumite and Lalibela stonework. Pure Views, so no assets to load.
function Lattice() {
  const cells = Array.from({ length: 60 });
  return (
    <View pointerEvents="none" style={{ position: 'absolute', top: -12, left: -12, right: -12, bottom: -12, flexDirection: 'row', flexWrap: 'wrap', opacity: 0.16 }}>
      {cells.map((_, i) => (
        <View key={i} style={{ width: 40, height: 40, alignItems: 'center', justifyContent: 'center' }}>
          <View style={{ width: 18, height: 18, borderWidth: 1.5, borderColor: '#fff', transform: [{ rotate: '45deg' }] }} />
        </View>
      ))}
    </View>
  );
}

// Low-data loading: tinted placeholder first, then the right-sized image fades in.
// Until the backend serves photos, a toned block with the lattice and the hotel initial stands in.
export function Photo({
  uri,
  accent,
  name,
  style,
  showInitial = true,
}: {
  uri: string | null;
  accent?: string;
  name: string;
  style?: StyleProp<ViewStyle>;
  /** The placeholder letter; cards that overlay text turn it off. */
  showInitial?: boolean;
}) {
  if (!uri) {
    return (
      <View style={[{ backgroundColor: accent ?? colors.charcoal, alignItems: 'center', justifyContent: 'center', overflow: 'hidden' }, style]}>
        <Lattice />
        {showInitial ? <T style={{ fontSize: 44, lineHeight: 56, color: 'rgba(255,255,255,0.9)' }}>{name.charAt(0)}</T> : null}
        {Array.from({ length: 12 }, (_, i) => (
          <View key={i} pointerEvents="none" style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: `${40 - i * 3}%`, backgroundColor: 'rgba(0,0,0,0.025)' }} />
        ))}
      </View>
    );
  }
  return (
    <Image
      source={{ uri }}
      style={style as any}
      placeholder={{ blurhash: 'L6Pj0^jE.AyE_3t7t7R**0o#DgR4' }}
      contentFit="cover"
      transition={250}
      cachePolicy="disk"
    />
  );
}
