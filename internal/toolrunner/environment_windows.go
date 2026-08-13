//go:build windows

package toolrunner

func minimalEnvironmentNames() []string {
	return []string{"PATH", "PATHEXT", "SYSTEMROOT", "TEMP", "TMP", "WINDIR"}
}
