-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- The signature key stays on the server; here we keep only which key signed the voucher and a hash of its PIN.
CREATE TABLE vouchers (
  booking_id   uuid PRIMARY KEY REFERENCES bookings (id),
  kid          text NOT NULL,           -- id of the signing key, for rotation
  pin_hash     bytea NOT NULL,          -- keyed hash of the 4-digit PIN, never the PIN itself
  pin_attempts integer NOT NULL DEFAULT 0 CHECK (pin_attempts >= 0),
  locked_until timestamptz,             -- set after 5 wrong PINs, enforced here as well as in the app
  issued_at    timestamptz NOT NULL DEFAULT now()
);

-- Actions a hotel did offline, sent when the phone is back online. action_id comes from the device, so
-- sending the same action twice finds the saved result instead of applying it twice.
CREATE TABLE sync_actions (
  action_id    uuid PRIMARY KEY,
  device_pk    uuid NOT NULL REFERENCES devices (id),
  property_id  uuid NOT NULL REFERENCES properties (id),
  seq          bigint NOT NULL,          -- the device's own order; never the phone's clock
  type         text NOT NULL,
  payload      jsonb NOT NULL,
  status       text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'applied', 'rejected')),
  result       jsonb,
  error        text,
  received_at  timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  UNIQUE (device_pk, seq)
);
CREATE INDEX sync_actions_property_idx ON sync_actions (property_id, received_at);

-- What changed for each hotel. The pull cursor is simply the last id the phone has seen.
CREATE TABLE change_log (
  id          bigserial PRIMARY KEY,
  property_id uuid NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
  entity      text NOT NULL CHECK (entity IN ('booking', 'room', 'cancelled_code')),
  entity_id   text NOT NULL,
  op          text NOT NULL CHECK (op IN ('upsert', 'delete')),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX change_log_property_idx ON change_log (property_id, id);

-- Security-relevant events: sign-ins, refresh-token reuse, PIN lockouts, revoked devices.
-- Never put a token, OTP, PIN, phone number or ID number in details.
CREATE TABLE audit_log (
  id         bigserial PRIMARY KEY,
  at         timestamptz NOT NULL DEFAULT now(),
  user_id    uuid REFERENCES users (id),
  device_pk  uuid REFERENCES devices (id),
  action     text NOT NULL,
  ip         inet,
  details    jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX audit_log_user_idx ON audit_log (user_id, at DESC);
CREATE INDEX audit_log_action_idx ON audit_log (action, at DESC);
CREATE TRIGGER audit_log_append_only BEFORE UPDATE OR DELETE ON audit_log FOR EACH ROW EXECUTE FUNCTION forbid_change();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log FOR EACH STATEMENT EXECUTE FUNCTION forbid_change();

-- +goose Down
DROP TABLE IF EXISTS app.audit_log, app.change_log, app.sync_actions, app.vouchers CASCADE;
