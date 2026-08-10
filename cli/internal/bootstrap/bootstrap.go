// Package bootstrap installs the pinned push-time dependencies managed by igit.
package bootstrap

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/klauspost/compress/zstd"
)

//go:embed deps.json
var manifestJSON []byte

type Manifest map[string]Dependency

type Dependency struct {
	Version   string              `json:"version"`
	Artifacts map[string]Artifact `json:"artifacts"`
}

type Artifact struct {
	URLs    []string          `json:"urls"`
	SHA256  string            `json:"sha256"`
	Archive string            `json:"archive"`
	Payload string            `json:"payload,omitempty"`
	Files   map[string]string `json:"files"`
}

type Options struct {
	Force      bool
	SkipKubo   bool
	Progress   io.Writer
	HTTPClient *http.Client
}

type Result struct {
	InjectivedBin string
	IPFSBin       string
	Installed     []string
	Reused        []string
	KuboStarted   bool
}

func LoadManifest() (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return nil, fmt.Errorf("parse embedded dependency manifest: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) error {
	for _, name := range []string{"kubo", "injectived"} {
		dep, ok := manifest[name]
		if !ok || strings.TrimSpace(dep.Version) == "" {
			return fmt.Errorf("dependency manifest is missing %s version", name)
		}
		if len(dep.Artifacts) == 0 {
			return fmt.Errorf("dependency manifest is missing %s artifacts", name)
		}
		for platform, artifact := range dep.Artifacts {
			if len(artifact.URLs) == 0 {
				return fmt.Errorf("%s %s artifact has no download URLs", name, platform)
			}
			for _, artifactURL := range artifact.URLs {
				if !strings.HasPrefix(artifactURL, "https://") {
					return fmt.Errorf("%s %s artifact URL must use HTTPS", name, platform)
				}
			}
			if decoded, err := hex.DecodeString(artifact.SHA256); err != nil || len(decoded) != sha256.Size {
				return fmt.Errorf("%s %s artifact has invalid SHA-256", name, platform)
			}
			if artifact.Archive != "zip" && artifact.Archive != "tar.gz" && artifact.Archive != "tar.gz+tar.zst" {
				return fmt.Errorf("%s %s artifact has unsupported archive %q", name, platform, artifact.Archive)
			}
			if artifact.Archive == "tar.gz+tar.zst" && strings.TrimSpace(artifact.Payload) == "" {
				return fmt.Errorf("%s %s nested artifact has no payload", name, platform)
			}
			if len(artifact.Files) == 0 {
				return fmt.Errorf("%s %s artifact has no files", name, platform)
			}
		}
	}
	return nil
}

// Prepare installs missing dependencies, preserves working user-managed tools,
// and returns a config updated with absolute executable paths.
func Prepare(ctx context.Context, cfg config.Config, opts Options) (config.Config, Result, error) {
	manifest, err := LoadManifest()
	if err != nil {
		return cfg, Result{}, err
	}
	if opts.Progress == nil {
		opts.Progress = io.Discard
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Minute}
	}
	root, err := config.Dir()
	if err != nil {
		return cfg, Result{}, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return cfg, Result{}, err
	}

	result := Result{}
	injectived, installed, err := ensureDependency(ctx, root, "injectived", cfg.InjectivedBin, []string{"version"}, manifest["injectived"], opts)
	if err != nil {
		return cfg, result, err
	}
	cfg.InjectivedBin = injectived
	result.InjectivedBin = injectived
	if installed {
		result.Installed = append(result.Installed, "injectived "+manifest["injectived"].Version)
	} else {
		result.Reused = append(result.Reused, "injectived")
	}

	if !opts.SkipKubo {
		ipfs, installed, err := ensureDependency(ctx, root, "kubo", cfg.IPFSBin, []string{"version", "--number"}, manifest["kubo"], opts)
		if err != nil {
			return cfg, result, err
		}
		cfg.IPFSBin = ipfs
		result.IPFSBin = ipfs
		if installed {
			result.Installed = append(result.Installed, "Kubo "+manifest["kubo"].Version)
		} else {
			result.Reused = append(result.Reused, "Kubo")
		}
		if err := StartKubo(ctx, cfg, opts.Progress); err != nil {
			return cfg, result, err
		}
		result.KuboStarted = true
	}
	if cfg.ContractAddress == "" {
		cfg.ContractAddress = config.DefaultContractAddress
	}
	return cfg, result, nil
}

func ensureDependency(ctx context.Context, root, name, configured string, versionArgs []string, dep Dependency, opts Options) (string, bool, error) {
	if !opts.Force {
		if binary, ok := workingBinary(ctx, configured, versionArgs); ok {
			fmt.Fprintf(opts.Progress, "Using existing %s: %s\n", name, binary)
			return binary, false, nil
		}
	}

	platform := runtime.GOOS + "-" + runtime.GOARCH
	artifact, ok := dep.Artifacts[platform]
	if !ok {
		return "", false, fmt.Errorf("automatic %s installation is not available for %s; install it manually and configure its path", name, platform)
	}
	installDir := filepath.Join(root, "deps", name, dep.Version, platform)
	binaryName := name
	if name == "kubo" {
		binaryName = "ipfs"
	}
	rawBinary := filepath.Join(installDir, binaryName)
	wrapper := filepath.Join(root, "bin", binaryName)
	if runtime.GOOS == "windows" {
		wrapper += ".cmd"
	}
	if !opts.Force {
		if binary, ok := workingBinary(ctx, wrapper, versionArgs); ok {
			fmt.Fprintf(opts.Progress, "Using igit-managed %s %s\n", name, dep.Version)
			return binary, false, nil
		}
	}

	installParent := filepath.Dir(installDir)
	if err := os.MkdirAll(installParent, 0o700); err != nil {
		return "", false, err
	}
	archivePath, err := downloadArtifact(ctx, root, name, dep.Version, artifact, opts)
	if err != nil {
		return "", false, err
	}
	defer os.Remove(archivePath)
	staging, err := os.MkdirTemp(installParent, "."+platform+"-staging-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(staging)
	if err := extractArtifact(archivePath, staging, artifact); err != nil {
		return "", false, fmt.Errorf("extract %s %s: %w", name, dep.Version, err)
	}
	if err := activateManagedInstall(root, staging, installDir); err != nil {
		return "", false, fmt.Errorf("activate %s %s: %w", name, dep.Version, err)
	}
	if err := writeWrapper(wrapper, rawBinary, installDir, name == "injectived"); err != nil {
		return "", false, err
	}
	if binary, ok := workingBinary(ctx, wrapper, versionArgs); ok {
		fmt.Fprintf(opts.Progress, "Installed %s %s: %s\n", name, dep.Version, binary)
		return binary, true, nil
	}
	return "", false, fmt.Errorf("installed %s %s but its version command failed", name, dep.Version)
}

func workingBinary(ctx context.Context, configured string, args []string) (string, bool) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", false
	}
	path, err := exec.LookPath(configured)
	if err != nil {
		return "", false
	}
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := exec.CommandContext(checkCtx, path, args...).Run(); err != nil {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return path, true
}

func downloadArtifact(ctx context.Context, root, name, version string, artifact Artifact, opts Options) (string, error) {
	downloadDir := filepath.Join(root, "downloads")
	if err := os.MkdirAll(downloadDir, 0o700); err != nil {
		return "", err
	}
	var failures []string
	for _, artifactURL := range artifact.URLs {
		path, err := downloadURL(ctx, downloadDir, name, version, artifactURL, artifact.SHA256, opts)
		if err == nil {
			return path, nil
		}
		failures = append(failures, artifactURL+": "+err.Error())
		fmt.Fprintf(opts.Progress, "Download source failed, trying the next pinned source: %v\n", err)
	}
	return "", fmt.Errorf("download %s failed from every pinned source: %s", name, strings.Join(failures, "; "))
}

func downloadURL(ctx context.Context, downloadDir, name, version, artifactURL, expectedSHA256 string, opts Options) (string, error) {
	tmp, err := os.CreateTemp(downloadDir, name+"-"+version+"-*.download")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(path)
		}
	}()

	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	fmt.Fprintf(opts.Progress, "Downloading %s %s from %s\n", name, version, artifactURL)
	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "igit-setup")
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", name, resp.StatusCode)
	}
	hash := sha256.New()
	timer := time.AfterFunc(45*time.Second, cancel)
	reader := &activityReader{
		reader: resp.Body, timer: timer, timeout: 45 * time.Second,
		progress: opts.Progress, name: name, total: resp.ContentLength, nextReport: 8 << 20,
	}
	if _, err := io.Copy(io.MultiWriter(tmp, hash), reader); err != nil {
		timer.Stop()
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	timer.Stop()
	if err := tmp.Close(); err != nil {
		return "", err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, expectedSHA256) {
		return "", fmt.Errorf("download %s failed SHA-256 verification: got %s, want %s", name, got, expectedSHA256)
	}
	fmt.Fprintf(opts.Progress, "Verified %s %s (%s)\n", name, version, got)
	ok = true
	return path, nil
}

type activityReader struct {
	reader     io.Reader
	timer      *time.Timer
	timeout    time.Duration
	progress   io.Writer
	name       string
	total      int64
	written    int64
	nextReport int64
}

func (r *activityReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.timer.Reset(r.timeout)
		r.written += int64(n)
		if r.progress != nil && r.written >= r.nextReport {
			if r.total > 0 {
				fmt.Fprintf(r.progress, "Downloaded %s: %d/%d MiB\n", r.name, r.written>>20, r.total>>20)
			} else {
				fmt.Fprintf(r.progress, "Downloaded %s: %d MiB\n", r.name, r.written>>20)
			}
			r.nextReport += 8 << 20
		}
	}
	return n, err
}

func extractArtifact(archivePath, destination string, artifact Artifact) error {
	switch artifact.Archive {
	case "zip":
		return extractZip(archivePath, destination, artifact.Files)
	case "tar.gz":
		return extractTarGz(archivePath, destination, artifact.Files)
	case "tar.gz+tar.zst":
		return extractNestedTarZst(archivePath, destination, artifact.Payload, artifact.Files)
	default:
		return fmt.Errorf("unsupported archive format %q", artifact.Archive)
	}
}

func extractZip(path, destination string, wanted map[string]string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	found := make(map[string]bool)
	for _, entry := range r.File {
		target, ok := wanted[filepath.ToSlash(entry.Name)]
		if !ok {
			continue
		}
		src, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeExtracted(destination, target, src)
		src.Close()
		if err != nil {
			return err
		}
		found[filepath.ToSlash(entry.Name)] = true
	}
	return ensureExtracted(wanted, found)
}

func extractTarGz(path, destination string, wanted map[string]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	return extractTarReader(tar.NewReader(gz), destination, wanted)
}

func extractNestedTarZst(path, destination, payload string, wanted map[string]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	outer := tar.NewReader(gz)
	for {
		header, err := outer.Next()
		if err == io.EOF {
			return fmt.Errorf("archive is missing nested payload %s", payload)
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(strings.TrimPrefix(header.Name, "./"))
		if name != filepath.ToSlash(payload) || header.Typeflag != tar.TypeReg {
			continue
		}
		decoder, err := zstd.NewReader(outer, zstd.WithDecoderConcurrency(1))
		if err != nil {
			return err
		}
		err = extractTarReader(tar.NewReader(decoder), destination, wanted)
		decoder.Close()
		return err
	}
}

func extractTarReader(tr *tar.Reader, destination string, wanted map[string]string) error {
	found := make(map[string]bool)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(strings.TrimPrefix(header.Name, "./"))
		target, ok := wanted[name]
		if !ok || header.Typeflag != tar.TypeReg {
			continue
		}
		if err := writeExtracted(destination, target, tr); err != nil {
			return err
		}
		found[name] = true
	}
	return ensureExtracted(wanted, found)
}

func writeExtracted(destination, relative string, src io.Reader) error {
	relative = filepath.Clean(relative)
	if relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe destination path %q", relative)
	}
	target := filepath.Join(destination, relative)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	base := filepath.Base(target)
	if base == "ipfs" || base == "injectived" {
		mode = 0o755
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func ensureExtracted(wanted map[string]string, found map[string]bool) error {
	var missing []string
	for source := range wanted {
		if !found[filepath.ToSlash(source)] {
			missing = append(missing, source)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("archive is missing required files: %s", strings.Join(missing, ", "))
}

func writeWrapper(path, binary, libraryDir string, withLibraryPath bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var content []byte
	if runtime.GOOS == "windows" {
		content = []byte(fmt.Sprintf("@echo off\r\n\"%s\" %%*\r\n", binary))
	} else {
		var script strings.Builder
		script.WriteString("#!/bin/sh\n")
		if withLibraryPath {
			fmt.Fprintf(&script, "export LD_LIBRARY_PATH=%q${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}\n", libraryDir)
			fmt.Fprintf(&script, "export DYLD_FALLBACK_LIBRARY_PATH=%q${DYLD_FALLBACK_LIBRARY_PATH:+:$DYLD_FALLBACK_LIBRARY_PATH}\n", libraryDir)
		}
		fmt.Fprintf(&script, "exec %q \"$@\"\n", binary)
		content = []byte(script.String())
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-staging-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(tmpPath, path)
}

func activateManagedInstall(root, staging, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	managedRoot := filepath.Join(rootAbs, "deps") + string(filepath.Separator)
	if !strings.HasPrefix(targetAbs+string(filepath.Separator), managedRoot) {
		return fmt.Errorf("refusing to replace unmanaged path %s", targetAbs)
	}
	if _, err := os.Stat(targetAbs); os.IsNotExist(err) {
		return os.Rename(staging, targetAbs)
	} else if err != nil {
		return err
	}
	backup, err := os.MkdirTemp(filepath.Dir(targetAbs), ".previous-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := os.Rename(targetAbs, backup); err != nil {
		return err
	}
	if err := os.Rename(staging, targetAbs); err != nil {
		_ = os.Rename(backup, targetAbs)
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}

func StartKubo(ctx context.Context, cfg config.Config, progress io.Writer) error {
	bin := strings.TrimSpace(cfg.IPFSBin)
	if bin == "" {
		bin = "ipfs"
	}
	if kuboReachable(ctx, cfg.IPFSAPI) {
		if installed, _ := ensureKuboUserService(ctx, bin, false, progress); installed {
			fmt.Fprintln(progress, "Kubo daemon is reachable and enabled for user-session startup")
		} else {
			fmt.Fprintln(progress, "Kubo daemon is already reachable")
		}
		return nil
	}
	initialized, err := kuboRepoInitialized()
	if err != nil {
		return err
	}
	if !initialized {
		fmt.Fprintln(progress, "Initializing the local Kubo repository")
		initCtx, initCancel := context.WithTimeout(ctx, 2*time.Minute)
		err = exec.CommandContext(initCtx, bin, "init", "--profile=server").Run()
		initCancel()
		if err != nil {
			return fmt.Errorf("initialize Kubo: %w", err)
		}
	}
	if installed, _ := ensureKuboUserService(ctx, bin, true, progress); installed {
		fmt.Fprintln(progress, "Starting Kubo through the user systemd service")
		return waitForKubo(ctx, cfg.IPFSAPI, "the user systemd service")
	}
	root, err := config.Dir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(root, "kubo.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "daemon", "--enable-gc")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	configureDetached(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start Kubo daemon: %w", err)
	}
	_ = cmd.Process.Release()
	_ = logFile.Close()
	fmt.Fprintf(progress, "Starting Kubo daemon (log: %s)\n", logPath)
	return waitForKubo(ctx, cfg.IPFSAPI, logPath)
}

func kuboRepoInitialized() (bool, error) {
	repo := strings.TrimSpace(os.Getenv("IPFS_PATH"))
	if repo == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, err
		}
		repo = filepath.Join(home, ".ipfs")
	}
	_, err := os.Stat(filepath.Join(repo, "config"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func waitForKubo(ctx context.Context, api, source string) error {
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Second)
		ready := kuboReachable(probeCtx, api)
		probeCancel()
		if ready {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("Kubo did not become reachable at %s; inspect %s", api, source)
}

func ensureKuboUserService(ctx context.Context, bin string, start bool, progress io.Writer) (bool, error) {
	if runtime.GOOS != "linux" {
		return false, nil
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := exec.CommandContext(probeCtx, "systemctl", "--user", "is-system-running").Run()
	cancel()
	if err != nil {
		return false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		return false, err
	}
	unitPath := filepath.Join(unitDir, "igit-kubo.service")
	unit := kuboServiceUnit(bin, os.Getenv("IPFS_PATH"))
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return false, err
	}
	commands := [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "igit-kubo.service"}}
	if start {
		commands = append(commands, []string{"--user", "start", "igit-kubo.service"})
	}
	for _, args := range commands {
		commandCtx, commandCancel := context.WithTimeout(ctx, 15*time.Second)
		out, err := exec.CommandContext(commandCtx, "systemctl", args...).CombinedOutput()
		commandCancel()
		if err != nil {
			fmt.Fprintf(progress, "User systemd setup unavailable (%s); using a detached Kubo daemon\n", strings.TrimSpace(string(out)))
			return false, nil
		}
	}
	return true, nil
}

func kuboServiceUnit(bin, ipfsPath string) string {
	escape := func(value string) string {
		return strconv.Quote(strings.ReplaceAll(value, "%", "%%"))
	}
	var environment string
	if strings.TrimSpace(ipfsPath) != "" {
		environment = "Environment=" + escape("IPFS_PATH="+ipfsPath) + "\n"
	}
	return `[Unit]
Description=igit local Kubo daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=` + escape(bin) + ` daemon --enable-gc
` + environment + `Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`
}

func kuboReachable(ctx context.Context, api string) bool {
	api = strings.TrimRight(strings.TrimSpace(api), "/")
	if api == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api+"/api/v0/version", nil)
	if err != nil {
		return false
	}
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
