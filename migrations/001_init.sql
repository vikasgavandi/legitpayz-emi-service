-- Credit Card System : Postgres schema
-- DB per microservice pattern simulated via schemas; for local dev single DB.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ===== Customer Service =====
CREATE TABLE IF NOT EXISTS customers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  full_name TEXT NOT NULL,
  email TEXT UNIQUE NOT NULL,
  phone TEXT UNIQUE NOT NULL,
  pan TEXT UNIQUE NOT NULL, -- ID doc, store encrypted in prod via Secrets Manager/KMS
  dob DATE NOT NULL,
  address JSONB DEFAULT '{}',
  kyc_status TEXT NOT NULL DEFAULT 'PENDING', -- PENDING, VERIFIED, REJECTED
  kyc_docs JSONB DEFAULT '[]', -- S3 URLs
  credit_score INT DEFAULT 650,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- ===== Card Service =====
CREATE TABLE IF NOT EXISTS card_products (
  code TEXT PRIMARY KEY, -- CLASSIC, GOLD, PLATINUM, INFINITE
  name TEXT NOT NULL,
  credit_limit_min BIGINT NOT NULL,
  credit_limit_max BIGINT NOT NULL,
  annual_fee BIGINT NOT NULL,
  interest_rate_apr NUMERIC(5,2) NOT NULL,
  facilities JSONB NOT NULL DEFAULT '[]', -- e.g. ["lounge","fuel_waiver","rewards_5x"]
  rank INT NOT NULL -- for upgrade(>) / downgrade(<) ordering
);

INSERT INTO card_products(code,name,credit_limit_min,credit_limit_max,annual_fee,interest_rate_apr,facilities,rank) VALUES
('CLASSIC','Classic Card',20000,100000,499,42.00,'["rewards_1x","fuel_waiver"]',1),
('GOLD','Gold Card',100000,300000,1999,38.00,'["rewards_3x","lounge_2q","fuel_waiver","emi_0fee"]',2),
('PLATINUM','Platinum Card',300000,800000,4999,34.00,'["rewards_5x","lounge_unlimited_dom","travel_insurance","golf"]',3),
('INFINITE','Infinite Card',800000,2000000,14999,30.00,'["rewards_10x","lounge_global","concierge","zero_forex"]',4)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS credit_cards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id UUID NOT NULL REFERENCES customers(id),
  product_code TEXT NOT NULL REFERENCES card_products(code),
  card_number TEXT UNIQUE NOT NULL, -- store tokenized; last4 indexed
  last4 CHAR(4) NOT NULL,
  cvv_hash TEXT NOT NULL,
  pin_hash TEXT, -- NULL until PIN set
  pin_attempts INT DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'PENDING', -- PENDING, ACTIVE, BLOCKED, CLOSED
  credit_limit BIGINT NOT NULL,
  used_limit BIGINT NOT NULL DEFAULT 0,
  billing_day INT NOT NULL DEFAULT 5,
  expiry DATE NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cards_customer ON credit_cards(customer_id);

CREATE TABLE IF NOT EXISTS card_events (
  id BIGSERIAL PRIMARY KEY,
  card_id UUID NOT NULL,
  event TEXT NOT NULL, -- ISSUED, PIN_SET, UPGRADED, DOWNGRADED, BLOCKED
  meta JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT now()
);

-- ===== Transaction Service =====
CREATE TABLE IF NOT EXISTS transactions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  card_id UUID NOT NULL REFERENCES credit_cards(id),
  txn_type TEXT NOT NULL, -- PURCHASE, PAYMENT, REFUND, FEE, EMI_DEBIT
  amount BIGINT NOT NULL, -- paise
  merchant TEXT,
  status TEXT NOT NULL DEFAULT 'POSTED',
  created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_txn_card ON transactions(card_id);

-- ===== EMI Service =====
CREATE TABLE IF NOT EXISTS emis (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  card_id UUID NOT NULL REFERENCES credit_cards(id),
  transaction_id UUID REFERENCES transactions(id),
  principal BIGINT NOT NULL,
  tenure_months INT NOT NULL,
  interest_rate_apr NUMERIC(5,2) NOT NULL,
  emi_amount BIGINT NOT NULL, -- paise/month
  paid_months INT DEFAULT 0,
  status TEXT DEFAULT 'ACTIVE', -- ACTIVE, CLOSED, FORECLOSED
  schedule JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMPTZ DEFAULT now()
);
