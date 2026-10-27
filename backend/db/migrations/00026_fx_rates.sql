-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Display exchange rates. Guests can see prices in USD or EUR, but BookDan always charges and refunds in ETB; these rates only turn
-- an ETB price into an approximate amount on screen (see fxrates.go). An admin sets them. The rate is stored as ETB per one unit of the
-- foreign currency, times 10000 (so 148.5 birr per dollar is 1485000), an integer: never a float in the database.
CREATE TABLE fx_rates (
  currency        text PRIMARY KEY CHECK (currency IN ('USD', 'EUR')),
  birr_per_unit_e4 bigint NOT NULL CHECK (birr_per_unit_e4 BETWEEN 10000 AND 100000000), -- 1 to 10,000 birr per unit
  updated_at      timestamptz NOT NULL DEFAULT now(),
  updated_by      text NOT NULL
);
ALTER TABLE fx_rates ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.fx_rates;
