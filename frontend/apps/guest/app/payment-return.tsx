import { Redirect } from 'expo-router';
import { useEffect, useState } from 'react';
import { ActivityIndicator, View } from 'react-native';
import { colors } from '@bookdan/ui';
import { loadPendingPayment } from '../src/payment';

/**
 * Deep link target (hotelapp://payment-return) the payment page sends the guest back to.
 * It carries no result: it only reopens the booking, and the booking screen learns the outcome from the server.
 */
export default function PaymentReturn() {
  const [target, setTarget] = useState<string | null | undefined>(undefined);

  useEffect(() => {
    loadPendingPayment().then(setTarget);
  }, []);

  if (target === undefined) {
    return <View style={{ flex: 1, justifyContent: 'center', backgroundColor: colors.background }}><ActivityIndicator color={colors.primary} /></View>;
  }
  return target ? <Redirect href={{ pathname: '/booking/[id]', params: { id: target } }} /> : <Redirect href="/" />;
}
