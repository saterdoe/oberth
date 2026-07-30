//go:build windows

package toolrunner

import (
	"os/exec"
	"strconv"
)

func configureProcessTree(_ *exec.Cmd) {}

func killProcessTree(command *exec.Cmd) {
	if command.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid)).Run()
	_ = command.Process.Kill()
}
