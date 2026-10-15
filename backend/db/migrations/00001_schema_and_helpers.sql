-- +goose Up

-- Everything lives in the "app" schema, not "public". On Supabase, "public" is exposed through an
-- automatic web API; keeping our tables out of it means that API can never reach them. PostGIS and
-- friends go in "extensions" (Supabase's own convention), which also works on plain PostgreSQL.
CREATE SCHEMA IF NOT EXISTS extensions;
CREATE SCHEMA IF NOT EXISTS app;
CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA extensions;
CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA extensions;

-- +goose StatementBegin
DO $$
DECLARE r text;
BEGIN
  -- Supabase's public API roles must never see anything in "app".
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON SCHEMA app FROM %I', r);
      EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA app REVOKE ALL ON TABLES FROM %I', r);
      EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA app REVOKE ALL ON SEQUENCES FROM %I', r);
      EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA app REVOKE ALL ON FUNCTIONS FROM %I', r);
    END IF;
  END LOOP;
END $$;
-- +goose StatementEnd

-- Keeps updated_at honest.
-- +goose StatementBegin
CREATE FUNCTION app.touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- Used on append-only tables (ledger, history, audit): rows are never changed or removed.
-- Corrections are new rows.
-- +goose StatementBegin
CREATE FUNCTION app.forbid_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION '% on % is not allowed: this table is append-only', TG_OP, TG_TABLE_NAME
    USING ERRCODE = 'restrict_violation';
END $$;
-- +goose StatementEnd

-- +goose Down
-- Development only. Never run a Down migration against production data.
DROP SCHEMA IF EXISTS app CASCADE;
