import { normalizeEmail, type Identity } from '@bookdan/domain';
import { ApiError, type AuthApi, type Tokens } from './types';

/** Any phone or email accepts this code in the mocks. */
export const MOCK_OTP = '123456';

const ACCESS_TTL_MS = 15 * 60_000;

/**
 * Sign-in, token expiry and refresh for the mock servers (guest and partner share it).
 * Access tokens expire; a refresh token trades for a new pair until it is revoked.
 */
export function createMockAuth(opts: { accessTtlMs?: number; gate: () => Promise<void> }) {
  const accessTtl = opts.accessTtlMs ?? ACCESS_TTL_MS;
  let token: string | null = null;
  let accessInvalidBefore = 0;
  let refreshInvalidBefore = 0;

  const issue = (value: string): Tokens => {
    const now = Date.now();
    return { token: `tok_${now}_${value}`, refresh_token: `ref_${now}_${value}` };
  };

  const api: AuthApi = {
    setToken(t) {
      token = t;
    },
    async requestOtp(identity: Identity) {
      await opts.gate();
      if (identity.type === 'phone' && !/^\+251[79]\d{8}$/.test(identity.value)) throw new ApiError('INVALID_PHONE', 'Invalid phone number');
      if (identity.type === 'email' && !normalizeEmail(identity.value)) throw new ApiError('INVALID_EMAIL', 'Invalid email address');
    },
    async verifyOtp(identity, code) {
      await opts.gate();
      if (code !== MOCK_OTP) throw new ApiError('INVALID_OTP', 'Wrong code');
      // A phone and an email can never collide, so each identity owns its own data in the mock.
      return { ...issue(identity.value), identity };
    },
    async logout() {
      // nothing to end on the mock server
    },
    async refresh(refreshToken) {
      await opts.gate();
      const m = refreshToken.match(/^ref_(\d+)_(.+)$/);
      if (!m || Number(m[1]) < refreshInvalidBefore) throw new ApiError('UNAUTHENTICATED', 'Please sign in again');
      return issue(m[2]);
    },
  };

  return {
    api,
    /** The signed-in identity's value, or an UNAUTHENTICATED error when missing or expired. */
    identityOf(): string {
      const m = token?.match(/^tok_(\d+)_(.+)$/);
      if (!m) throw new ApiError('UNAUTHENTICATED', 'Sign in required');
      const iat = Number(m[1]);
      if (Date.now() - iat >= accessTtl || iat < accessInvalidBefore) throw new ApiError('UNAUTHENTICATED', 'Session expired');
      return m[2];
    },
    expireAccessToken() {
      accessInvalidBefore = Date.now() + 1;
    },
    revokeRefreshToken() {
      accessInvalidBefore = Date.now() + 1;
      refreshInvalidBefore = Date.now() + 1;
    },
  };
}
