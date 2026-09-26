package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageDoctorBeforeOrdinaryConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("IGIT_CONFIG_DIR", filepath.Join(dir, "nonexistent-network-config"))
	t.Setenv("UNUSED_WRITER_ID", "SECRET-NOT-READ")
	p := filepath.Join(dir, "storage.json")
	data := `{"version":1,"profiles":{"writer":{"provider":"aws-s3","bucket":"test-bucket","region":"us-east-1","prefix":"project","credentialRef":{"kind":"env","accessKeyEnv":"UNUSED_WRITER_ID","secretKeyEnv":"UNSET_SECRET"}},"reader":{"provider":"cloudflare-r2","bucket":"read-bucket","region":"auto","accountId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","prefix":"project","publicReadBase":"https://read.example.com"}},"repositories":[]}`
	if e := os.WriteFile(p, []byte(data), 0600); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	for _, command := range []string{"doctor", "show"} {
		out.Reset()
		if e := storageCommand([]string{command, p}, &out); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(out.String(), "SECRET-NOT-READ") {
			t.Fatal("credential leak")
		}
	}
	if e := run([]string{"storage", "doctor", p}); e != nil {
		t.Fatal("loaded network config or credentials", e)
	}
}
