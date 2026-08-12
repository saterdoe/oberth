//go:build !windows

package permission

func resolvePlatformPath(path string) (string, error) { return path, nil }
