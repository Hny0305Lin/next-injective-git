package suitedeploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

const (
	artifactSchema       = "igit.evm-suite.solc-artifact.v1"
	requiredSolcVersion  = "0.8.24"
	maximumInitcodeBytes = 49_152
	maximumRuntimeBytes  = 24_576
)

var contractOrder = []string{
	"SuiteDirectory",
	"BootstrapCoordinator",
	"RepositoryCore",
	"RecoveryModule",
	"ModerationModule",
	"EconomicModule",
	"UsernameModule",
	"BadgeModule",
	"ReleaseModule",
}

type compilerSettings struct {
	Optimizer struct {
		Enabled bool   `json:"enabled"`
		Runs    uint64 `json:"runs"`
	} `json:"optimizer"`
	ViaIR      bool   `json:"via_ir"`
	EVMVersion string `json:"evm_version"`
}

type compilerMetadata struct {
	Version  string           `json:"version"`
	Settings compilerSettings `json:"settings"`
}

// ImmutableReference is a byte range in the deployed runtime template which
// solc replaces with one constructor-derived immutable value.
type ImmutableReference struct {
	Length uint64 `json:"length"`
	Start  uint64 `json:"start"`
}

type checkedArtifact struct {
	Schema                 string                          `json:"schema"`
	ContractName           string                          `json:"contract_name"`
	SourceName             string                          `json:"source_name"`
	SourceSHA256           string                          `json:"source_sha256"`
	Compiler               compilerMetadata                `json:"compiler"`
	ABI                    json.RawMessage                 `json:"abi"`
	CreationBytecode       string                          `json:"creation_bytecode"`
	CreationBytecodeSHA256 string                          `json:"creation_bytecode_sha256"`
	RuntimeTemplate        string                          `json:"runtime_template"`
	RuntimeTemplateSHA256  string                          `json:"runtime_template_sha256"`
	ImmutableReferences    map[string][]ImmutableReference `json:"immutable_references"`

	parsedABI *abi.ABI
	creation  []byte
	runtime   []byte
}

// ArtifactSummary is the immutable compiler/source evidence available before
// any chain transaction is signed.
type ArtifactSummary struct {
	ContractName           string `json:"contract_name"`
	SourceName             string `json:"source_name"`
	SourceSHA256           string `json:"source_sha256"`
	CreationBytecodeSHA256 string `json:"creation_bytecode_sha256"`
	RuntimeTemplateSHA256  string `json:"runtime_template_sha256"`
	CreationBytes          int    `json:"creation_bytes"`
	RuntimeBytes           int    `json:"runtime_bytes"`
}

// ArtifactInspection is a deterministic digest of the nine checked-in suite
// artifacts. It does not compile Solidity or contact an RPC endpoint.
type ArtifactInspection struct {
	Schema            string            `json:"schema"`
	Compiler          CompilerEvidence  `json:"compiler"`
	ArtifactSetSHA256 string            `json:"artifact_set_sha256"`
	Contracts         []ArtifactSummary `json:"contracts"`
}

type artifactSet struct {
	byName map[string]*checkedArtifact
	digest string
}

// InspectArtifacts validates the exact nine production artifact files and
// returns their stable hashes. No compiler output is accepted at runtime.
func InspectArtifacts(directory string) (*ArtifactInspection, error) {
	set, err := loadArtifactSet(directory)
	if err != nil {
		return nil, err
	}
	return set.inspection(), nil
}

func (set *artifactSet) inspection() *ArtifactInspection {
	inspection := &ArtifactInspection{
		Schema:            "igit.evm-suite.artifact-inspection.v1",
		Compiler:          requiredCompilerEvidence(),
		ArtifactSetSHA256: set.digest,
	}
	for _, name := range contractOrder {
		artifact := set.byName[name]
		inspection.Contracts = append(inspection.Contracts, ArtifactSummary{
			ContractName:           artifact.ContractName,
			SourceName:             artifact.SourceName,
			SourceSHA256:           artifact.SourceSHA256,
			CreationBytecodeSHA256: artifact.CreationBytecodeSHA256,
			RuntimeTemplateSHA256:  artifact.RuntimeTemplateSHA256,
			CreationBytes:          len(artifact.creation),
			RuntimeBytes:           len(artifact.runtime),
		})
	}
	return inspection
}

func loadArtifactSet(directory string) (*artifactSet, error) {
	directory = filepath.Clean(strings.TrimSpace(directory))
	if directory == "." || directory == "" {
		return nil, fmt.Errorf("suite artifact directory is required")
	}
	set := &artifactSet{byName: make(map[string]*checkedArtifact, len(contractOrder))}
	hasher := sha256.New()
	contractDirectory := filepath.Dir(directory)
	for _, name := range contractOrder {
		path := filepath.Join(directory, name+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read checked-in artifact %s: %w", path, err)
		}
		artifact, err := parseCheckedArtifact(name, data)
		if err != nil {
			return nil, fmt.Errorf("validate checked-in artifact %s: %w", path, err)
		}
		sourcePath := filepath.Join(contractDirectory, filepath.FromSlash(artifact.SourceName))
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read artifact-bound source %s: %w", sourcePath, err)
		}
		sourceDigest := sha256.Sum256(source)
		if hex.EncodeToString(sourceDigest[:]) != artifact.SourceSHA256 {
			return nil, fmt.Errorf("artifact source hash does not match %s", sourcePath)
		}
		set.byName[name] = artifact
		hasher.Write([]byte(name))
		hasher.Write([]byte{0})
		hasher.Write(data)
		hasher.Write([]byte{0})
	}
	set.digest = hex.EncodeToString(hasher.Sum(nil))
	return set, nil
}

func parseCheckedArtifact(expectedName string, data []byte) (*checkedArtifact, error) {
	var artifact checkedArtifact
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return nil, fmt.Errorf("decode artifact: %w", err)
	}
	if artifact.Schema != artifactSchema {
		return nil, fmt.Errorf("schema %q, want %q", artifact.Schema, artifactSchema)
	}
	if artifact.ContractName != expectedName {
		return nil, fmt.Errorf("contract_name %q, want %q", artifact.ContractName, expectedName)
	}
	if strings.TrimSpace(artifact.SourceName) == "" || filepath.IsAbs(artifact.SourceName) || strings.Contains(filepath.Clean(artifact.SourceName), "..") {
		return nil, fmt.Errorf("invalid source_name %q", artifact.SourceName)
	}
	if err := validateDigest("source_sha256", artifact.SourceSHA256); err != nil {
		return nil, err
	}
	if artifact.Compiler.Version != requiredSolcVersion || !artifact.Compiler.Settings.Optimizer.Enabled ||
		artifact.Compiler.Settings.Optimizer.Runs != 1 || !artifact.Compiler.Settings.ViaIR ||
		artifact.Compiler.Settings.EVMVersion != "default" {
		return nil, fmt.Errorf("compiler settings are not locked to solc 0.8.24, optimizer runs 1, viaIR, default EVM")
	}
	parsed, err := abi.JSON(bytes.NewReader(artifact.ABI))
	if err != nil {
		return nil, fmt.Errorf("parse ABI with go-ethereum: %w", err)
	}
	artifact.parsedABI = &parsed
	artifact.creation, err = decodeBytecode("creation_bytecode", artifact.CreationBytecode)
	if err != nil {
		return nil, err
	}
	artifact.runtime, err = decodeBytecode("runtime_template", artifact.RuntimeTemplate)
	if err != nil {
		return nil, err
	}
	if len(artifact.creation) == 0 || len(artifact.creation) > maximumInitcodeBytes {
		return nil, fmt.Errorf("creation bytecode length %d is outside 1..%d", len(artifact.creation), maximumInitcodeBytes)
	}
	if len(artifact.runtime) == 0 || len(artifact.runtime) > maximumRuntimeBytes {
		return nil, fmt.Errorf("runtime bytecode length %d is outside 1..%d", len(artifact.runtime), maximumRuntimeBytes)
	}
	if err := verifyBytecodeDigest("creation_bytecode_sha256", artifact.CreationBytecodeSHA256, artifact.creation); err != nil {
		return nil, err
	}
	if err := verifyBytecodeDigest("runtime_template_sha256", artifact.RuntimeTemplateSHA256, artifact.runtime); err != nil {
		return nil, err
	}
	if err := validateImmutableReferences(artifact.runtime, artifact.ImmutableReferences); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func decodeBytecode(field, value string) ([]byte, error) {
	if !strings.HasPrefix(value, "0x") || value != strings.ToLower(value) {
		return nil, fmt.Errorf("%s must be lower-case 0x-prefixed hex", field)
	}
	decoded, err := hex.DecodeString(value[2:])
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	return decoded, nil
}

func verifyBytecodeDigest(field, expected string, value []byte) error {
	if err := validateDigest(field, expected); err != nil {
		return err
	}
	actual := sha256.Sum256(value)
	if hex.EncodeToString(actual[:]) != expected {
		return fmt.Errorf("%s mismatch", field)
	}
	return nil
}

func validateDigest(field, value string) error {
	if len(value) != 64 || value != strings.ToLower(value) {
		return fmt.Errorf("%s must be 64 lower-case hex characters", field)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return fmt.Errorf("decode %s: %w", field, err)
	}
	allZero := true
	for _, b := range decoded {
		allZero = allZero && b == 0
	}
	if allZero {
		return fmt.Errorf("%s cannot be zero", field)
	}
	return nil
}

func validateImmutableReferences(runtime []byte, references map[string][]ImmutableReference) error {
	if len(references) == 0 {
		return fmt.Errorf("immutable_references is empty")
	}
	occupied := make([]bool, len(runtime))
	keys := make([]string, 0, len(references))
	for key := range references {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" || len(references[key]) == 0 {
			return fmt.Errorf("immutable reference group %q is empty", key)
		}
		for _, reference := range references[key] {
			if reference.Length != 32 || reference.Start > uint64(len(runtime)) || reference.Start+reference.Length > uint64(len(runtime)) {
				return fmt.Errorf("immutable reference %q [%d,%d) is invalid for %d-byte runtime", key, reference.Start, reference.Start+reference.Length, len(runtime))
			}
			for index := reference.Start; index < reference.Start+reference.Length; index++ {
				if occupied[index] {
					return fmt.Errorf("immutable reference %q overlaps another reference at byte %d", key, index)
				}
				occupied[index] = true
			}
		}
	}
	return nil
}
