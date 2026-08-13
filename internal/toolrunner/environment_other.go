//go:build !windows

package toolrunner

func minimalEnvironmentNames() []string {
	return []string{"HOME", "LANG", "PATH", "TMPDIR"}
}
