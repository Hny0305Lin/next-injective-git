//go:build linux

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
	"syscall"
	"testing"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// This opt-in test exercises the pinned native Linux artifact. It uses an
// isolated repository and shuts down only the exact daemon PID it started.
func TestNativeKuboLifecycleIntegration(t *testing.T) {
	if os.Getenv("IGIT_RUN_NATIVE_KUBO_INTEGRATION") != "1" {
		t.Skip("set IGIT_RUN_NATIVE_KUBO_INTEGRATION=1 to run the native Kubo lifecycle smoke test")
	}
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("the CI native Kubo lifecycle smoke supports Linux amd64/arm64")
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
	updated, result, prepareErr := PrepareKubo(ctx, cfg, Options{Force: true, Progress: io.Discard})
	if strings.TrimSpace(updated.IPFSBin) != "" && result.KuboPID > 0 {
		defer shutdownIntegrationKubo(t, updated.IPFSBin, cfg.IPFSAPI, result.KuboPID)
	}
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}
	if !result.KuboStarted || result.KuboPID <= 0 || len(result.Installed) != 1 {
		t.Fatalf("unexpected native Kubo result: %#v", result)
	}
	if rel, err := filepath.Rel(igitHome, updated.IPFSBin); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "shutdown")
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("shut down native Kubo: %v: %s", err, strings.TrimSpace(string(output)))
	}
	for ctx.Err() == nil {
		probeCtx, probeCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		reachable := kuboReachable(probeCtx, api)
		probeCancel()
		if !reachable {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Errorf("native Kubo API remained reachable after shutdown")
	}
	if err := syscall.Kill(pid, 0); err == nil {
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			t.Errorf("terminate lingering native Kubo process %d: %v", pid, err)
		}
	}
}
