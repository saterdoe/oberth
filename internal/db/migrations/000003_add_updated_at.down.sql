ALTER TABLE sessions DROP COLUMN IF EXISTS updated_at;
ALTER TABLE providers DROP COLUMN IF EXISTS updated_at;
ALTER TABLE routing_rules DROP COLUMN IF EXISTS updated_at;
ALTER TABLE budgets DROP COLUMN IF EXISTS updated_at;
