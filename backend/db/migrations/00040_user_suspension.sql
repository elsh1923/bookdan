-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Why, when and by whom BookDan staff suspended an account. (A deleted account is also 'suspended', with none of these set.)
ALTER TABLE users
  ADD COLUMN suspended_at     timestamptz,
  ADD COLUMN suspended_reason text,
  ADD COLUMN suspended_by     uuid REFERENCES users (id);
CREATE INDEX users_suspended_idx ON users (suspended_at DESC) WHERE suspended_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS app.users_suspended_idx;
ALTER TABLE app.users DROP COLUMN IF EXISTS suspended_by, DROP COLUMN IF EXISTS suspended_reason, DROP COLUMN IF EXISTS suspended_at;
