-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- BookDan staff accounts for the admin page: each person has their own password and a role (super, support or finance), and can add a second step (an
-- authenticator app code). This replaces the one password shared through ADMIN_PASSWORD, which stays only to let the first person in.
CREATE TABLE admin_accounts (
  user_id              uuid PRIMARY KEY REFERENCES users (id),
  role                 text NOT NULL CHECK (role IN ('super', 'support', 'finance')),
  password_hash        text NOT NULL,
  must_change_password boolean NOT NULL DEFAULT true,   -- a password someone else chose or generated must be replaced at the first sign-in
  password_changed_at  timestamptz,
  totp_secret_enc      bytea,                           -- the authenticator secret, encrypted
  totp_pending_enc     bytea,                           -- a secret being set up, not yet confirmed with a code
  totp_enabled_at      timestamptz,
  totp_last_step       bigint NOT NULL DEFAULT 0,       -- the last 30-second step accepted, so one code cannot be used twice
  failed_attempts      integer NOT NULL DEFAULT 0,
  locked_until         timestamptz,
  disabled_at          timestamptz,
  disabled_by          uuid REFERENCES users (id),
  created_by           uuid REFERENCES users (id),
  created_at           timestamptz NOT NULL DEFAULT now(),
  last_login_at        timestamptz,
  CONSTRAINT admin_totp_together CHECK ((totp_secret_enc IS NULL) = (totp_enabled_at IS NULL))
);
CREATE INDEX admin_accounts_role_idx ON admin_accounts (role) WHERE disabled_at IS NULL;
ALTER TABLE admin_accounts ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.admin_accounts;
