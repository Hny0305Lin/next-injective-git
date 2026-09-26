package main

import (
	"encoding/json"
	"fmt"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/storageconfig"
	"io"
	"os"
)

// This dispatch runs before ordinary chain/IPFS config or signer loading.
// doctor is strictly local: no credential resolution and no cloud requests.
func cmdStorage(args []string) error { return storageCommand(args, os.Stdout) }
func storageCommand(args []string, out io.Writer) error {
	if len(args) != 2 || (args[0] != "doctor" && args[0] != "show") {
		return fmt.Errorf("usage: igit storage <doctor|show> <storage-config.json>")
	}
	c, err := storageconfig.Load(args[1])
	if err != nil {
		return err
	}
	if args[0] == "show" {
		return json.NewEncoder(out).Encode(c)
	}
	_, err = fmt.Fprintf(out, "PASS: local storage configuration (%d profiles, %d repository bindings). Credentials not resolved; cloud access, CORS and successor Git integration NOT PROVEN.\n", len(c.Profiles), len(c.Repositories))
	return err
}
