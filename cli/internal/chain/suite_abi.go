package chain

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
)

//go:embed abi/SuiteDirectory.json
var suiteDirectoryABIJSON string

//go:embed abi/BootstrapCoordinator.json
var bootstrapCoordinatorABIJSON string

//go:embed abi/RepositoryCore.json
var repositoryCoreABIJSON string

//go:embed abi/RecoveryModule.json
var recoveryModuleABIJSON string

//go:embed abi/ModerationModule.json
var moderationModuleABIJSON string

//go:embed abi/EconomicModule.json
var economicModuleABIJSON string

//go:embed abi/UsernameModule.json
var usernameModuleABIJSON string

//go:embed abi/BadgeModule.json
var badgeModuleABIJSON string

//go:embed abi/ReleaseModule.json
var releaseModuleABIJSON string

type suiteABIName string

const (
	suiteABIDirectory   suiteABIName = "directory"
	suiteABICoordinator suiteABIName = "coordinator"
	suiteABICore        suiteABIName = "core"
	suiteABIRecovery    suiteABIName = "recovery"
	suiteABIModeration  suiteABIName = "moderation"
	suiteABIEconomic    suiteABIName = "economic"
	suiteABIUsername    suiteABIName = "username"
	suiteABIBadge       suiteABIName = "badge"
	suiteABIRelease     suiteABIName = "release"
)

var (
	suiteABIsOnce sync.Once
	suiteABIs     map[suiteABIName]gethabi.ABI
	suiteABIsErr  error
)

func loadSuiteABI(name suiteABIName) (gethabi.ABI, error) {
	suiteABIsOnce.Do(func() {
		suiteABIs = make(map[suiteABIName]gethabi.ABI)
		raw := map[suiteABIName]string{
			suiteABIDirectory: suiteDirectoryABIJSON, suiteABICoordinator: bootstrapCoordinatorABIJSON,
			suiteABICore:     repositoryCoreABIJSON,
			suiteABIRecovery: recoveryModuleABIJSON, suiteABIModeration: moderationModuleABIJSON,
			suiteABIEconomic: economicModuleABIJSON, suiteABIUsername: usernameModuleABIJSON,
			suiteABIBadge: badgeModuleABIJSON, suiteABIRelease: releaseModuleABIJSON,
		}
		for key, source := range raw {
			parsed, err := gethabi.JSON(strings.NewReader(source))
			if err != nil {
				suiteABIsErr = fmt.Errorf("parse embedded %s ABI: %w", key, err)
				return
			}
			suiteABIs[key] = parsed
		}
	})
	if suiteABIsErr != nil {
		return gethabi.ABI{}, suiteABIsErr
	}
	parsed, ok := suiteABIs[name]
	if !ok {
		return gethabi.ABI{}, fmt.Errorf("unknown suite ABI %q", name)
	}
	return parsed, nil
}
