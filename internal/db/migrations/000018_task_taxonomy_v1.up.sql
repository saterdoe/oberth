UPDATE tasks SET task_type = CASE lower(trim(task_type))
  WHEN 'code' THEN 'implementation'
  WHEN 'dev' THEN 'implementation'
  WHEN 'development' THEN 'implementation'
  WHEN 'implement' THEN 'implementation'
  WHEN 'bug' THEN 'bug_fix'
  WHEN 'bugfix' THEN 'bug_fix'
  WHEN 'fix' THEN 'bug_fix'
  WHEN 'test' THEN 'testing'
  WHEN 'tests' THEN 'testing'
  WHEN 'doc' THEN 'docs'
  WHEN 'documentation' THEN 'docs'
  WHEN 'design' THEN 'architecture'
  WHEN 'implementation' THEN 'implementation'
  WHEN 'bug_fix' THEN 'bug_fix'
  WHEN 'review' THEN 'review'
  WHEN 'testing' THEN 'testing'
  WHEN 'docs' THEN 'docs'
  WHEN 'architecture' THEN 'architecture'
  WHEN 'research' THEN 'research'
  ELSE 'implementation'
END;

ALTER TABLE tasks ADD COLUMN taxonomy_version TEXT NOT NULL DEFAULT '1';
ALTER TABLE tasks ADD CONSTRAINT tasks_task_type_v1_check
  CHECK (task_type IN ('implementation','bug_fix','review','testing','docs','architecture','research'));
