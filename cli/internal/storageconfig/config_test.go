package storageconfig

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() Config {
	w := Profile{Provider: "aws-s3", Bucket: "test-bucket", Region: "us-east-1", Prefix: "project", CredentialRef: &CredentialRef{Kind: "env", AccessKeyEnv: "WRITER_ID", SecretKeyEnv: "WRITER_SECRET"}}
	r := w
	r.CredentialRef = &CredentialRef{Kind: "env", AccessKeyEnv: "READER_ID", SecretKeyEnv: "READER_SECRET"}
	return Config{Version: 1, Profiles: map[string]Profile{"writer": w, "reader": r}, Repositories: []Binding{{ChainID: "1776", SuiteDirectory: "0x" + strings.Repeat("1", 40), RepoID: "0x" + strings.Repeat("2", 64), Writer: "writer", Reader: "reader"}}}
}
func TestSeparateCredentialReferences(t *testing.T) {
	c := validConfig()
	if c.Validate() != nil {
		t.Fatal("valid")
	}
	calls := []string{}
	p := c.Profiles["reader"].CredentialRef.Resolve(func(n string) (string, bool) {
		calls = append(calls, n)
		if n == "READER_ID" {
			return "FAKE_READ_ID", true
		}
		if n == "READER_SECRET" {
			return "FAKE_READ_SECRET", true
		}
		t.Fatal("unexpected fallback", n)
		return "", false
	})
	creds, e := p.Retrieve(context.Background())
	if e != nil || creds.AccessKeyID != "FAKE_READ_ID" || len(calls) != 2 {
		t.Fatal(e, calls)
	}
	p = c.Profiles["reader"].CredentialRef.Resolve(func(string) (string, bool) { return "", false })
	if _, e = p.Retrieve(context.Background()); e == nil {
		t.Fatal("missing secret accepted")
	}
	c.Profiles["reader"] = c.Profiles["writer"]
	if c.Validate() == nil {
		t.Fatal("writer reused as reader")
	}
}
func TestLocalConfigRejectsSecretsAndEndpoints(t *testing.T) {
	c := validConfig()
	data, _ := json.Marshal(c)
	dir := t.TempDir()
	p := filepath.Join(dir, "storage.json")
	if e := os.WriteFile(p, data, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(p); e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{`"endpoint":"http://localhost:9000",`, `"secretAccessKey":"DO-NOT-LEAK",`, `"accessKeyId":"DO-NOT-LEAK",`, `"authorization":"DO-NOT-LEAK",`} {
		bad := strings.Replace(string(data), `"provider":`, field+`"provider":`, 1)
		os.WriteFile(p, []byte(bad), 0600)
		if _, e := Load(p); e == nil || strings.Contains(e.Error(), "DO-NOT-LEAK") {
			t.Fatal("unsafe config", e)
		}
	}
	for _, change := range []func(*Profile){func(p *Profile) { p.Provider = "minio" }, func(p *Profile) { p.Region = "http://evil.example.com" }, func(p *Profile) { p.PublicReadBase = "https://example.com/?secret=x" }, func(p *Profile) { p.Prefix = "../bucket" }, func(p *Profile) { p.Bucket = "127.0.0.1" }, func(p *Profile) { p.Provider = "cloudflare-r2"; p.Region = "auto"; p.AccountID = "not-an-account" }} {
		p := c.Profiles["writer"]
		change(&p)
		if p.Validate() == nil {
			t.Fatal("invalid provider accepted", p)
		}
	}
}
func TestRepoBindingsFailClosed(t *testing.T) {
	for _, change := range []func(*Config){func(c *Config) { c.Repositories[0].Reader = "missing" }, func(c *Config) { c.Repositories[0].Reader = "writer" }, func(c *Config) { c.Repositories[0].ChainID = "01776" }, func(c *Config) { c.Repositories = append(c.Repositories, c.Repositories[0]) }} {
		c := validConfig()
		change(&c)
		if c.Validate() == nil {
			t.Fatal("invalid binding")
		}
	}
}
