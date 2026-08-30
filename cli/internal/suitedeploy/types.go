package suitedeploy

import (
	"encoding/json"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
)

const manifestSchema = "igit.evm-suite.deployment.v1"

const (
	evidenceModeLiveBroadcast      = "live-broadcast"
	evidenceModeHistoricalRecovery = "historical-recovery"
)

type CompilerEvidence struct {
	Version   string `json:"version"`
	Optimizer struct {
		Enabled bool   `json:"enabled"`
		Runs    uint64 `json:"runs"`
	} `json:"optimizer"`
	ViaIR      bool   `json:"via_ir"`
	EVMVersion string `json:"evm_version"`
}

func requiredCompilerEvidence() CompilerEvidence {
	value := CompilerEvidence{Version: requiredSolcVersion, ViaIR: true, EVMVersion: "default"}
	value.Optimizer.Enabled = true
	value.Optimizer.Runs = 1
	return value
}

type SourceEvidence struct {
	Commit            string            `json:"commit"`
	ArtifactSetSHA256 string            `json:"artifact_set_sha256"`
	Contracts         []ArtifactSummary `json:"contracts"`
}

type ChainEvidence struct {
	Network       string `json:"network"`
	ChainID       uint64 `json:"chain_id"`
	RPCEndpoint   string `json:"rpc_endpoint"`
	BlockExplorer string `json:"block_explorer,omitempty"`
}

type GovernanceEvidence struct {
	BootstrapOperator string `json:"bootstrap_operator"`
	Admin             string `json:"admin"`
	Treasury          string `json:"treasury"`
	Committee         string `json:"committee"`
	UsernamePolicy    string `json:"username_policy_admin"`
	ReleaseAuthority  string `json:"release_authority"`
	ProductionReady   bool   `json:"production_ready"`
	Warning           string `json:"warning"`
}

type ConstructorArgument struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type RuntimeEvidence struct {
	BlockNumber           string            `json:"block_number"`
	BlockHash             string            `json:"block_hash"`
	ByteLength            int               `json:"byte_length"`
	SHA256                string            `json:"sha256"`
	CodeHashKeccak256     string            `json:"code_hash_keccak256"`
	TemplateSHA256        string            `json:"template_sha256"`
	ImmutableValuesByID   map[string]string `json:"immutable_values_by_solc_id"`
	TemplateMatchVerified bool              `json:"template_match_verified"`
}

type ContractEvidence struct {
	TransactionOrder       uint64                `json:"transaction_order"`
	ContractName           string                `json:"contract_name"`
	ModuleID               string                `json:"module_id,omitempty"`
	SourceName             string                `json:"source_name"`
	SourceSHA256           string                `json:"source_sha256"`
	CreationBytecodeSHA256 string                `json:"creation_bytecode_sha256"`
	RuntimeTemplateSHA256  string                `json:"runtime_template_sha256"`
	ConstructorArguments   []ConstructorArgument `json:"constructor_arguments"`
	ConstructorArgsABI     string                `json:"constructor_args_abi"`
	InitcodeSHA256         string                `json:"initcode_sha256"`
	Address                string                `json:"address,omitempty"`
	TransactionHash        string                `json:"transaction_hash,omitempty"`
	Receipt                *chain.EVMReceipt     `json:"receipt,omitempty"`
	Runtime                *RuntimeEvidence      `json:"runtime,omitempty"`
}

type ConfigurationTransaction struct {
	TransactionOrder uint64            `json:"transaction_order"`
	Purpose          string            `json:"purpose"`
	ModuleID         string            `json:"module_id,omitempty"`
	Target           string            `json:"target"`
	Calldata         string            `json:"calldata"`
	CalldataSHA256   string            `json:"calldata_sha256"`
	TransactionHash  string            `json:"transaction_hash,omitempty"`
	Receipt          *chain.EVMReceipt `json:"receipt,omitempty"`
}

type ModuleBindingEvidence struct {
	ModuleID           string `json:"module_id"`
	ContractName       string `json:"contract_name"`
	Address            string `json:"address"`
	DirectoryCodeHash  string `json:"directory_code_hash"`
	ObservedCodeHash   string `json:"observed_code_hash"`
	DirectoryVerified  bool   `json:"directory_verified"`
	ModuleDirectory    string `json:"module_directory"`
	ModuleCoordinator  string `json:"module_coordinator"`
	ObservedModuleID   string `json:"observed_module_id"`
	BootstrapFinalized bool   `json:"bootstrap_finalized"`
	AuthorityRole      string `json:"authority_role,omitempty"`
	AuthorityAddress   string `json:"authority_address,omitempty"`
}

type InitialGovernanceBinding struct {
	ModerationAdmin        string `json:"moderation_admin"`
	ModerationCommittee    string `json:"moderation_committee"`
	EconomicAdmin          string `json:"economic_admin"`
	EconomicTreasury       string `json:"economic_treasury"`
	EconomicPlatformFeeBPS uint16 `json:"economic_platform_fee_bps"`
	UsernamePolicyAdmin    string `json:"username_policy_admin"`
	ReleaseAuthority       string `json:"release_authority"`
}

type DirectoryBindingEvidence struct {
	BlockNumber             string                   `json:"block_number"`
	BlockHash               string                   `json:"block_hash"`
	SuiteVersion            uint64                   `json:"suite_version"`
	State                   uint8                    `json:"state"`
	Active                  bool                     `json:"active"`
	ConfiguredChainID       uint64                   `json:"configured_chain_id"`
	SnapshotRoot            string                   `json:"snapshot_root"`
	BootstrapAuthority      string                   `json:"bootstrap_authority"`
	BootstrapCoordinator    string                   `json:"bootstrap_coordinator"`
	CoordinatorCodeHash     string                   `json:"coordinator_code_hash"`
	RegisteredModuleCount   uint64                   `json:"registered_module_count"`
	CoordinatorDirectory    string                   `json:"coordinator_directory"`
	CoordinatorSnapshotRoot string                   `json:"coordinator_snapshot_root"`
	CoordinatorOperator     string                   `json:"coordinator_operator"`
	CoordinatorActivated    bool                     `json:"coordinator_activated"`
	CoordinatorNextModule   uint64                   `json:"coordinator_next_module_index"`
	Modules                 []ModuleBindingEvidence  `json:"modules"`
	InitialGovernance       InitialGovernanceBinding `json:"initial_governance"`
}

type BlockscoutVerification struct {
	Status    string            `json:"status"`
	Explorer  string            `json:"explorer,omitempty"`
	Contracts map[string]string `json:"contracts"`
	Note      string            `json:"note"`
}

type RecoveryEvidence struct {
	Source                string `json:"source"`
	RecoveredAt           string `json:"recovered_at"`
	TransactionsValidated uint64 `json:"transactions_validated"`
}

// Manifest is append-progress deployment evidence. Status remains
// "bootstrapping" after successful deployment because import and activation
// are deliberately separate cutover operations.
type Manifest struct {
	Schema                       string                      `json:"schema"`
	Status                       string                      `json:"status"`
	EvidenceMode                 string                      `json:"evidence_mode"`
	CreatedAt                    string                      `json:"created_at"`
	UpdatedAt                    string                      `json:"updated_at"`
	FailureStage                 string                      `json:"failure_stage,omitempty"`
	Failure                      string                      `json:"failure,omitempty"`
	Compiler                     CompilerEvidence            `json:"compiler"`
	Source                       SourceEvidence              `json:"source"`
	Chain                        ChainEvidence               `json:"chain"`
	SnapshotRoot                 string                      `json:"snapshot_root"`
	PlatformFeeBPS               uint16                      `json:"platform_fee_bps"`
	Governance                   GovernanceEvidence          `json:"governance"`
	Contracts                    []*ContractEvidence         `json:"contracts"`
	ConfigurationTransactions    []*ConfigurationTransaction `json:"configuration_transactions"`
	DirectoryBindingVerification *DirectoryBindingEvidence   `json:"directory_binding_verification,omitempty"`
	BlockscoutVerification       BlockscoutVerification      `json:"blockscout_verification"`
	Recovery                     *RecoveryEvidence           `json:"recovery,omitempty"`
}

type Options struct {
	ArtifactDirectory string
	SourceCommit      string
	Network           string
	ChainID           uint64
	RPCEndpoint       string
	BlockExplorer     string
	SnapshotRoot      string
	Operator          string
	PlatformFeeBPS    uint16
	Clock             func() time.Time
}

// MarshalCanonical renders a stable indented representation suitable for the
// no-clobber evidence file. Struct fields and deployment order are fixed.
func (m *Manifest) MarshalCanonical() ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
