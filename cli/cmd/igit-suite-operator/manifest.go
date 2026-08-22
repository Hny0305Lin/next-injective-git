package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest represents the calldata manifest structure
type Manifest struct {
	ChainID     uint64  `json:"chainId"`
	Directory   string  `json:"directory"`
	Coordinator string  `json:"coordinator"`
	Batches     []Batch `json:"batches"`
}

// Batch represents a single import batch
type Batch struct {
	Index    int    `json:"index"`
	CallData string `json:"callData"` // hex-encoded calldata
	GasLimit uint64 `json:"gasLimit,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// LoadManifest loads and validates a calldata manifest
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	// Validate manifest
	if manifest.ChainID == 0 {
		return nil, fmt.Errorf("invalid manifest: chainId is required")
	}
	if manifest.Directory == "" {
		return nil, fmt.Errorf("invalid manifest: directory is required")
	}
	if manifest.Coordinator == "" {
		return nil, fmt.Errorf("invalid manifest: coordinator is required")
	}
	if len(manifest.Batches) == 0 {
		return nil, fmt.Errorf("invalid manifest: no batches")
	}

	// Validate batch indices are sequential
	for i, batch := range manifest.Batches {
		if batch.Index != i {
			return nil, fmt.Errorf("invalid manifest: batch %d has wrong index (expected %d)", batch.Index, i)
		}
		if batch.CallData == "" {
			return nil, fmt.Errorf("invalid manifest: batch %d has no callData", i)
		}
	}

	return &manifest, nil
}
