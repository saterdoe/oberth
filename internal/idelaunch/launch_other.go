//go:build !windows

package idelaunch

import "os/exec"

func setPlatformAttributes(command *exec.Cmd) {}
