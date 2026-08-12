//go:build windows

package bootstrap

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"golang.org/x/sys/windows"
)

// This opt-in test downloads the pinned artifact, initializes an isolated
// repository, starts the native daemon, probes it, and shuts it down. It is
// intentionally excluded from ordinary unit tests because it needs network
// access and binds Kubo's default local ports.
func TestNativeKuboLifecycleIntegration(t *testing.T) {
	if os.Getenv("IGIT_RUN_NATIVE_KUBO_INTEGRATION") != "1" {
		t.Skip("set IGIT_RUN_NATIVE_KUBO_INTEGRATION=1 to run the native Kubo lifecycle smoke test")
	}
	if runtime.GOARCH != "amd64" {
		t.Skip("the embedded Windows Kubo artifact currently supports amd64")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:5001")
	if err != nil {
		t.Fatalf("Kubo API port 5001 is already in use: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	igitHome := t.TempDir()
	ipfsRepo := filepath.Join(t.TempDir(), "ipfs-repo")
	t.Setenv("IGIT_HOME", igitHome)
	t.Setenv("IPFS_PATH", ipfsRepo)

	cfg := config.Defaults()
	cfg.IPFSBin = ""
	cfg.IPFSAPI = "http://127.0.0.1:5001"
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	updated, result, prepareErr := PrepareKubo(ctx, cfg, Options{
		Force:    true,
		Progress: io.Discard,
	})
	if strings.TrimSpace(updated.IPFSBin) != "" && result.KuboPID > 0 {
		defer shutdownIntegrationKubo(t, updated.IPFSBin, cfg.IPFSAPI, result.KuboPID)
	}
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}
	if !result.KuboStarted || result.KuboPID <= 0 || len(result.Installed) != 1 {
		t.Fatalf("unexpected native Kubo result: %#v", result)
	}

	rel, err := filepath.Rel(igitHome, updated.IPFSBin)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("managed Kubo path %q is outside IGIT_HOME %q", updated.IPFSBin, igitHome)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.IPFSAPI+"/api/v0/version", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("Kubo version probe returned HTTP %d", resp.StatusCode)
	}
}

func shutdownIntegrationKubo(t *testing.T, binary, api string, pid int) {
	t.Helper()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Errorf("open native Kubo process %d for cleanup: %v", pid, err)
		return
	}
	defer windows.CloseHandle(handle)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(shutdownCtx, binary, "shutdown").CombinedOutput(); err != nil {
		t.Errorf("shut down native Kubo: %v: %s", err, strings.TrimSpace(string(output)))
	}

	for shutdownCtx.Err() == nil {
		probeCtx, probeCancel := context.WithTimeout(shutdownCtx, 500*time.Millisecond)
		reachable := kuboReachable(probeCtx, api)
		probeCancel()
		if !reachable {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if shutdownCtx.Err() != nil {
		t.Errorf("native Kubo API remained reachable after shutdown")
	}

	waitResult, waitErr := windows.WaitForSingleObject(handle, 20_000)
	if waitErr == nil && waitResult == windows.WAIT_OBJECT_0 {
		return
	}
	// Kubo can finish closing its API before the Windows process releases its
	// executable. The integration harness owns this exact PID, so force cleanup
	// after the bounded grace period to keep the test hermetic.
	if err := windows.TerminateProcess(handle, 1); err != nil {
		t.Errorf("terminate lingering native Kubo process %d: %v", pid, err)
		return
	}
	if result, err := windows.WaitForSingleObject(handle, 10_000); err != nil || result != windows.WAIT_OBJECT_0 {
		t.Errorf("wait for terminated native Kubo process %d (result=%d, err=%v)", pid, result, err)
		return
	}
	t.Logf("native Kubo API stopped cleanly; process %d required bounded test cleanup", pid)
}
