-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- When more than one server runs, only one of them should run the background sweeper at a time, and each daily job should run once a day in total.
-- A lease says which server is the sweeper for the next minute (it renews it every pass; if it stops, another server takes over). It is a row, not a
-- database lock, so it also works through a connection pooler.
CREATE TABLE job_leases (
  name   text PRIMARY KEY,
  holder text NOT NULL,
  until  timestamptz NOT NULL
);
ALTER TABLE job_leases ENABLE ROW LEVEL SECURITY;

-- The daily jobs: the first server to claim a (job, day) runs it.
CREATE TABLE job_runs (
  name   text NOT NULL,
  day    date NOT NULL,
  ran_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (name, day)
);
ALTER TABLE job_runs ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.job_runs;
DROP TABLE IF EXISTS app.job_leases;
