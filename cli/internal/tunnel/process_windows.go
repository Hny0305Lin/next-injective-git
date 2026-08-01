//go:build windows

package tunnel

import (
	"fmt"
	"os/exec"
	"syscall"
)

func startDetached(cmd *exec.Cmd) (int, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func stopDetached(pid int) error {
	out, err := exec.Command("taskkill", "/PID", fmt.Sprint(pid), "/T", "/F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}
