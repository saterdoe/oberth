ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_task_type_v1_check;
ALTER TABLE tasks DROP COLUMN IF EXISTS taxonomy_version;
