//go:build windows

package hugo

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func setProcessGroup(cmd *exec.Cmd) {
	// no console window flashing up behind the app
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
