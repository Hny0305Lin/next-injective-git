package chain

// RefInfo is the transport-neutral repository ref returned by the EVM suite.
type RefInfo struct {
	RefName   string   `json:"ref_name"`
	CommitSha string   `json:"commit_sha"`
	PackURIs  []string `json:"pack_uris"`
	UpdatedAt uint64   `json:"updated_at"`
	UpdatedBy string   `json:"updated_by"`
}

// RepoInfo is the transport-neutral repository metadata returned by Core.
type RepoInfo struct {
	Owner            string `json:"owner"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	DefaultBranch    string `json:"default_branch"`
	CreatedAt        uint64 `json:"created_at"`
	UpdatedAt        uint64 `json:"updated_at"`
	ModerationStatus string `json:"moderation_status"`
}

type CollaboratorInfo struct {
	Address string `json:"address"`
	Role    string `json:"role"`
}

type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type SplitEntry struct {
	Address string `json:"address"`
	Bps     uint16 `json:"bps"`
}

type ReleaseArtifact struct {
	Version      string `json:"version"`
	Platform     string `json:"platform"`
	SHA256       string `json:"sha256"`
	RegisteredBy string `json:"registered_by"`
	RegisteredAt uint64 `json:"registered_at"`
}

type OwnershipTransferInfo struct {
	NewOwner     string `json:"new_owner"`
	ProposedAt   uint64 `json:"proposed_at"`
	ExecuteAfter uint64 `json:"execute_after"`
	ExpiresAt    uint64 `json:"expires_at,omitempty"`
}

type RecoveryProposalInfo struct {
	NewOwner     string   `json:"new_owner"`
	ProposedAt   uint64   `json:"proposed_at"`
	ExecuteAfter uint64   `json:"execute_after"`
	Approvals    []string `json:"approvals"`
}

type OwnershipSecurityInfo struct {
	Transfer          *OwnershipTransferInfo `json:"transfer"`
	Recovery          *RecoveryProposalInfo  `json:"recovery"`
	Guardians         []string               `json:"guardians"`
	GuardianThreshold uint8                  `json:"guardian_threshold"`
}

type ModerationReportInfo struct {
	ID             uint64  `json:"id"`
	Owner          string  `json:"owner"`
	Repo           string  `json:"repo"`
	Reporter       string  `json:"reporter"`
	ReasonHash     string  `json:"reason_hash"`
	Status         string  `json:"status"`
	Resolution     string  `json:"resolution"`
	ResolutionHash *string `json:"resolution_hash"`
	AppealHash     *string `json:"appeal_hash"`
	CreatedAt      uint64  `json:"created_at"`
	UpdatedAt      uint64  `json:"updated_at"`
}

type Badge struct {
	ID        uint64 `json:"id"`
	RepoID    string `json:"repo_id,omitempty"`
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
	Recipient string `json:"recipient"`
	Reason    string `json:"reason"`
	AwardedBy string `json:"awarded_by,omitempty"`
	AwardedAt uint64 `json:"awarded_at"`
}
