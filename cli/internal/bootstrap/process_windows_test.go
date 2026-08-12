//go:build windows

package bootstrap

import (
	"os/exec"
	"testing"
)

func TestConfigureDetachedUsesHiddenIndependentProcessGroup(t *testing.T) {
	cmd := exec.Command("ipfs.exe", "daemon")
	configureDetached(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("configureDetached did not set SysProcAttr")
	}
	const (
		detachedProcess       = 0x00000008
		createNewProcessGroup = 0x00000200
		createNoWindow        = 0x08000000
	)
	flags := cmd.SysProcAttr.CreationFlags
	if flags&createNoWindow == 0 || flags&createNewProcessGroup == 0 || !cmd.SysProcAttr.HideWindow {
		t.Fatalf("Windows daemon flags=%#x hideWindow=%v", flags, cmd.SysProcAttr.HideWindow)
	}
	if flags&detachedProcess != 0 {
		t.Fatalf("Windows daemon still uses DETACHED_PROCESS: flags=%#x", flags)
	}
}
