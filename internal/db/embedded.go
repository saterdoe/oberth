package db

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

type Embedded struct {
	database *embeddedpostgres.EmbeddedPostgres
	DSN      string
}

func StartEmbedded(dataRoot string) (*Embedded, error) {
	return startEmbedded(dataRoot, "")
}

// StartEmbeddedOffline uses a prepared distribution and never downloads it.
// binariesRoot is the distribution root containing bin/pg_ctl, not a database.
func StartEmbeddedOffline(dataRoot, binariesRoot string) (*Embedded, error) {
	if binariesRoot == "" || !filepath.IsAbs(binariesRoot) {
		return nil, fmt.Errorf("offline PostgreSQL distribution must be an absolute path")
	}
	// embedded-postgres checks this exact path even on Windows.
	for _, name := range []string{"pg_ctl", "postgres", "initdb"} {
		binary := name
		if runtime.GOOS == "windows" && name != "pg_ctl" {
			binary += ".exe"
		}
		if !fileExists(filepath.Join(binariesRoot, "bin", binary)) {
			return nil, fmt.Errorf("offline PostgreSQL distribution is missing bin/%s; prepare dependencies first", name)
		}
	}
	return startEmbedded(dataRoot, binariesRoot)
}

func startEmbedded(dataRoot, binariesRoot string) (*Embedded, error) {
	ensureWindowsVCRuntime()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("reserve embedded database port: %w", err)
	}
	port := uint32(listener.Addr().(*net.TCPAddr).Port)
	_ = listener.Close()
	root, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, err
	}
	const user, password, name = "oberth", "oberth-local", "oberth"
	// PostgreSQL 18's current Windows bundle requires a newer native runtime
	// than is present on otherwise supported Windows installations. Pin the
	// embedded distribution to the mature v16 line for portable local startup.
	versionRoot := filepath.Join(root, "v16")
	cfg := embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(port).
		Username(user).
		Password(password).
		Database(name).
		RuntimePath(filepath.Join(versionRoot, "runtime")).
		BinariesPath(filepath.Join(versionRoot, "binaries")).
		DataPath(filepath.Join(versionRoot, "database")).
		StartTimeout(60 * time.Second)
	if binariesRoot != "" {
		cfg = cfg.BinariesPath(binariesRoot)
	}
	database := embeddedpostgres.NewDatabase(cfg)
	if err := database.Start(); err != nil {
		return nil, fmt.Errorf("start embedded database: %w", err)
	}
	return &Embedded{
		database: database,
		DSN:      fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable", user, password, port, name),
	}, nil
}

// PostgreSQL's Windows distribution dynamically links the Microsoft C++
// runtime. Windows installations often already carry the redistributable with
// Edge or a JBR even when it is not registered system-wide. Make that existing
// local runtime discoverable without copying files or installing software.
func ensureWindowsVCRuntime() {
	if runtime.GOOS != "windows" {
		return
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("SystemRoot"), "System32", "vcruntime140.dll")); err == nil {
		return
	}
	var candidates []string
	for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if root == "" {
			continue
		}
		for _, pattern := range []string{
			filepath.Join(root, "Microsoft", "Edge", "Application", "*"),
			filepath.Join(root, "JetBrains", "*", "jbr", "bin"),
		} {
			matches, _ := filepath.Glob(pattern)
			candidates = append(candidates, matches...)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(candidates)))
	for _, candidate := range candidates {
		if fileExists(filepath.Join(candidate, "vcruntime140.dll")) &&
			fileExists(filepath.Join(candidate, "vcruntime140_1.dll")) &&
			fileExists(filepath.Join(candidate, "msvcp140.dll")) {
			_ = os.Setenv("PATH", candidate+string(os.PathListSeparator)+os.Getenv("PATH"))
			return
		}
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(strings.TrimSpace(path))
	return err == nil && !info.IsDir()
}

func (e *Embedded) Stop() error {
	if e == nil || e.database == nil {
		return nil
	}
	return e.database.Stop()
}
