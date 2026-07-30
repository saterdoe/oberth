//go:build windows

package idelaunch

import (
	"os/exec"
	"syscall"
)

func setPlatformAttributes(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
