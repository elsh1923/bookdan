import type { BookingStatus } from '@bookdan/domain';
import { useTranslation } from '@bookdan/i18n';
import { Badge } from '@bookdan/ui';

const LOOK: Record<BookingStatus, { tone: 'success' | 'warning' | 'danger' | 'neutral'; icon: string }> = {
  HELD: { tone: 'warning', icon: '⏱' },
  PAID_AWAITING_HOTEL: { tone: 'warning', icon: '⏳' },
  CONFIRMED: { tone: 'success', icon: '✓' },
  CHECKED_IN: { tone: 'success', icon: '✓' },
  COMPLETED: { tone: 'neutral', icon: '✓' },
  EXPIRED: { tone: 'neutral', icon: '✕' },
  DECLINED_REFUNDED: { tone: 'danger', icon: '✕' },
  CANCELLED_REFUNDED: { tone: 'neutral', icon: '✕' },
  NO_SHOW: { tone: 'danger', icon: '✕' },
  NEEDS_RELOCATION: { tone: 'danger', icon: '⚠' },
  DISPUTED: { tone: 'warning', icon: '⚠' },
};

/** Status is always icon + label + colour, never colour alone. */
export function StatusBadge({ status }: { status: BookingStatus }) {
  const { t } = useTranslation();
  const { tone, icon } = LOOK[status];
  return <Badge tone={tone} icon={icon} label={t(`status.${status}`)} />;
}
