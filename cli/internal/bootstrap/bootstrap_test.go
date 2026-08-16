package bootstrap

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestEmbeddedManifestIsValid(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest["kubo"].Version != "0.42.0" || len(manifest) != 1 {
		t.Fatalf("unexpected dependency versions: %#v", manifest)
	}
}

func TestEmbeddedWindowsKuboArtifactExtractsIPFSExecutable(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	artifact, ok := manifest["kubo"].Artifacts["windows-amd64"]
	if !ok {
		t.Fatal("embedded manifest is missing windows-amd64 Kubo")
	}
	if artifact.Archive != "zip" || artifact.Files["kubo/ipfs.exe"] != "ipfs.exe" {
		t.Fatalf("unexpected Windows Kubo artifact: %#v", artifact)
	}
	if artifact.SHA256 != "f24e4d24445c8abf7bd26bd034cb9f14dac77e30452731a617d2e8e2f2ceb150" {
		t.Fatalf("unexpected Windows Kubo SHA-256: %s", artifact.SHA256)
	}
	const cid = "QmWmvYCTA74ise9EYx5PRMBCdwo4SMjERgWtTjBBEqWBHn"
	wantURLs := []string{
		"https://gateway.pinata.cloud/ipfs/" + cid + "/kubo/v0.42.0/kubo_v0.42.0_windows-amd64.zip",
		"https://igit-hk.haohanyh.ovh/ipfs/" + cid + "/kubo/v0.42.0/kubo_v0.42.0_windows-amd64.zip",
		"https://igit-us.haohanyh.ovh/ipfs/" + cid + "/kubo/v0.42.0/kubo_v0.42.0_windows-amd64.zip",
		"https://dist.ipfs.tech/kubo/v0.42.0/kubo_v0.42.0_windows-amd64.zip",
		"https://github.com/ipfs/kubo/releases/download/v0.42.0/kubo_v0.42.0_windows-amd64.zip",
	}
	if len(artifact.URLs) != len(wantURLs) {
		t.Fatalf("Windows Kubo URLs = %#v, want %#v", artifact.URLs, wantURLs)
	}
	for index := range wantURLs {
		if artifact.URLs[index] != wantURLs[index] {
			t.Fatalf("Windows Kubo URL %d = %q, want %q", index, artifact.URLs[index], wantURLs[index])
		}
	}

	archive := filepath.Join(t.TempDir(), "kubo.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	writeZipEntry(t, zw, "kubo/ipfs.exe", "windows-ipfs-binary")
	writeZipEntry(t, zw, "kubo/README.md", "ignored")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := extractArtifact(archive, dest, artifact); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "ipfs.exe"))
	if err != nil || string(got) != "windows-ipfs-binary" {
		t.Fatalf("extracted Windows Kubo data=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
		t.Fatal("unlisted Windows Kubo archive file was extracted")
	}
}

func TestPrepareKuboDoesNotCheckInjectived(t *testing.T) {
	commandDir := t.TempDir()
	marker := filepath.Join(commandDir, "injectived-called")
	ipfs := writeTestCommand(t, commandDir, "ipfs", "exit /b 0", "exit 0")
	injectived := writeTestCommand(
		t,
		commandDir,
		"injectived",
		fmt.Sprintf("> \"%s\" echo called\r\nexit /b 0", marker),
		fmt.Sprintf("printf called > %q\nexit 0", marker),
	)
	t.Setenv("IGIT_HOME", t.TempDir())
	// Keep Linux test environments from discovering a live user systemd while
	// still allowing the absolute fake Kubo path to execute.
	t.Setenv("PATH", commandDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v0/version" {
			t.Errorf("unexpected Kubo probe: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.Config{
		ContractAddress: "",
		InjectivedBin:   injectived,
		IPFSBin:         ipfs,
		IPFSAPI:         server.URL,
	}
	updated, result, err := PrepareKubo(context.Background(), cfg, Options{Progress: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if updated.InjectivedBin != injectived {
		t.Fatalf("injectived config changed: got %q want %q", updated.InjectivedBin, injectived)
	}
	if updated.ContractAddress != "" {
		t.Fatalf("Kubo-only preparation changed contract config: %q", updated.ContractAddress)
	}
	if !result.KuboStarted || len(result.Reused) != 1 || result.Reused[0] != "Kubo" || len(result.Installed) != 0 {
		t.Fatalf("unexpected Kubo-only result: %#v", result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("PrepareKubo invoked injectived; marker stat error: %v", err)
	}
}

func TestPrepareKuboSkipIsNoOp(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-created")
	t.Setenv("IGIT_HOME", home)
	cfg := config.Config{
		ContractAddress: "leave-empty",
		InjectivedBin:   "never-check-injectived",
		IPFSBin:         "never-check-ipfs",
	}
	updated, result, err := PrepareKubo(context.Background(), cfg, Options{SkipKubo: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContractAddress != cfg.ContractAddress || updated.InjectivedBin != cfg.InjectivedBin || updated.IPFSBin != cfg.IPFSBin {
		t.Fatalf("SkipKubo changed config: got %#v want %#v", updated, cfg)
	}
	if result.IPFSBin != "" || len(result.Installed) != 0 || len(result.Reused) != 0 || result.KuboStarted {
		t.Fatalf("SkipKubo returned work result: %#v", result)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("SkipKubo created IGIT_HOME; stat error: %v", err)
	}
}

func TestExtractZipSelectsOnlyManifestFiles(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "dep.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	writeZipEntry(t, zw, "injectived", "binary")
	writeZipEntry(t, zw, "ignored.txt", "ignored")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := extractZip(archive, dest, map[string]string{"injectived": "injectived"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "injectived"))
	if err != nil || string(data) != "binary" {
		t.Fatalf("extracted data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "ignored.txt")); !os.IsNotExist(err) {
		t.Fatal("unlisted archive file was extracted")
	}
}

func TestExtractTarGzRequiresEveryManifestFile(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "dep.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	payload := []byte("ipfs-binary")
	if err := tw.WriteHeader(&tar.Header{Name: "kubo/ipfs", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(tw, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	err = extractTarGz(archive, t.TempDir(), map[string]string{"kubo/ipfs": "ipfs", "kubo/README.md": "README.md"})
	if err == nil {
		t.Fatal("expected missing required file error")
	}
}

func TestDownloadArtifactUsesNextPinnedSourceAndVerifiesHash(t *testing.T) {
	payload := []byte("verified dependency")
	sum := sha256.Sum256(payload)
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	defer failed.Close()
	succeeded := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer succeeded.Close()

	root := t.TempDir()
	artifact := Artifact{URLs: []string{failed.URL, succeeded.URL}, SHA256: fmt.Sprintf("%x", sum)}
	path, err := downloadArtifact(context.Background(), root, "dependency", "1.0.0", artifact, Options{Progress: io.Discard, HTTPClient: succeeded.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("downloaded data=%q err=%v", got, err)
	}

	artifact.SHA256 = fmt.Sprintf("%064d", 0)
	if _, err := downloadArtifact(context.Background(), root, "dependency", "1.0.0", artifact, Options{Progress: io.Discard, HTTPClient: succeeded.Client()}); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestDownloadArtifactBoundsAStalledSourceAndFallsBack(t *testing.T) {
	payload := []byte("fallback dependency")
	sum := sha256.Sum256(payload)
	stalled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer stalled.Close()
	succeeded := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer succeeded.Close()

	timeouts := downloadTimeouts{
		connect: 250 * time.Millisecond, tlsHandshake: 250 * time.Millisecond,
		responseHeader: 250 * time.Millisecond, idle: 75 * time.Millisecond, total: time.Second,
	}
	artifact := Artifact{URLs: []string{stalled.URL, succeeded.URL}, SHA256: fmt.Sprintf("%x", sum)}
	started := time.Now()
	path, err := downloadArtifact(context.Background(), t.TempDir(), "dependency", "1.0.0", artifact, Options{
		Progress: io.Discard,
		timeouts: &timeouts,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("stalled source delayed fallback for %s", elapsed)
	}
}

func TestDownloadArtifactBoundsStalledHeadersAndFallsBack(t *testing.T) {
	payload := []byte("header-timeout fallback")
	sum := sha256.Sum256(payload)
	stalled := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer stalled.Close()
	succeeded := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer succeeded.Close()

	timeouts := downloadTimeouts{
		connect: 250 * time.Millisecond, tlsHandshake: 250 * time.Millisecond,
		responseHeader: 75 * time.Millisecond, idle: 250 * time.Millisecond, total: time.Second,
	}
	artifact := Artifact{URLs: []string{stalled.URL, succeeded.URL}, SHA256: fmt.Sprintf("%x", sum)}
	path, err := downloadArtifact(context.Background(), t.TempDir(), "dependency", "1.0.0", artifact, Options{
		Progress: io.Discard,
		timeouts: &timeouts,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
}

func TestDownloadArtifactBoundsAnActiveSourceByTotalTimeout(t *testing.T) {
	payload := []byte("total-timeout fallback")
	sum := sha256.Sum256(payload)
	trickling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("test response does not support flushing")
			return
		}
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = w.Write([]byte("x"))
				flusher.Flush()
			}
		}
	}))
	defer trickling.Close()
	succeeded := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer succeeded.Close()

	timeouts := downloadTimeouts{
		connect: 250 * time.Millisecond, tlsHandshake: 250 * time.Millisecond,
		responseHeader: 250 * time.Millisecond, idle: time.Second, total: 250 * time.Millisecond,
	}
	artifact := Artifact{URLs: []string{trickling.URL, succeeded.URL}, SHA256: fmt.Sprintf("%x", sum)}
	started := time.Now()
	path, err := downloadArtifact(context.Background(), t.TempDir(), "dependency", "1.0.0", artifact, Options{
		Progress: io.Discard,
		timeouts: &timeouts,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if elapsed := time.Since(started); elapsed < 200*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("active source total timeout and fallback took %s", elapsed)
	}
}

func TestDownloadHTTPClientBoundsConnectAndHeaders(t *testing.T) {
	timeouts := downloadTimeouts{
		connect: 11 * time.Millisecond, tlsHandshake: 12 * time.Millisecond,
		responseHeader: 13 * time.Millisecond, idle: 14 * time.Millisecond, total: 15 * time.Millisecond,
	}
	client := newDownloadHTTPClient(timeouts)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("download transport = %T", client.Transport)
	}
	if transport.TLSHandshakeTimeout != timeouts.tlsHandshake || transport.ResponseHeaderTimeout != timeouts.responseHeader {
		t.Fatalf("download transport timeouts = TLS %s header %s", transport.TLSHandshakeTimeout, transport.ResponseHeaderTimeout)
	}
	if transport.ForceAttemptHTTP2 {
		t.Fatal("dependency downloads must not force HTTP/2 for long IPFS gateway streams")
	}
}

func TestForceDownloadFailureKeepsWorkingManagedInstall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	root := t.TempDir()
	platform := runtime.GOOS + "-" + runtime.GOARCH
	installDir := filepath.Join(root, "deps", "kubo", "1.0.0", platform)
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(installDir, "working-version")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	dep := Dependency{Version: "1.0.0", Artifacts: map[string]Artifact{
		platform: {URLs: []string{server.URL}, SHA256: fmt.Sprintf("%064d", 0), Archive: "zip", Files: map[string]string{"ipfs": "ipfs"}},
	}}
	_, _, err := ensureDependency(context.Background(), root, "kubo", "ipfs", []string{"version"}, dep, Options{
		Force: true, Progress: io.Discard, HTTPClient: server.Client(),
	})
	if err == nil {
		t.Fatal("expected download failure")
	}
	data, readErr := os.ReadFile(marker)
	if readErr != nil || string(data) != "keep" {
		t.Fatalf("working install was modified: data=%q err=%v", data, readErr)
	}
}

func TestForcedKuboExtractionFailureKeepsWorkingManagedInstall(t *testing.T) {
	payload := []byte("not a zip archive")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	root := t.TempDir()
	platform := runtime.GOOS + "-" + runtime.GOARCH
	installDir := filepath.Join(root, "deps", "kubo", "1.0.0", platform)
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(installDir, "working-version")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	dep := Dependency{Version: "1.0.0", Artifacts: map[string]Artifact{
		platform: {
			URLs:    []string{server.URL},
			SHA256:  fmt.Sprintf("%x", sum),
			Archive: "zip",
			Files:   map[string]string{"kubo/ipfs.exe": "ipfs.exe"},
		},
	}}
	_, _, err := ensureDependency(context.Background(), root, "kubo", "", []string{"version", "--number"}, dep, Options{
		Force: true, Progress: io.Discard, HTTPClient: server.Client(),
	})
	if err == nil {
		t.Fatal("expected extraction failure")
	}
	data, readErr := os.ReadFile(marker)
	if readErr != nil || string(data) != "keep" {
		t.Fatalf("working install was modified: data=%q err=%v", data, readErr)
	}
}

func TestKuboServiceUnitEscapesExecutableAndIPFSPath(t *testing.T) {
	unit := kuboServiceUnit("/home/user/igit%tools/ipfs", "/home/user/ipfs data")
	for _, want := range []string{
		`ExecStart="/home/user/igit%%tools/ipfs" daemon --enable-gc`,
		`Environment="IPFS_PATH=/home/user/ipfs data"`,
		"WantedBy=default.target",
	} {
		if !bytes.Contains([]byte(unit), []byte(want)) {
			t.Errorf("unit does not contain %q:\n%s", want, unit)
		}
	}
}

func TestKuboRepoInitializedUsesConfigFileWithoutLockingRepo(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("IPFS_PATH", repo)
	initialized, err := kuboRepoInitialized()
	if err != nil || initialized {
		t.Fatalf("empty repo initialized=%v err=%v", initialized, err)
	}
	if err := os.WriteFile(filepath.Join(repo, "config"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	initialized, err = kuboRepoInitialized()
	if err != nil || !initialized {
		t.Fatalf("configured repo initialized=%v err=%v", initialized, err)
	}
}

func TestWriteExtractedRejectsTraversal(t *testing.T) {
	if err := writeExtracted(t.TempDir(), "../outside", bytes.NewBufferString("bad")); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func writeZipEntry(t *testing.T, zw *zip.Writer, name, value string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, value); err != nil {
		t.Fatal(err)
	}
}

func writeTestCommand(t *testing.T, dir, name, windowsBody, unixBody string) string {
	t.Helper()
	var content string
	if runtime.GOOS == "windows" {
		name += ".cmd"
		content = "@echo off\r\n" + windowsBody + "\r\n"
	} else {
		content = "#!/bin/sh\n" + unixBody + "\n"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
