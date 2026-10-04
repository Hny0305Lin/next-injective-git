// Package successor provides the versioned client types, ABI bindings and a
// contract-faithful fake chain for the storage-neutral successor suite
// (contracts/evm-v2-successor). It never loads or modifies the legacy v3 ABI.
package successor

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
)

//go:embed abi/RepositoryCore.json
var repositoryCoreABIJSON string

//go:embed abi/SuiteDirectory.json
var suiteDirectoryABIJSON string

var (
	abisOnce sync.Once
	abis     map[string]gethabi.ABI
	abisErr  error
)

// CoreABI returns the parsed checked-in successor RepositoryCore ABI.
func CoreABI() (gethabi.ABI, error) { return load("core") }

// DirectoryABI returns the parsed checked-in successor SuiteDirectory ABI.
func DirectoryABI() (gethabi.ABI, error) { return load("directory") }

func load(name string) (gethabi.ABI, error) {
	abisOnce.Do(func() {
		abis = make(map[string]gethabi.ABI)
		for key, source := range map[string]string{"core": repositoryCoreABIJSON, "directory": suiteDirectoryABIJSON} {
			parsed, err := gethabi.JSON(strings.NewReader(source))
			if err != nil {
				abisErr = fmt.Errorf("parse embedded successor %s ABI: %w", key, err)
				return
			}
			abis[key] = parsed
		}
	})
	if abisErr != nil {
		return gethabi.ABI{}, abisErr
	}
	parsed, ok := abis[name]
	if !ok {
		return gethabi.ABI{}, fmt.Errorf("unknown successor ABI %q", name)
	}
	return parsed, nil
}
