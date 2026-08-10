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

	"github.com/klauspost/compress/zstd"
)

func TestEmbeddedManifestIsValid(t *testing.T) {
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest["kubo"].Version != "0.42.0" || manifest["injectived"].Version != "1.17.2" {
		t.Fatalf("unexpected dependency versions: %#v", manifest)
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

func TestExtractNestedTarZst(t *testing.T) {
	var inner bytes.Buffer
	zstdWriter, err := zstd.NewWriter(&inner)
	if err != nil {
		t.Fatal(err)
	}
	innerTar := tar.NewWriter(zstdWriter)
	payload := []byte("injectived-binary")
	if err := innerTar.WriteHeader(&tar.Header{Name: "injectived-linux-x64", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := innerTar.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := innerTar.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zstdWriter.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "package.tgz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	outerTar := tar.NewWriter(gz)
	if err := outerTar.WriteHeader(&tar.Header{Name: "package/bin/injectived.tar.zst", Mode: 0o644, Size: int64(inner.Len()), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(outerTar, &inner); err != nil {
		t.Fatal(err)
	}
	if err := outerTar.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := extractNestedTarZst(archive, dest, "package/bin/injectived.tar.zst", map[string]string{"injectived-linux-x64": "injectived"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "injectived"))
	if err != nil || string(got) != string(payload) {
		t.Fatalf("extracted data=%q err=%v", got, err)
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
