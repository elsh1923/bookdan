-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- More display currencies than USD and EUR (the list lives in fxrates.go), including some worth less than one birr per unit (the yen), so the lower bound
-- of a rate goes from 1 birr to 0.01 birr per unit. Charging and refunding are still ETB only.
ALTER TABLE fx_rates DROP CONSTRAINT fx_rates_currency_check;
ALTER TABLE fx_rates ADD CONSTRAINT fx_rates_currency_check CHECK (currency ~ '^[A-Z]{3}$' AND currency <> 'ETB');
ALTER TABLE fx_rates DROP CONSTRAINT fx_rates_birr_per_unit_e4_check;
ALTER TABLE fx_rates ADD CONSTRAINT fx_rates_birr_per_unit_e4_check CHECK (birr_per_unit_e4 BETWEEN 100 AND 100000000);

-- +goose Down
DELETE FROM app.fx_rates WHERE currency NOT IN ('USD', 'EUR') OR birr_per_unit_e4 < 10000;
ALTER TABLE app.fx_rates DROP CONSTRAINT fx_rates_birr_per_unit_e4_check;
ALTER TABLE app.fx_rates ADD CONSTRAINT fx_rates_birr_per_unit_e4_check CHECK (birr_per_unit_e4 BETWEEN 10000 AND 100000000);
ALTER TABLE app.fx_rates DROP CONSTRAINT fx_rates_currency_check;
ALTER TABLE app.fx_rates ADD CONSTRAINT fx_rates_currency_check CHECK (currency IN ('USD', 'EUR'));
