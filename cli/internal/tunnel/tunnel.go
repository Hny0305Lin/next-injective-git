// Package tunnel manages local SSH forwards to the private Kubo control APIs.
package tunnel

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// statePath returns a tunnel state file kept outside the repository.
func statePath(name, ext string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tunnels", name+ext), nil
}

// Start opens an SSH local forward with a loopback-only local listener. The
// caller's known_hosts policy remains in effect: first use must be verified by
// the operator, rather than silently accepting a potentially forged host key.
func Start(profile config.Tunnel) error {
	if profile.Name == "" || profile.Host == "" || profile.User == "" || profile.LocalAddr == "" || profile.RemoteAddr == "" {
		return fmt.Errorf("incomplete tunnel profile")
	}
	if profile.IdentityFile == "" {
		return fmt.Errorf("no identity file configured for %s (run `igit tunnel key %s <private-key-path>`)", profile.Name, profile.Name)
	}
	pidPath, err := statePath(profile.Name, ".pid")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o700); err != nil {
		return err
	}
	if Check(profile) == nil {
		return nil
	}
	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale tunnel state: %w", err)
	}
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=20",
		"-o", "ServerAliveCountMax=3",
		"-N",
		"-L", profile.LocalAddr + ":" + profile.RemoteAddr,
		"-i", profile.IdentityFile,
		profile.User + "@" + profile.Host,
	}
	logPath, err := statePath(profile.Name, ".log")
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command("ssh", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	pid, err := startDetached(cmd)
	if err != nil {
		return fmt.Errorf("open %s tunnel: %w", profile.Name, err)
	}
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0o600); err != nil {
		_ = stopDetached(pid)
		return fmt.Errorf("record %s tunnel state: %w", profile.Name, err)
	}
	// SSH handshakes to the HK route can occasionally take several seconds.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if err := Check(profile); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = stopDetached(pid)
	_ = os.Remove(pidPath)
	return fmt.Errorf("%s tunnel did not become ready (see %s)", profile.Name, logPath)
}

// Stop closes the exact background SSH process created by Start.
func Stop(profile config.Tunnel) error {
	pidPath, err := statePath(profile.Name, ".pid")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil || pid <= 0 {
		return fmt.Errorf("invalid %s tunnel state", profile.Name)
	}
	if err := stopDetached(pid); err != nil {
		return fmt.Errorf("close %s tunnel: %w", profile.Name, err)
	}
	return os.Remove(pidPath)
}

// Check verifies that the locally forwarded address answers Kubo's version
// endpoint. It never contacts the remote server directly.
func Check(profile config.Tunnel) error {
	endpoint := "http://" + profile.LocalAddr + "/api/v0/version"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(endpoint, "application/octet-stream", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// APIURL is the loopback URL to use in ipfs_api after a tunnel is live.
func APIURL(profile config.Tunnel) string {
	return (&url.URL{Scheme: "http", Host: profile.LocalAddr}).String()
}
