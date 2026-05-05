CREATE TABLE IF NOT EXISTS providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  code TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS provider_accounts (
  id TEXT PRIMARY KEY,
  provider_id TEXT NOT NULL REFERENCES providers(id),
  environment TEXT NOT NULL,
  display_name TEXT NOT NULL,
  credential_json TEXT NOT NULL,
  config_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(provider_id, environment)
);

CREATE TABLE IF NOT EXISTS payment_intents (
  id TEXT PRIMARY KEY,
  external_ref TEXT NOT NULL,
  provider_id TEXT NOT NULL REFERENCES providers(id),
  amount INTEGER NOT NULL,
  currency TEXT NOT NULL,
  status TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS payment_attempts (
  id TEXT PRIMARY KEY,
  payment_intent_id TEXT NOT NULL REFERENCES payment_intents(id),
  provider_id TEXT NOT NULL REFERENCES providers(id),
  request_json TEXT NOT NULL,
  response_json TEXT NOT NULL,
  status TEXT NOT NULL,
  provider_reference TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_events (
  id TEXT PRIMARY KEY,
  provider_id TEXT NOT NULL REFERENCES providers(id),
  provider_event_id TEXT,
  event_type TEXT NOT NULL,
  signature_valid INTEGER NOT NULL,
  payload_json TEXT NOT NULL,
  headers_json TEXT NOT NULL,
  received_at TEXT NOT NULL,
  processed_at TEXT,
  processing_status TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_forwarding_targets (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  provider_id TEXT NOT NULL REFERENCES providers(id),
  event_filter_json TEXT NOT NULL DEFAULT '{}',
  target_url TEXT NOT NULL,
  auth_json TEXT NOT NULL DEFAULT '{}',
  retry_policy_json TEXT NOT NULL DEFAULT '{}',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS webhook_forwarding_attempts (
  id TEXT PRIMARY KEY,
  webhook_event_id TEXT NOT NULL REFERENCES webhook_events(id),
  forwarding_target_id TEXT NOT NULL REFERENCES webhook_forwarding_targets(id),
  request_json TEXT NOT NULL,
  response_json TEXT NOT NULL,
  status TEXT NOT NULL,
  attempt_no INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS refunds (
  id TEXT PRIMARY KEY,
  payment_intent_id TEXT NOT NULL REFERENCES payment_intents(id),
  provider_id TEXT NOT NULL REFERENCES providers(id),
  amount INTEGER NOT NULL,
  status TEXT NOT NULL,
  request_json TEXT NOT NULL,
  response_json TEXT NOT NULL,
  provider_reference TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  actor_type TEXT NOT NULL,
  actor_id TEXT,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_payment_intents_external_ref ON payment_intents(external_ref);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_intents_external_ref_unique ON payment_intents(external_ref);
CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_accounts_provider_environment ON provider_accounts(provider_id, environment);
CREATE INDEX IF NOT EXISTS idx_payment_attempts_payment_intent_id ON payment_attempts(payment_intent_id);
CREATE INDEX IF NOT EXISTS idx_payment_attempts_provider_reference ON payment_attempts(provider_reference);
CREATE INDEX IF NOT EXISTS idx_webhook_events_provider_event_id ON webhook_events(provider_event_id);
CREATE INDEX IF NOT EXISTS idx_webhook_events_event_type ON webhook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_webhook_forwarding_targets_provider_id ON webhook_forwarding_targets(provider_id);
CREATE INDEX IF NOT EXISTS idx_webhook_forwarding_attempts_webhook_event_id ON webhook_forwarding_attempts(webhook_event_id);
CREATE INDEX IF NOT EXISTS idx_refunds_payment_intent_id ON refunds(payment_intent_id);

INSERT OR IGNORE INTO providers (id, name, code, status, created_at, updated_at)
VALUES
  ('provider_midtrans', 'Midtrans', 'midtrans', 'active', datetime('now'), datetime('now')),
  ('provider_xendit', 'Xendit', 'xendit', 'active', datetime('now'), datetime('now'));
