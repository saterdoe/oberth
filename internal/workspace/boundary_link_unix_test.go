//go:build !windows

package workspace

import "os"

func makeDirectoryLink(link, target string) error { return os.Symlink(target, link) }
