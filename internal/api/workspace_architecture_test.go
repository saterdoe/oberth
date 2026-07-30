package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This guard covers the production handlers reachable from the alpha API. File
// and process access belongs in WorkspaceService or a dedicated boundary.
func TestAlphaHandlersDoNotBypassWorkspaceBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, forbidden := range []string{"os.WriteFile(", "os.ReadFile(", "exec.Command(", "exec.CommandContext("} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s contains forbidden direct boundary call %q", name, forbidden)
			}
		}
		if regexp.MustCompile(`filepath\.(Abs|EvalSymlinks)\(`).MatchString(source) && name == "agent_tools.go" {
			t.Errorf("%s resolves caller paths outside WorkspaceService", name)
		}
	}
}
