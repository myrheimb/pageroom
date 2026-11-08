package app

import (
	"os/exec"
	"runtime"
)

func openPath(p string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", p).Start()
	case "windows":
		return exec.Command("explorer", p).Start()
	default:
		return exec.Command("xdg-open", p).Start()
	}
}
