-- +goose Up

-- Row level security on every table, with no policies: any role except the table owner sees nothing.
-- The Go backend connects as the owner (the Supabase "postgres" user), so it is unaffected. This is a second
-- lock behind the schema being outside Supabase's web API: even if a table were exposed by mistake, the
-- public keys could not read it.
-- +goose StatementBegin
DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT tablename FROM pg_tables WHERE schemaname = 'app' LOOP
    EXECUTE format('ALTER TABLE app.%I ENABLE ROW LEVEL SECURITY', t.tablename);
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE t record;
BEGIN
  FOR t IN SELECT tablename FROM pg_tables WHERE schemaname = 'app' LOOP
    EXECUTE format('ALTER TABLE app.%I DISABLE ROW LEVEL SECURITY', t.tablename);
  END LOOP;
END $$;
-- +goose StatementEnd
