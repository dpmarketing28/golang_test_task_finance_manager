-- Family members. One of them (configured via FAMILY_ADMIN_TELEGRAM_ID)
-- is the head of the family with the 'admin' role and sees everyone's
-- expenses; everyone else is a regular 'member' who only sees their own.
CREATE TYPE user_role AS ENUM ('member', 'admin');

CREATE TABLE users (
    telegram_id BIGINT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    role        user_role NOT NULL DEFAULT 'member',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fixed category list, exactly as given in the task.
CREATE TYPE expense_category AS ENUM (
    'еда', 'транспорт', 'жильё', 'развлечения', 'здоровье', 'покупки', 'другое'
);

CREATE TABLE expenses (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(telegram_id) ON DELETE CASCADE,
    amount     NUMERIC(12, 2) NOT NULL CHECK (amount > 0),
    category   expense_category NOT NULL,
    spent_at   DATE NOT NULL,
    comment    TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Supports the web panel's filters (by member, by period) and the bot's
-- "last expense" lookup.
CREATE INDEX idx_expenses_user_spent_at ON expenses (user_id, spent_at DESC);
CREATE INDEX idx_expenses_category ON expenses (category);
