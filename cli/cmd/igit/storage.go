package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/i18n"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/storageconfig"
)

// This dispatch runs before ordinary chain/IPFS config or signer loading.
// doctor is strictly local: no credential resolution and no cloud requests.
func cmdStorage(args []string) error { return storageCommand(args, os.Stdout) }
func storageCommand(args []string, out io.Writer) error {
	if len(args) >= 2 && args[0] == "add" {
		return storageAdd(args[1], out)
	}
	if len(args) != 2 || (args[0] != "doctor" && args[0] != "show") {
		return fmt.Errorf("usage: igit storage <add|doctor|show> <storage-config.json>")
	}
	c, err := storageconfig.Load(args[1])
	if err != nil {
		return err
	}
	if args[0] == "show" {
		return json.NewEncoder(out).Encode(c)
	}
	_, err = fmt.Fprintf(out, "PASS: local storage configuration (%d profiles, %d repository bindings). Credentials not resolved; cloud access, CORS and Git integration tested separately.\n", len(c.Profiles), len(c.Repositories))
	return err
}

// storageAdd validates an explicit storage profile file and records only its
// absolute path in the igit config. Secrets stay out of both files.
func storageAdd(path string, out io.Writer) error {
	if _, err := storageconfig.Load(path); err != nil {
		return i18n.Errorf("invalid storage configuration: %w", "storage 配置无效：%w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.StorageConfig = absolute
	if err := config.Save(cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", i18n.Text(
		"storage profile registered (path only; credentials are referenced, never stored)",
		"storage 配置已登记（仅保存路径；凭据只引用，不落值）"))
	return err
}
