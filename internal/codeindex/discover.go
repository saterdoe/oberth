package codeindex

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saterdoe/oberth/pkg/secrets"
)

type Options struct {
	MaxFileBytes                                                   int64
	MaxFiles, MaxChunks, MaxChunkLines, OverlapLines, PerFileLimit int
	Exclude                                                        []string
}

func DefaultOptions() Options {
	return Options{MaxFileBytes: 512 * 1024, MaxFiles: 5000, MaxChunks: 20000, MaxChunkLines: 240, OverlapLines: 20, PerFileLimit: 4}
}

type File struct {
	Path, AbsPath, Language string
	Content                 []byte
	Hash                    string
	IsTest                  bool
}

var excludedDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "target": true, "coverage": true, ".cache": true, "__pycache__": true, ".next": true, "bin": true, "obj": true}
var secretNames = map[string]bool{".env": true, ".env.local": true, "credentials": true, "credentials.json": true, "id_rsa": true, "id_ed25519": true}
var binaryExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".pdf": true, ".zip": true, ".exe": true, ".dll": true, ".so": true, ".db": true, ".sqlite": true, ".woff": true, ".mp4": true, ".mp3": true, ".pem": true, ".key": true, ".p12": true}

func Discover(root string, o Options) ([]File, error) {
	if o.MaxFileBytes <= 0 {
		o = DefaultOptions()
	}
	abs, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	real, e := filepath.EvalSymlinks(abs)
	if e != nil {
		return nil, e
	}
	st, e := os.Stat(real)
	if e != nil || !st.IsDir() {
		return nil, errors.New("repository root is not a directory")
	}
	o.Exclude = append(o.Exclude, readGitignore(real)...)
	var out []File
	e = filepath.WalkDir(real, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == real {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if d.IsDir() {
			if excludedDirs[name] || matchesExclude(name, o.Exclude) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(out) >= o.MaxFiles {
			return nil
		}
		rel, e := filepath.Rel(real, path)
		if e != nil || strings.HasPrefix(rel, "..") {
			return nil
		}
		rel = filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(name))
		if secretNames[name] || strings.HasPrefix(name, ".env.") || binaryExt[ext] || strings.HasSuffix(name, ".min.js") || strings.HasSuffix(name, ".min.css") || strings.HasSuffix(name, ".lock") || matchesExclude(rel, o.Exclude) {
			return nil
		}
		info, e := d.Info()
		if e != nil || info.Size() > o.MaxFileBytes {
			return nil
		}
		data, e := os.ReadFile(path)
		if e != nil || isBinary(data) || looksGenerated(data) || secrets.HasSecrets(string(data)) {
			return nil
		}
		out = append(out, File{Path: rel, AbsPath: path, Language: Language(rel), Content: data, Hash: hash(string(data)), IsTest: isTest(rel)})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, e
}
func matchesExclude(path string, x []string) bool {
	p := strings.ToLower(filepath.ToSlash(path))
	for _, v := range x {
		v = strings.Trim(strings.ToLower(filepath.ToSlash(v)), "/")
		matched, _ := filepath.Match(v, p)
		baseMatched, _ := filepath.Match(v, filepath.Base(p))
		if v != "" && (p == v || strings.HasPrefix(p, v+"/") || matched || baseMatched) {
			return true
		}
	}
	return false
}
func readGitignore(root string) []string {
	b, e := os.ReadFile(filepath.Join(root, ".gitignore"))
	if e != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		out = append(out, strings.Trim(line, "/"))
	}
	return out
}
func isBinary(b []byte) bool { b = b[:min(len(b), 8192)]; return bytes.IndexByte(b, 0) >= 0 }
func looksGenerated(b []byte) bool {
	s := strings.ToLower(string(b[:min(len(b), 2048)]))
	return strings.Contains(s, "code generated") && strings.Contains(s, "do not edit")
}
func isTest(p string) bool {
	s := strings.ToLower(p)
	return strings.Contains(s, "_test.") || strings.Contains(s, ".test.") || strings.Contains(s, ".spec.") || strings.Contains(s, "/test/") || strings.Contains(s, "/tests/")
}
func Language(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".go":
		return "go"
	case ".java":
		return "java"
	case ".cs":
		return "csharp"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".py":
		return "python"
	case ".md", ".mdx":
		return "markdown"
	case ".json", ".yaml", ".yml", ".toml":
		return "configuration"
	default:
		return "unknown"
	}
}
