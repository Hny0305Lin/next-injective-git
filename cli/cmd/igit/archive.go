package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/archivev1"
)

const archiveUsage = `usage:
  igit archive query --lcd URL --contract inj1... --height N '<smart-query-json>'
  igit archive inventory --tx-search FILE --block-evidence FILE --chain-id ID --contract inj1... --height N --output FILE
  igit archive verify --snapshot FILE --inventory FILE --tx-search FILE --block-evidence FILE

Archive commands are read-only. They never load a signer or broadcast a V1 transaction.
`

func cmdArchive(args []string) error {
	if len(args) == 0 {
		return errors.New(archiveUsage)
	}
	switch args[0] {
	case "query":
		return cmdArchiveQuery(args[1:])
	case "inventory":
		return cmdArchiveInventory(args[1:])
	case "verify":
		return cmdArchiveVerify(args[1:])
	case "help", "-h", "--help":
		fmt.Print(archiveUsage)
		return nil
	default:
		return fmt.Errorf("unknown archive command %q\n%s", args[0], archiveUsage)
	}
}

func cmdArchiveQuery(args []string) error {
	flags := flag.NewFlagSet("igit archive query", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	lcd := flags.String("lcd", "", "Cosmos LCD endpoint")
	contract := flags.String("contract", "", "CosmWasm V1 contract")
	height := flags.Uint64("height", 0, "fixed archive height")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, archiveUsage)
	}
	if flags.NArg() != 1 {
		return errors.New(archiveUsage)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := (archivev1.Client{LCD: *lcd, Contract: *contract, Height: *height}).SmartQuery(
		ctx, json.RawMessage(flags.Arg(0)),
	)
	if err != nil {
		return err
	}
	var rendered bytes.Buffer
	if err := json.Indent(&rendered, data, "", "  "); err != nil {
		return fmt.Errorf("format archived V1 response: %w", err)
	}
	fmt.Println(rendered.String())
	return nil
}

func cmdArchiveInventory(args []string) error {
	flags := flag.NewFlagSet("igit archive inventory", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	txSearch := flags.String("tx-search", "", "saved CometBFT tx_search response")
	blockEvidencePath := flags.String("block-evidence", "", "saved CometBFT block responses")
	chainID := flags.String("chain-id", "", "source chain ID")
	contract := flags.String("contract", "", "CosmWasm V1 contract")
	height := flags.Uint64("height", 0, "cutover height")
	output := flags.String("output", "v1-inventory.json", "inventory output")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		if err == nil {
			err = errors.New("unexpected positional arguments")
		}
		return fmt.Errorf("%w\n%s", err, archiveUsage)
	}
	if strings.TrimSpace(*txSearch) == "" {
		return errors.New("--tx-search is required")
	}
	raw, err := os.ReadFile(*txSearch)
	if err != nil {
		return fmt.Errorf("read tx_search evidence: %w", err)
	}
	blockEvidence, err := archivev1.ReadBlockEvidence(*blockEvidencePath)
	if err != nil {
		return fmt.Errorf("read block evidence: %w", err)
	}
	inventory, err := archivev1.BuildInventory(raw, *chainID, *contract, *height, blockEvidence)
	if err != nil {
		return err
	}
	rendered, err := archivev1.MarshalInventory(inventory)
	if err != nil {
		return err
	}
	if err := writeArchiveEvidence(*output, rendered); err != nil {
		return err
	}
	fmt.Printf("inventory: %s\ncutover height: %d\ncutover block: %s\ntx_search sha256: %s\nblock evidence sha256: %s\nevent commitment: %s\nread-only: true\n", *output, inventory.CutoverHeight, inventory.CutoverBlockHash, inventory.SourceSHA256, inventory.BlockSHA256, inventory.EventCommitment)
	return nil
}

func cmdArchiveVerify(args []string) error {
	flags := flag.NewFlagSet("igit archive verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	snapshot := flags.String("snapshot", "", "V1 snapshot")
	inventoryPath := flags.String("inventory", "", "V1 inventory")
	txSearchPath := flags.String("tx-search", "", "saved CometBFT tx_search response")
	blockEvidencePath := flags.String("block-evidence", "", "saved CometBFT block responses")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		if err == nil {
			err = errors.New("unexpected positional arguments")
		}
		return fmt.Errorf("%w\n%s", err, archiveUsage)
	}
	inventory, err := archivev1.ReadInventory(*inventoryPath)
	if err != nil {
		return err
	}
	txSearch, err := os.ReadFile(*txSearchPath)
	if err != nil {
		return fmt.Errorf("read tx_search evidence: %w", err)
	}
	blockEvidence, err := archivev1.ReadBlockEvidence(*blockEvidencePath)
	if err != nil {
		return fmt.Errorf("read block evidence: %w", err)
	}
	if err := archivev1.VerifyInventoryEvidence(inventory, txSearch, blockEvidence); err != nil {
		return err
	}
	if err := archivev1.VerifySnapshotInventory(*snapshot, inventory); err != nil {
		return err
	}
	fmt.Printf("snapshot inventory verification: pass\nheight: %s\nblock hash: %s\nsource replay: pass\nread-only: true\n", strconv.FormatUint(inventory.CutoverHeight, 10), inventory.CutoverBlockHash)
	return nil
}

func writeArchiveEvidence(path string, raw []byte) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return errors.New("archive evidence output path is required")
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("archive evidence %s already exists and is never overwritten", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".igit-archive-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return fmt.Errorf("publish archive evidence without overwrite: %w", err)
	}
	return nil
}
