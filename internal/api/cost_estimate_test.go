package api

import "testing"

func TestNemotronLocalModelHasNoAPICost(t *testing.T) {
	input, output := estimateCallCost("nemotron-3-nano-4b", 10_000, 2_000)
	if input != 0 || output != 0 {
		t.Fatalf("local Nemotron cost = (%v, %v), want (0, 0)", input, output)
	}
}
