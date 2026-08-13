//go:build windows

package toolrunner

func minimalEnvironmentNames() []string {
	return []string{"LOCALAPPDATA", "PATH", "PATHEXT", "SYSTEMROOT", "TEMP", "TMP", "USERPROFILE", "WINDIR"}
}
