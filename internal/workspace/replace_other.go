//go:build !windows

package workspace

import "os"

func replaceFile(source, target string) error {
	return os.Rename(source, target)
}
