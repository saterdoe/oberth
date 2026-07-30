ALTER TABLE tasks
  ADD COLUMN risk TEXT NOT NULL DEFAULT 'medium' CHECK (risk IN ('low','medium','high')),
  ADD COLUMN strategy TEXT NOT NULL DEFAULT 'guided' CHECK (strategy IN ('ask','guided','agent','ci'));
