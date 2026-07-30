package repoanalyzer

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Options struct{ MaxFiles int }
type SearchOptions struct{ Limit int }
type Metadata struct {
	PrimaryLanguage string   `json:"primary_language"`
	PackageManager  string   `json:"package_manager"`
	Frameworks      []string `json:"frameworks"`
	Entrypoints     []string `json:"entrypoints"`
	Manifests       []string `json:"manifests"`
}
type Result struct {
	Metadata  Metadata `json:"metadata"`
	Tree      []string `json:"tree"`
	Map       string   `json:"repo_map"`
	Truncated bool     `json:"truncated"`
}
type Match struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}
type Symbol struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Line int    `json:"line"`
}

var ignored = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, ".idea": true, ".vscode": true}
var symbolPatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"function", regexp.MustCompile(`^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`)},
	{"function", regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)`)},
	{"type", regexp.MustCompile(`^\s*(?:type|class|interface|struct)\s+([A-Za-z_]\w*)`)},
}

func Analyze(root string, opts Options) (Result, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return Result{}, err
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("repository root is not a directory")
	}
	limit := opts.MaxFiles
	if limit <= 0 {
		limit = 2000
	}
	result := Result{}
	err = walk(root, func(path, rel string, entry fs.DirEntry) error {
		if entry.IsDir() {
			return nil
		}
		if len(result.Tree) >= limit {
			result.Truncated = true
			return nil
		}
		result.Tree = append(result.Tree, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	result.Metadata = detectMetadata(root, result.Tree)
	result.Map = buildMap(result.Tree)
	return result, nil
}

func Search(root, query string, opts SearchOptions) ([]Match, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Match{}, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	var out []Match
	err := walk(root, func(path, rel string, entry fs.DirEntry) error {
		if entry.IsDir() || len(out) >= limit {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || isBinary(data) {
			return nil
		}
		s := bufio.NewScanner(strings.NewReader(string(data)))
		line := 0
		for s.Scan() {
			line++
			if strings.Contains(strings.ToLower(s.Text()), strings.ToLower(query)) {
				out = append(out, Match{Path: filepath.ToSlash(rel), Line: line, Text: strings.TrimSpace(s.Text())})
				if len(out) >= limit {
					break
				}
			}
		}
		return nil
	})
	return out, err
}

func SearchSymbols(root, query string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []Symbol
	err := walk(root, func(path, rel string, entry fs.DirEntry) error {
		if entry.IsDir() || len(out) >= limit {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".go" && ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".py" && ext != ".rs" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		s := bufio.NewScanner(strings.NewReader(string(data)))
		line := 0
		for s.Scan() {
			line++
			for _, pattern := range symbolPatterns {
				match := pattern.re.FindStringSubmatch(s.Text())
				if len(match) > 1 && strings.Contains(strings.ToLower(match[1]), strings.ToLower(query)) {
					out = append(out, Symbol{Path: filepath.ToSlash(rel), Line: line, Name: match[1], Kind: pattern.kind})
					break
				}
			}
			if len(out) >= limit {
				break
			}
		}
		return nil
	})
	return out, err
}

func walk(root string, fn func(path, rel string, entry fs.DirEntry) error) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && ignored[entry.Name()] {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return fn(path, rel, entry)
	})
}

func detectMetadata(root string, files []string) Metadata {
	m := Metadata{}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		return err == nil
	}
	checks := []struct{ file, lang, manager string }{{"go.mod", "Go", "go"}, {"package.json", "TypeScript/JavaScript", "npm"}, {"pyproject.toml", "Python", "python"}, {"Cargo.toml", "Rust", "cargo"}, {"pom.xml", "Java", "maven"}, {"build.gradle", "Java", "gradle"}}
	for _, check := range checks {
		if exists(check.file) {
			m.Manifests = append(m.Manifests, check.file)
			if m.PrimaryLanguage == "" {
				m.PrimaryLanguage = check.lang
				m.PackageManager = check.manager
			}
		}
	}
	if exists("next.config.js") || exists("next.config.ts") {
		m.Frameworks = append(m.Frameworks, "Next.js")
	}
	if exists("vite.config.ts") || exists("vite.config.js") {
		m.Frameworks = append(m.Frameworks, "Vite")
	}
	for _, file := range files {
		base := filepath.Base(file)
		if base == "main.go" || base == "main.py" || base == "index.ts" || base == "index.js" {
			m.Entrypoints = append(m.Entrypoints, file)
		}
	}
	sort.Strings(m.Entrypoints)
	return m
}

func buildMap(files []string) string {
	type stats struct{ files, tests int }
	groups := map[string]*stats{}
	for _, file := range files {
		dir := filepath.ToSlash(filepath.Dir(file))
		if dir == "." {
			dir = "root"
		}
		s := groups[dir]
		if s == nil {
			s = &stats{}
			groups[dir] = s
		}
		s.files++
		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			s.tests++
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "- %s (files: %d, tests: %d)\n", key, groups[key].files, groups[key].tests)
	}
	return b.String()
}

func isBinary(data []byte) bool {
	limit := len(data)
	if limit > 8000 {
		limit = 8000
	}
	for _, b := range data[:limit] {
		if b == 0 {
			return true
		}
	}
	return false
}
