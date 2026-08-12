//go:build windows

package workspace

import (
	"fmt"
	"os/exec"
	"strings"
)

func makeDirectoryLink(link, target string) error {
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
