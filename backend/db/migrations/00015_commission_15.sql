-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- BookDan's commission on every booking paid online: 15% of what the guest paid (VAT on the commission stays at 15%).
UPDATE pricing_config SET commission_bps = 1500 WHERE id = 1;

-- +goose Down
UPDATE app.pricing_config SET commission_bps = 0 WHERE id = 1;
