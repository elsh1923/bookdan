-- +goose Up
SET LOCAL search_path = app, extensions, public;

CREATE TABLE payments (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  booking_id    uuid NOT NULL REFERENCES bookings (id),
  provider      text NOT NULL CHECK (provider IN ('mock', 'telebirr', 'chapa')),
  provider_ref  text NOT NULL,  -- the gateway's own id for this payment
  amount_santim bigint NOT NULL CHECK (amount_santim > 0),
  currency      text NOT NULL CHECK (currency IN ('ETB', 'USD', 'EUR')),
  status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed', 'refunded')),
  payment_url   text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (provider, provider_ref)
);
CREATE TRIGGER payments_touch BEFORE UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE INDEX payments_booking_idx ON payments (booking_id);
-- Retrying "pay" while a payment is open returns the same payment, so a guest is never charged twice.
CREATE UNIQUE INDEX payments_one_pending_per_booking ON payments (booking_id) WHERE status = 'pending';
-- At most one successful payment per booking.
CREATE UNIQUE INDEX payments_one_success_per_booking ON payments (booking_id) WHERE status = 'succeeded';

-- Every callback from a gateway. The unique key is what makes a duplicate callback harmless (acceptance
-- test M4): the second insert fails and the handler stops.
CREATE TABLE payment_events (
  id           bigserial PRIMARY KEY,
  provider     text NOT NULL,
  event_id     text NOT NULL,
  payload      jsonb NOT NULL,
  received_at  timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  UNIQUE (provider, event_id)
);
-- Only processed_at may change, once.
-- +goose StatementBegin
CREATE FUNCTION payment_events_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'payment events cannot be deleted' USING ERRCODE = 'restrict_violation';
  END IF;
  IF (NEW.id, NEW.provider, NEW.event_id, NEW.payload, NEW.received_at)
     IS DISTINCT FROM (OLD.id, OLD.provider, OLD.event_id, OLD.payload, OLD.received_at) OR OLD.processed_at IS NOT NULL THEN
    RAISE EXCEPTION 'payment events are append-only' USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER payment_events_guard BEFORE UPDATE OR DELETE ON payment_events FOR EACH ROW EXECUTE FUNCTION payment_events_guard();

-- ---- Double-entry ledger. Every movement of money is a transaction whose entries add up to zero.
CREATE TABLE ledger_accounts (
  code        text PRIMARY KEY,
  kind        text NOT NULL CHECK (kind IN ('asset', 'liability', 'revenue', 'expense')),
  description text NOT NULL
);

INSERT INTO ledger_accounts (code, kind, description) VALUES
  ('gateway_clearing',          'asset',     'Money the payment gateways hold for us, not yet paid out to our bank'),
  ('guest_deposits_held',       'liability', 'Guest money received and not yet released to the hotel or refunded'),
  ('property_payable',          'liability', 'What we owe hotels for confirmed stays'),
  ('commission_revenue',        'revenue',   'Our commission on confirmed bookings'),
  ('vat_on_commission_payable', 'liability', 'VAT charged on our commission, owed to the tax authority'),
  ('refunds_payable',           'liability', 'Refunds owed to guests, not yet paid back through the gateway');

CREATE TABLE ledger_transactions (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  booking_id uuid REFERENCES bookings (id),
  payment_id uuid REFERENCES payments (id),
  kind       text NOT NULL CHECK (kind IN ('deposit_received', 'booking_confirmed', 'refund', 'payout', 'adjustment')),
  memo       text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ledger_transactions_booking_idx ON ledger_transactions (booking_id);

-- Amounts are signed: positive = debit, negative = credit. A transaction's entries must add up to zero.
CREATE TABLE ledger_entries (
  id             bigserial PRIMARY KEY,
  transaction_id uuid NOT NULL REFERENCES ledger_transactions (id),
  account_code   text NOT NULL REFERENCES ledger_accounts (code),
  amount_santim  bigint NOT NULL CHECK (amount_santim <> 0),
  currency       text NOT NULL CHECK (currency IN ('ETB', 'USD', 'EUR')),
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ledger_entries_transaction_idx ON ledger_entries (transaction_id);
CREATE INDEX ledger_entries_account_idx ON ledger_entries (account_code, created_at);

-- Checked when the database transaction commits, so a transaction can be built entry by entry.
-- +goose StatementBegin
CREATE FUNCTION ledger_check_balanced() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE bad record;
BEGIN
  SELECT currency, sum(amount_santim) AS total INTO bad
  FROM ledger_entries WHERE transaction_id = NEW.transaction_id
  GROUP BY currency HAVING sum(amount_santim) <> 0 LIMIT 1;
  IF FOUND THEN
    RAISE EXCEPTION 'ledger transaction % does not balance (% % off)', NEW.transaction_id, bad.total, bad.currency
      USING ERRCODE = 'check_violation';
  END IF;
  RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER ledger_entries_balanced AFTER INSERT ON ledger_entries
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION ledger_check_balanced();

-- A transaction needs at least two entries (one debit, one credit).
-- +goose StatementBegin
CREATE FUNCTION ledger_check_has_entries() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (SELECT count(*) FROM ledger_entries WHERE transaction_id = NEW.id) < 2 THEN
    RAISE EXCEPTION 'ledger transaction % needs at least two entries', NEW.id USING ERRCODE = 'check_violation';
  END IF;
  RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER ledger_transactions_have_entries AFTER INSERT ON ledger_transactions
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION ledger_check_has_entries();

-- Nothing in the ledger is ever edited or deleted. A mistake is fixed with a new, opposite transaction.
CREATE TRIGGER ledger_entries_append_only BEFORE UPDATE OR DELETE ON ledger_entries FOR EACH ROW EXECUTE FUNCTION forbid_change();
CREATE TRIGGER ledger_transactions_append_only BEFORE UPDATE OR DELETE ON ledger_transactions FOR EACH ROW EXECUTE FUNCTION forbid_change();
CREATE TRIGGER ledger_entries_no_truncate BEFORE TRUNCATE ON ledger_entries FOR EACH STATEMENT EXECUTE FUNCTION forbid_change();
CREATE TRIGGER ledger_transactions_no_truncate BEFORE TRUNCATE ON ledger_transactions FOR EACH STATEMENT EXECUTE FUNCTION forbid_change();

-- Balance of each account, per currency. Statements and reconciliation read this.
CREATE VIEW ledger_balances AS
  SELECT account_code, currency, sum(amount_santim)::bigint AS balance_santim
  FROM ledger_entries GROUP BY account_code, currency;

-- +goose Down
DROP VIEW IF EXISTS app.ledger_balances;
DROP TABLE IF EXISTS app.ledger_entries, app.ledger_transactions, app.ledger_accounts,
  app.payment_events, app.payments CASCADE;
DROP FUNCTION IF EXISTS app.ledger_check_has_entries(), app.ledger_check_balanced(), app.payment_events_guard();
