-- Legacy editor/chat tables are intentionally not recreated. Roll back the
-- application binary instead; durable task_runs and run_events are unaffected.
SELECT 1;
