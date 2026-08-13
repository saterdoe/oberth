CREATE TABLE IF NOT EXISTS cost_reservations (
    reservation_id UUID NOT NULL,
    budget_id UUID NOT NULL REFERENCES budgets(id) ON DELETE CASCADE,
    amount DOUBLE PRECISION NOT NULL CHECK (amount > 0),
    state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active','committed','released','expired')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (reservation_id, budget_id)
);

CREATE INDEX IF NOT EXISTS idx_cost_reservations_active
    ON cost_reservations (budget_id, expires_at) WHERE state = 'active';
