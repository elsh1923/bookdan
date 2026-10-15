-- +goose Up
SET LOCAL search_path = app, extensions, public;

CREATE TABLE users (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  phone        text UNIQUE CHECK (phone ~ '^\+[1-9][0-9]{7,14}$'),
  email        text UNIQUE CHECK (email = lower(email) AND email ~ '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$'),
  display_name text,
  role         text NOT NULL DEFAULT 'guest' CHECK (role IN ('guest', 'owner', 'receptionist', 'admin')),
  status       text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT users_has_identifier CHECK (phone IS NOT NULL OR email IS NOT NULL)
);
CREATE TRIGGER users_touch BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- One row per code we send. Only a keyed hash of the code is stored, never the code.
CREATE TABLE otp_challenges (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  channel     text NOT NULL CHECK (channel IN ('sms', 'email')),
  identifier  text NOT NULL,  -- E.164 phone or lower-cased email
  code_hash   bytea NOT NULL,
  attempts    integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
  expires_at  timestamptz NOT NULL,
  consumed_at timestamptz,
  ip          inet,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT otp_attempts_bounded CHECK (attempts <= max_attempts)
);
-- Rate limits and "latest code for this identifier" both read by identifier, newest first.
CREATE INDEX otp_challenges_identifier_idx ON otp_challenges (identifier, created_at DESC);

-- A phone running one of our apps. The server can revoke it, and the partner app signs in again.
CREATE TABLE devices (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  device_id    text NOT NULL,  -- the X-Device-Id header
  app          text NOT NULL CHECK (app IN ('guest', 'partner')),
  platform     text NOT NULL CHECK (platform IN ('android', 'ios', 'web')),
  app_version  text,
  push_token   text,           -- FCM token; the alert for a new booking goes here
  revoked_at   timestamptz,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, device_id, app)
);
CREATE INDEX devices_push_idx ON devices (user_id) WHERE push_token IS NOT NULL AND revoked_at IS NULL;

-- Refresh tokens rotate on every use. Presenting an already-rotated token means it was stolen, so the
-- whole family (every token descended from one sign-in) is revoked.
CREATE TABLE refresh_tokens (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  device_pk  uuid NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
  family_id  uuid NOT NULL,
  token_hash bytea NOT NULL UNIQUE,  -- hash only, never the token
  expires_at timestamptz NOT NULL,
  rotated_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);

-- +goose Down
DROP TABLE IF EXISTS app.refresh_tokens, app.devices, app.otp_challenges, app.users CASCADE;
