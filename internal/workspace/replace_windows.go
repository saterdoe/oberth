//go:build windows

package workspace

import (
	"os"
)

func replaceFile(source, target string) error {
	backup := target + ".oberth-backup"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	return os.Remove(backup)
}
