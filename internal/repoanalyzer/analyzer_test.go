package repoanalyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                       "module example.com/demo\n\ngo 1.24\n",
		"README.md":                    "# Demo\n",
		"cmd/app/main.go":              "package main\nfunc main() { RunServer() }\n",
		"internal/server/http.go":      "package server\nfunc RunServer() {}\n",
		"internal/server/http_test.go": "package server\nfunc TestServer() {}\n",
		"node_modules/skip.js":         "secret needle\n",
		".git/config":                  "needle\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestAnalyzeDetectsStackAndEntrypoints(t *testing.T) {
	result, err := Analyze(fixture(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metadata.PrimaryLanguage != "Go" {
		t.Fatalf("language = %q", result.Metadata.PrimaryLanguage)
	}
	if result.Metadata.PackageManager != "go" {
		t.Fatalf("package manager = %q", result.Metadata.PackageManager)
	}
	if !contains(result.Metadata.Entrypoints, "cmd/app/main.go") {
		t.Fatalf("entrypoints = %#v", result.Metadata.Entrypoints)
	}
}

func TestTreeFiltersIgnoredDirectoriesAndHonorsLimit(t *testing.T) {
	result, err := Analyze(fixture(t), Options{MaxFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(result.Tree, "\n")
	if strings.Contains(joined, "node_modules") || strings.Contains(joined, ".git") {
		t.Fatalf("ignored path leaked: %s", joined)
	}
	if len(result.Tree) > 3 || !result.Truncated {
		t.Fatalf("tree=%d truncated=%v", len(result.Tree), result.Truncated)
	}
}

func TestSearchFindsTextAndSymbolsWithoutIgnoredFiles(t *testing.T) {
	root := fixture(t)
	text, err := Search(root, "RunServer", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(text) != 2 {
		t.Fatalf("results = %#v", text)
	}
	symbols, err := SearchSymbols(root, "RunServer", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 1 || symbols[0].Kind != "function" {
		t.Fatalf("symbols = %#v", symbols)
	}
}

func TestRepoMapSummarizesPackagesAndTests(t *testing.T) {
	result, err := Analyze(fixture(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Map, "cmd/app") || !strings.Contains(result.Map, "internal/server") {
		t.Fatalf("map = %s", result.Map)
	}
	if !strings.Contains(result.Map, "tests: 1") {
		t.Fatalf("map = %s", result.Map)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
