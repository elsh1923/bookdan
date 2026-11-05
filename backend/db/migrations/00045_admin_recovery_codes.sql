-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Recovery codes for an admin who has an authenticator app: eight single-use codes shown once when the app is set up. One of them signs the admin in if they
-- lose their phone, so a super admin does not have to remove the second step for them. Only a keyed hash of each code is kept.
CREATE TABLE admin_recovery_codes (
  user_id    uuid NOT NULL REFERENCES users (id),
  code_hash  bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  used_at    timestamptz,
  PRIMARY KEY (user_id, code_hash)
);
ALTER TABLE admin_recovery_codes ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.admin_recovery_codes;
