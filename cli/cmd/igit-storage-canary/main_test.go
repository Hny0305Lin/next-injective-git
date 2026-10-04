package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// localGit mirrors internal/gitio test conventions: isolated config, fixed identity.
func localGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-config"), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.com", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.com")
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v %s", args, e, out)
	}
	return strings.TrimSpace(string(out))
}

const testConfig = `{
  "version": 1,
  "profiles": {
    "r2-writer": {
      "provider": "cloudflare-r2",
      "bucket": "igit-canary-test",
      "region": "auto",
      "accountId": "11111111111111111111111111111111",
      "prefix": "igit-canary/test",
      "credentialRef": { "kind": "env", "accessKeyEnv": "CANARY_TEST_WRITER_ID", "secretKeyEnv": "CANARY_TEST_WRITER_SECRET" }
    },
    "r2-reader": {
      "provider": "cloudflare-r2",
      "bucket": "igit-canary-test",
      "region": "auto",
      "accountId": "11111111111111111111111111111111",
      "prefix": "igit-canary/test",
      "credentialRef": { "kind": "env", "accessKeyEnv": "CANARY_TEST_READER_ID", "secretKeyEnv": "CANARY_TEST_READER_SECRET" }
    }
  },
  "repositories": [
    {
      "chainId": "1776",
      "suiteDirectory": "0x1111111111111111111111111111111111111111",
      "repoId": "0x2222222222222222222222222222222222222222222222222222222222222222",
      "writer": "r2-writer",
      "reader": "r2-reader"
    }
  ]
}`

func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	localGit(t, dir, "init", "--object-format=sha1", "-b", "main")
	if e := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("canary fixture"), 0o600); e != nil {
		t.Fatal(e)
	}
	localGit(t, dir, "add", ".")
	localGit(t, dir, "commit", "-m", "fixture commit")
	return dir
}

// TestPlanLocalAndGatedExecute keeps every pre-real-cloud gate offline: plan
// performs no credential resolution or network access, execute refuses without
// an approved plan hash, and any config drift after plan aborts execution.
func TestPlanLocalAndGatedExecute(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "storage.json")
	if e := os.WriteFile(cfg, []byte(testConfig), 0o600); e != nil {
		t.Fatal(e)
	}
	repo := fixtureRepo(t)
	rundir := filepath.Join(base, "run")
	t.Setenv("CANARY_TEST_WRITER_ID", "sentinel-writer-secret-value")
	t.Setenv("CANARY_TEST_READER_ID", "sentinel-reader-secret-value")

	if err := cmdPlan([]string{"-config", cfg, "-repo", repo, "-rundir", rundir}); err != nil {
		t.Fatalf("plan: %v", err)
	}
	planBytes, err := os.ReadFile(filepath.Join(rundir, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(planBytes, []byte("sentinel-writer-secret-value")) || bytes.Contains(planBytes, []byte("sentinel-reader-secret-value")) {
		t.Fatal("plan.json leaked an environment credential value")
	}
	if !bytes.Contains(planBytes, []byte("CANARY_TEST_WRITER_ID")) {
		t.Fatal("plan.json should name the writer env reference")
	}
	var doc planDoc
	if err := json.Unmarshal(planBytes, &doc); err != nil {
		t.Fatalf("plan.json parse: %v", err)
	}
	if doc.Provider != "cloudflare-r2" || doc.Bucket != "igit-canary-test" || doc.Prefix != "igit-canary/test" {
		t.Fatalf("plan binding mismatch: %+v", doc)
	}
	if _, err := os.Stat(filepath.Join(rundir, doc.PackRelPath)); err != nil {
		t.Fatalf("pack artifact missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rundir, doc.ManifestPath)); err != nil {
		t.Fatalf("manifest artifact missing: %v", err)
	}

	if err := cmdExecute([]string{"-config", cfg, "-rundir", rundir}); err == nil {
		t.Fatal("execute without -approve must fail")
	}
	if err := cmdExecute([]string{"-config", cfg, "-rundir", rundir, "-approve", strings.Repeat("0", 64)}); err == nil {
		t.Fatal("execute with a wrong approve hash must fail")
	}

	planSHA, _, err := fileSHA256(filepath.Join(rundir, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Correct hash but writer credentials unset: must stop before any network use.
	os.Unsetenv("CANARY_TEST_WRITER_ID")
	if err := cmdExecute([]string{"-config", cfg, "-rundir", rundir, "-approve", planSHA}); err == nil || !strings.Contains(err.Error(), "CANARY_TEST_WRITER_ID") {
		t.Fatalf("execute must require writer env, got: %v", err)
	}

	// Config drift after plan must abort even with a valid hash.
	t.Setenv("CANARY_TEST_WRITER_ID", "sentinel-writer-secret-value")
	drifted := strings.Replace(testConfig, "igit-canary/test", "igit-canary/other", 1)
	if e := os.WriteFile(cfg, []byte(drifted), 0o600); e != nil {
		t.Fatal(e)
	}
	if err := cmdExecute([]string{"-config", cfg, "-rundir", rundir, "-approve", planSHA}); err == nil {
		t.Fatal("execute must refuse a config that changed after plan")
	}
}

func TestRedactRemovesSecretValues(t *testing.T) {
	out := redact([]byte("a=short key=sneaky-secret-value-9876543210"), []string{"sneaky-secret-value-9876543210", "short"})
	if strings.Contains(string(out), "sneaky-secret-value-9876543210") || !strings.Contains(string(out), "<redacted>") {
		t.Fatalf("redaction failed: %s", out)
	}
	if !strings.Contains(string(out), "a=short") {
		t.Fatal("short values below the redaction threshold must be left alone")
	}
}
