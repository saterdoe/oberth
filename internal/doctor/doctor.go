package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	secretspkg "github.com/saterdoe/oberth/pkg/secrets"
)

const (
	StatusPass = "PASS"
	StatusFail = "FAIL"
	StatusWarn = "WARN"
)

type Check struct {
	Name    string
	Status  string
	Message string
}

func ConfigCheck(path string) Check {
	info, err := os.Stat(path)
	if err != nil {
		return Check{"Config file", StatusFail, err.Error()}
	}
	if info.IsDir() {
		return Check{"Config file", StatusFail, fmt.Sprintf("%q is a directory", path)}
	}
	return Check{"Config file", StatusPass, fmt.Sprintf("found at %s", path)}
}

// RuntimeConfigCheck mirrors oberth-server startup semantics: configuration is
// optional and defaults are valid, but a path that exists must be a regular
// file. This prevents `doctor` from rejecting the zero-config start flow.
func RuntimeConfigCheck(path string) Check {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return Check{"Config file", StatusPass, "not present; using built-in defaults"}
	}
	if err != nil {
		return Check{"Config file", StatusFail, err.Error()}
	}
	if info.IsDir() {
		return Check{"Config file", StatusFail, fmt.Sprintf("%q is a directory", path)}
	}
	return Check{"Config file", StatusPass, fmt.Sprintf("found at %s", path)}
}

func VaultStructureCheck(path string) Check {
	info, err := os.Stat(path)
	if err != nil {
		return Check{"Vault directory", StatusFail, err.Error()}
	}
	if !info.IsDir() {
		return Check{"Vault directory", StatusFail, fmt.Sprintf("%q is not a directory", path)}
	}

	required := []string{"architecture", "decisions", "patterns", "bugs", "sessions", "tasks"}
	var missing []string
	for _, dir := range required {
		d := filepath.Join(path, dir)
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			missing = append(missing, dir)
		}
	}

	if len(missing) > 0 {
		return Check{"Vault structure", StatusWarn, fmt.Sprintf("missing directories: %s", strings.Join(missing, ", "))}
	}

	return Check{"Vault structure", StatusPass, "all directories present"}
}

func SecretsCheck(vaultPath string) Check {
	info, err := os.Stat(vaultPath)
	if err != nil {
		return Check{"Secrets scan", StatusWarn, "vault not found, skipping"}
	}
	if !info.IsDir() {
		return Check{"Secrets scan", StatusWarn, "vault path is not a directory, skipping"}
	}

	var found int
	var examples []string
	_ = filepath.WalkDir(vaultPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		r := secretspkg.Scan(string(data))
		if r.HasSecrets {
			rel, _ := filepath.Rel(vaultPath, path)
			found++
			if len(examples) < 3 {
				examples = append(examples, rel)
			}
		}
		return nil
	})

	if found > 0 {
		return Check{
			"Secrets scan",
			StatusWarn,
			fmt.Sprintf("found %d vault note(s) with potential secrets: %s", found, strings.Join(examples, ", ")),
		}
	}
	return Check{"Secrets scan", StatusPass, "no secrets detected in vault notes"}
}

func AllChecks(configPath, vaultPath string) []Check {
	return []Check{
		ConfigCheck(configPath),
		VaultStructureCheck(vaultPath),
		SecretsCheck(vaultPath),
	}
}
