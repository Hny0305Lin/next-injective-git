//go:build windows

package bootstrap

import (
	"os/exec"
	"syscall"
)

func configureDetached(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW keeps the native daemon invisible without the shutdown
	// behavior of DETACHED_PROCESS, while CREATE_NEW_PROCESS_GROUP isolates it
	// from Ctrl+C delivered to the setup command's console.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000200,
		HideWindow:    true,
	}
}
