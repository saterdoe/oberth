package migration

// Format describes the owner and compatibility contract for persisted data.
type Format struct {
	Name           string
	Owner          string
	CurrentVersion string
	Storage        string
	Rollback       string
}

// Formats is the executable inventory mirrored by docs/PERSISTED_DATA_MIGRATIONS.md.
var Formats = []Format{
	{Name: "configuration", Owner: "internal/config", CurrentVersion: "1", Storage: ".oberth.yaml and environment overrides", Rollback: "restore the pre-migration configuration backup"},
	{Name: "database", Owner: "internal/db", CurrentVersion: "24", Storage: "PostgreSQL schema_migrations", Rollback: "restore the database backup; down migrations are development-only"},
	{Name: "task_state", Owner: "internal/api", CurrentVersion: "1", Storage: "tasks, sessions and task_runs", Rollback: "retain source rows and mark incompatible records for operator recovery"},
	{Name: "run_event", Owner: "internal/api", CurrentVersion: "1", Storage: "run_events.schema_version", Rollback: "append compensating recovery evidence; never rewrite the event log"},
	{Name: "result_bundle", Owner: "internal/api", CurrentVersion: "1", Storage: "task_runs.result_bundle and exported JSON", Rollback: "restore the byte-identical pre-migration backup"},
}

func Descriptor(name string) (Format, bool) {
	for _, format := range Formats {
		if format.Name == name {
			return format, true
		}
	}
	return Format{}, false
}
