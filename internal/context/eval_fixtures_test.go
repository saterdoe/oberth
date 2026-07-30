package context

import (
	"encoding/json"
	"os"
	"testing"
)

func TestEvaluationCohortHasThirtyRepresentativeTasks(t *testing.T) {
	data, err := os.ReadFile("testdata/eval_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 30 {
		t.Fatalf("quality gate requires 30 tasks, got %d", len(cases))
	}
	for _, item := range cases {
		if item.Name == "" || item.Query == "" || item.TaskType == "" || len(item.ExpectedSources) == 0 {
			t.Fatalf("incomplete evaluation fixture: %#v", item)
		}
	}
}
