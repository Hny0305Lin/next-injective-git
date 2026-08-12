package suitemigration

const (
	SnapshotSchema = "igit.cosmwasm-v1.suite-snapshot.v1"
	PlanSchema     = "igit.evm-suite.bootstrap-plan.v1"
	ManifestSchema = "igit.evm-suite.bootstrap-calldata-manifest.v1"

	SuiteVersion        = uint64(3)
	DefaultBatchSize    = 64
	MaximumBatchItems   = 128
	MaximumPayloadBytes = 96_000
	RequiredModuleCount = 7
)

type Snapshot struct {
	Schema          string                   `json:"schema"`
	Source          SnapshotSource           `json:"source"`
	ContractPolicy  ContractPolicy           `json:"contract_policy"`
	Repositories    []SnapshotRepository     `json:"repositories"`
	Aliases         []SnapshotAlias          `json:"aliases"`
	Refs            []SnapshotRef            `json:"refs"`
	Collaborators   []SnapshotCollaborator   `json:"collaborators"`
	GuardianConfigs []SnapshotGuardianConfig `json:"guardian_configs"`
	Moderation      SnapshotModeration       `json:"moderation"`
	Economic        SnapshotEconomic         `json:"economic"`
	Username        SnapshotUsername         `json:"username"`
	Badges          []SnapshotBadge          `json:"badges"`
	BadgeIndexes    SnapshotBadgeIndexes     `json:"badge_indexes"`
	Releases        []SnapshotRelease        `json:"releases"`
	ReleaseVersions []SnapshotReleaseVersion `json:"release_versions"`
}

type SnapshotSource struct {
	ChainID               string `json:"chain_id"`
	Contract              string `json:"contract"`
	Height                uint64 `json:"height"`
	BlockHash             string `json:"block_hash"`
	InventorySHA256       string `json:"inventory_sha256"`
	TxSearchSHA256        string `json:"tx_search_sha256"`
	BlockEvidenceSHA256   string `json:"block_evidence_sha256"`
	EventCommitmentSHA256 string `json:"event_commitment_sha256"`
}

// ContractPolicy is retained in the immutable plan so deployment evidence can
// prove that constructor policy matches the cutover snapshot.
type ContractPolicy struct {
	Admin               string `json:"admin"`
	Treasury            string `json:"treasury"`
	Committee           string `json:"committee"`
	ReleaseAuthority    string `json:"release_authority"`
	UsernamePolicyAdmin string `json:"username_policy_admin"`
	PlatformFeeBPS      uint16 `json:"platform_fee_bps"`
}

type SnapshotRepository struct {
	ID            string `json:"id"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	ForkedFrom    string `json:"forked_from,omitempty"`
	CreatedAt     uint64 `json:"created_at"`
	UpdatedAt     uint64 `json:"updated_at"`
}

type SnapshotAlias struct {
	RepoID string `json:"repo_id"`
	Owner  string `json:"owner"`
	Name   string `json:"name"`
}

type SnapshotRef struct {
	RepoID    string   `json:"repo_id"`
	RefName   string   `json:"ref_name"`
	CommitSHA string   `json:"commit_sha"`
	PackURIs  []string `json:"pack_uris"`
	UpdatedAt uint64   `json:"updated_at"`
	UpdatedBy string   `json:"updated_by"`
}

type SnapshotCollaborator struct {
	RepoID  string `json:"repo_id"`
	Account string `json:"account"`
	Role    string `json:"role"`
}

type SnapshotGuardianConfig struct {
	RepoID       string   `json:"repo_id"`
	ConfiguredBy string   `json:"configured_by"`
	Threshold    uint8    `json:"threshold"`
	Guardians    []string `json:"guardians"`
}

type SnapshotModeration struct {
	FinalStatuses []SnapshotFinalStatus `json:"final_statuses"`
	Reports       []SnapshotReport      `json:"reports"`
	StatusTrails  []SnapshotStatusTrail `json:"status_trails"`
}

type SnapshotFinalStatus struct {
	RepoID string `json:"repo_id"`
	Status string `json:"status"`
}

type SnapshotReport struct {
	ID         uint64                `json:"id"`
	RepoID     string                `json:"repo_id"`
	Reporter   string                `json:"reporter"`
	Status     string                `json:"status"`
	Resolution string                `json:"resolution"`
	ReasonHash string                `json:"reason_hash"`
	CreatedAt  uint64                `json:"created_at"`
	UpdatedAt  uint64                `json:"updated_at"`
	Trail      []SnapshotReportTrail `json:"trail"`
}

type SnapshotReportTrail struct {
	Action     string `json:"action"`
	Actor      string `json:"actor"`
	Status     string `json:"status"`
	ReasonHash string `json:"reason_hash"`
	Timestamp  uint64 `json:"timestamp"`
}

type SnapshotStatusTrail struct {
	RepoID string                        `json:"repo_id"`
	Trail  []SnapshotRepositoryStatusLog `json:"trail"`
}

type SnapshotRepositoryStatusLog struct {
	Status     string `json:"status"`
	Actor      string `json:"actor"`
	ReasonHash string `json:"reason_hash"`
	Timestamp  uint64 `json:"timestamp"`
	ReportID   uint64 `json:"report_id,omitempty"`
}

type SnapshotEconomic struct {
	Splits []SnapshotRevenueSplits `json:"splits"`
	Totals []SnapshotSponsorTotal  `json:"totals"`
}

type SnapshotRevenueSplits struct {
	RepoID string          `json:"repo_id"`
	Splits []SnapshotSplit `json:"splits"`
}

type SnapshotSplit struct {
	Recipient string `json:"recipient"`
	BPS       uint16 `json:"bps"`
}

type SnapshotSponsorTotal struct {
	RepoID string `json:"repo_id"`
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type SnapshotUsername struct {
	OriginalOwners []SnapshotOriginalUsername `json:"original_owners"`
	ReservedNames  []string                   `json:"reserved_names"`
	EscrowRelease  UsernameEscrowRelease      `json:"escrow_release"`
}

type SnapshotOriginalUsername struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

type UsernameEscrowRelease struct {
	AllReleased    bool   `json:"all_released"`
	Contract       string `json:"contract"`
	Height         uint64 `json:"height"`
	BlockHash      string `json:"block_hash"`
	Denom          string `json:"denom"`
	EscrowBalance  string `json:"escrow_balance"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type SnapshotBadge struct {
	ID        uint64 `json:"id"`
	RepoID    string `json:"repo_id"`
	Recipient string `json:"recipient"`
	AwardedBy string `json:"awarded_by"`
	Reason    string `json:"reason"`
	AwardedAt uint64 `json:"awarded_at"`
}

type SnapshotBadgeIndexes struct {
	ByRecipient  []SnapshotBadgeRecipientIndex  `json:"by_recipient"`
	ByRepository []SnapshotBadgeRepositoryIndex `json:"by_repository"`
}

type SnapshotBadgeRecipientIndex struct {
	Recipient string   `json:"recipient"`
	BadgeIDs  []uint64 `json:"badge_ids"`
}

type SnapshotBadgeRepositoryIndex struct {
	RepoID   string   `json:"repo_id"`
	BadgeIDs []uint64 `json:"badge_ids"`
}

type SnapshotRelease struct {
	Version      string `json:"version"`
	Platform     string `json:"platform"`
	SHA256       string `json:"sha256"`
	RegisteredBy string `json:"registered_by"`
	RegisteredAt uint64 `json:"registered_at"`
}

type SnapshotReleaseVersion struct {
	Version   string   `json:"version"`
	Platforms []string `json:"platforms"`
}

type BuildOptions struct {
	TargetChainID uint64
	Directory     string
	Coordinator   string
	BatchSize     int
}

type Plan struct {
	Schema                     string                `json:"schema"`
	Executable                 bool                  `json:"executable"`
	Snapshot                   PlanSnapshot          `json:"snapshot"`
	Target                     PlanTarget            `json:"target"`
	ContractPolicy             ContractPolicy        `json:"contract_policy"`
	UsernameEscrowRelease      UsernameEscrowRelease `json:"username_escrow_release"`
	UsernameEscrowEvidenceHash string                `json:"username_escrow_evidence_hash"`
	Limits                     PlanLimits            `json:"limits"`
	Modules                    []ModulePlan          `json:"modules"`
	Summary                    PlanSummary           `json:"summary"`
}

type PlanSnapshot struct {
	Schema                string `json:"schema"`
	SHA256                string `json:"sha256"`
	Root                  string `json:"root"`
	SourceChainID         string `json:"source_chain_id"`
	SourceContract        string `json:"source_contract"`
	SourceHeight          uint64 `json:"source_height"`
	SourceBlockHash       string `json:"source_block_hash"`
	InventorySHA256       string `json:"inventory_sha256"`
	TxSearchSHA256        string `json:"tx_search_sha256"`
	BlockEvidenceSHA256   string `json:"block_evidence_sha256"`
	EventCommitmentSHA256 string `json:"event_commitment_sha256"`
}

type PlanTarget struct {
	ChainID      uint64 `json:"chain_id"`
	SuiteVersion uint64 `json:"suite_version"`
	Directory    string `json:"directory"`
	Coordinator  string `json:"coordinator"`
}

type PlanLimits struct {
	BatchSize       int `json:"batch_size"`
	MaxBatchItems   int `json:"max_batch_items"`
	MaxPayloadBytes int `json:"max_payload_bytes"`
}

type ModulePlan struct {
	Order           int         `json:"order"`
	Name            string      `json:"name"`
	ID              string      `json:"id"`
	ExpectedCount   uint64      `json:"expected_count"`
	ExpectedBatches uint64      `json:"expected_batches"`
	ExpectedRoot    string      `json:"expected_root"`
	Batches         []PlanBatch `json:"batches"`
}

type PlanBatch struct {
	Sequence    uint64   `json:"sequence"`
	Kind        string   `json:"kind"`
	KindCode    uint8    `json:"kind_code"`
	Count       uint64   `json:"count"`
	ItemKeys    []string `json:"item_keys"`
	Payload     string   `json:"payload"`
	PayloadHash string   `json:"payload_hash"`
	RollingRoot string   `json:"rolling_root"`
}

type PlanSummary struct {
	RepositoryCount       int `json:"repository_count"`
	AliasCount            int `json:"alias_count"`
	RefCount              int `json:"ref_count"`
	CollaboratorCount     int `json:"collaborator_count"`
	GuardianConfigCount   int `json:"guardian_config_count"`
	ModerationStatusCount int `json:"moderation_status_count"`
	ModerationReportCount int `json:"moderation_report_count"`
	ModerationTrailCount  int `json:"moderation_status_trail_count"`
	EconomicSplitCount    int `json:"economic_split_count"`
	EconomicTotalCount    int `json:"economic_total_count"`
	UsernameOwnerCount    int `json:"username_owner_count"`
	ReservedNameCount     int `json:"reserved_name_count"`
	BadgeCount            int `json:"badge_count"`
	ReleaseArtifactCount  int `json:"release_artifact_count"`
	BatchCount            int `json:"batch_count"`
	TransactionCount      int `json:"transaction_count"`
}

type CalldataManifest struct {
	Schema        string                `json:"schema"`
	CalldataReady bool                  `json:"calldata_ready"`
	Signed        bool                  `json:"signed"`
	Broadcast     bool                  `json:"broadcast"`
	PlanSHA256    string                `json:"plan_sha256"`
	SnapshotRoot  string                `json:"snapshot_root"`
	Target        PlanTarget            `json:"target"`
	Transactions  []ManifestTransaction `json:"transactions"`
}

type ManifestTransaction struct {
	Order         int     `json:"order"`
	ModuleOrder   *int    `json:"module_order,omitempty"`
	Module        string  `json:"module,omitempty"`
	ModuleID      string  `json:"module_id,omitempty"`
	Sequence      *uint64 `json:"sequence,omitempty"`
	Phase         string  `json:"phase"`
	Function      string  `json:"function"`
	ExpectedEvent string  `json:"expected_event"`
	ExpectedTopic string  `json:"expected_event_topic"`
	To            string  `json:"to"`
	Value         string  `json:"value"`
	Data          string  `json:"data"`
}
