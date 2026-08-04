// Package chain talks to the Injective repo-registry contract:
// smart queries go through the LCD REST API; transactions are signed and
// broadcast by shelling out to injectived (keys never leave the local keyring).
package chain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// Client wraps query + tx access to the repo-registry contract.
type Client struct {
	cfg  config.Config
	http *http.Client
}

// New creates a chain client from CLI config.
func New(cfg config.Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}}
}

// ---- contract message types (mirror msg.rs) ----

// RefInfo mirrors the contract's RefInfo response item.
type RefInfo struct {
	RefName   string   `json:"ref_name"`
	CommitSha string   `json:"commit_sha"`
	PackURIs  []string `json:"pack_uris"`
	UpdatedAt uint64   `json:"updated_at"`
	UpdatedBy string   `json:"updated_by"`
}

// RepoInfo mirrors the contract's RepoInfoResponse.
type RepoInfo struct {
	Owner            string `json:"owner"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	DefaultBranch    string `json:"default_branch"`
	CreatedAt        uint64 `json:"created_at"`
	UpdatedAt        uint64 `json:"updated_at"`
	ModerationStatus string `json:"moderation_status"`
}

type listRefsResponse struct {
	Refs []RefInfo `json:"refs"`
}

type resolveRefResponse struct {
	RefName   string   `json:"ref_name"`
	CommitSha string   `json:"commit_sha"`
	PackURIs  []string `json:"pack_uris"`
}

// CollaboratorInfo mirrors the contract's collaborator list item.
type CollaboratorInfo struct {
	Address string `json:"address"`
	Role    string `json:"role"`
}

type listCollaboratorsResponse struct {
	Collaborators []CollaboratorInfo `json:"collaborators"`
}

// Coin mirrors the cosmos coin JSON shape.
type Coin struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

// ConfigInfo mirrors the contract's ConfigResponse.
type ConfigInfo struct {
	Admin               string   `json:"admin"`
	ModerationCommittee string   `json:"moderation_committee"`
	Treasury            string   `json:"treasury"`
	PlatformFeeBps      uint16   `json:"platform_fee_bps"`
	UsernameDeposit     Coin     `json:"username_deposit"`
	UsernameFee         Coin     `json:"username_fee"`
	ReservedUsernames   []string `json:"reserved_usernames"`
}

// SplitEntry mirrors the contract's revenue split entry.
type SplitEntry struct {
	Address string `json:"address"`
	Bps     uint16 `json:"bps"`
}

type revenueSplitsResponse struct {
	Splits []SplitEntry `json:"splits"`
}

type usernameResponse struct {
	Owner string `json:"owner"`
}

type addressUsernameResponse struct {
	Name *string `json:"name"`
}

// ReleaseArtifact mirrors an immutable on-chain release checksum record.
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

// UpgradeProposalInfo mirrors the contract's delayed upgrade announcement.
type UpgradeProposalInfo struct {
	WasmSHA256   string `json:"wasm_sha256"`
	ProposedAt   uint64 `json:"proposed_at"`
	ExecuteAfter uint64 `json:"execute_after"`
}

type UpgradeSecurityInfo struct {
	Proposal        *UpgradeProposalInfo `json:"proposal"`
	TimelockSeconds uint64               `json:"timelock_seconds"`
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

// ---- smart queries via LCD ----

type lcdSmartResponse struct {
	Data json.RawMessage `json:"data"`
}

// SmartQuery runs a wasm smart query against the contract and decodes the
// response into out.
func (c *Client) SmartQuery(queryMsg any, out any) error {
	raw, err := json.Marshal(queryMsg)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	endpoint := fmt.Sprintf(
		"%s/cosmwasm/wasm/v1/contract/%s/smart/%s",
		strings.TrimRight(c.cfg.LCDEndpoint, "/"),
		url.PathEscape(c.cfg.ContractAddress),
		url.PathEscape(encoded),
	)
	resp, err := c.http.Get(endpoint)
	if err != nil {
		return fmt.Errorf("lcd query: %w", err)
	}
	defer resp.Body.Close()
	var body lcdSmartResponse
	dec := json.NewDecoder(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var lcdErr struct {
			Message string `json:"message"`
		}
		_ = dec.Decode(&lcdErr)
		return fmt.Errorf("lcd query: HTTP %d: %s", resp.StatusCode, lcdErr.Message)
	}
	if err := dec.Decode(&body); err != nil {
		return fmt.Errorf("lcd query: decode: %w", err)
	}
	return json.Unmarshal(body.Data, out)
}

// ListRefs returns all refs of owner/repo (handles pagination).
func (c *Client) ListRefs(owner, repo string) ([]RefInfo, error) {
	var all []RefInfo
	var startAfter *string
	for {
		query := map[string]any{
			"list_refs": map[string]any{
				"owner":       owner,
				"repo":        repo,
				"start_after": startAfter,
				"limit":       100,
			},
		}
		var page listRefsResponse
		if err := c.SmartQuery(query, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Refs...)
		if len(page.Refs) < 100 {
			return all, nil
		}
		last := page.Refs[len(page.Refs)-1].RefName
		startAfter = &last
	}
}

// ResolveRef resolves one ref to (sha, pack URIs).
func (c *Client) ResolveRef(owner, repo, refName string) (string, []string, error) {
	query := map[string]any{
		"resolve_ref": map[string]any{
			"owner":    owner,
			"repo":     repo,
			"ref_name": refName,
		},
	}
	var out resolveRefResponse
	if err := c.SmartQuery(query, &out); err != nil {
		return "", nil, err
	}
	return out.CommitSha, out.PackURIs, nil
}

// RepoInfo fetches repository metadata.
func (c *Client) RepoInfo(owner, repo string) (*RepoInfo, error) {
	query := map[string]any{
		"repo_info": map[string]any{"owner": owner, "repo": repo},
	}
	var out RepoInfo
	if err := c.SmartQuery(query, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListCollaborators returns all collaborators of owner/repo (handles pagination).
func (c *Client) ListCollaborators(owner, repo string) ([]CollaboratorInfo, error) {
	var all []CollaboratorInfo
	var startAfter *string
	for {
		query := map[string]any{
			"list_collaborators": map[string]any{
				"owner":       owner,
				"repo":        repo,
				"start_after": startAfter,
				"limit":       100,
			},
		}
		var page listCollaboratorsResponse
		if err := c.SmartQuery(query, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Collaborators...)
		if len(page.Collaborators) < 100 {
			return all, nil
		}
		last := page.Collaborators[len(page.Collaborators)-1].Address
		startAfter = &last
	}
}

// ---- transactions via injectived ----

// Execute signs and broadcasts a wasm execute tx through injectived,
// then waits for inclusion and surfaces the DeliverTx result: with
// --broadcast-mode sync the immediate response only covers CheckTx, so
// contract-level rejections (frozen repo, ref conflict, unauthorized)
// only show up once the tx is in a block.
func (c *Client) Execute(execMsg any) error {
	return c.ExecuteWithFunds(execMsg, "")
}

// ExecuteWithFunds is Execute with coins attached (e.g. "1000inj").
func (c *Client) ExecuteWithFunds(execMsg any, amount string) error {
	raw, err := json.Marshal(execMsg)
	if err != nil {
		return err
	}
	args := []string{
		"tx", "wasm", "execute", c.cfg.ContractAddress, string(raw),
		"--from", c.cfg.KeyName,
		"--chain-id", c.cfg.ChainID,
		"--node", c.cfg.Node,
		"--keyring-backend", c.cfg.KeyringBackend,
		"--gas", "auto",
		"--gas-adjustment", "1.4",
		"--gas-prices", c.cfg.GasPrices,
		"--broadcast-mode", "sync",
		"--output", "json",
		"--yes",
	}
	if amount != "" {
		args = append(args, "--amount", amount)
	}
	bin := c.cfg.InjectivedBin
	if bin == "" {
		bin = "injectived"
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("injectived tx failed: %w\n%s", err, out)
	}
	// injectived returns JSON with a "code" field; non-zero means CheckTx failed
	var result struct {
		Code   int    `json:"code"`
		RawLog string `json:"raw_log"`
		TxHash string `json:"txhash"`
	}
	if jsonStart := strings.Index(string(out), "{"); jsonStart >= 0 {
		_ = json.Unmarshal(out[jsonStart:], &result)
	}
	if result.Code != 0 {
		return fmt.Errorf("tx rejected (code %d): %s", result.Code, result.RawLog)
	}
	if result.TxHash == "" {
		return fmt.Errorf("injectived returned no txhash:\n%s", out)
	}
	return c.waitTx(result.TxHash)
}

// waitTx polls the LCD until the tx is included and returns an error if
// DeliverTx failed (e.g. the contract rejected the message).
func (c *Client) waitTx(txHash string) error {
	endpoint := fmt.Sprintf(
		"%s/cosmos/tx/v1beta1/txs/%s",
		strings.TrimRight(c.cfg.LCDEndpoint, "/"), url.PathEscape(txHash),
	)
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := c.http.Get(endpoint)
		if err == nil && resp.StatusCode == http.StatusOK {
			var body struct {
				TxResponse struct {
					Code   int    `json:"code"`
					RawLog string `json:"raw_log"`
				} `json:"tx_response"`
			}
			err = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if err == nil {
				if body.TxResponse.Code != 0 {
					return fmt.Errorf(
						"tx %s failed on chain (code %d): %s",
						txHash, body.TxResponse.Code, body.TxResponse.RawLog,
					)
				}
				return nil
			}
		} else if resp != nil {
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tx %s not found on chain after 30s (network congestion?)", txHash)
		}
		time.Sleep(2 * time.Second)
	}
}

// CreateRepo registers a new repository owned by the configured key.
func (c *Client) CreateRepo(name, description, defaultBranch string) error {
	msg := map[string]any{
		"create_repo": map[string]any{
			"name": name,
		},
	}
	inner := msg["create_repo"].(map[string]any)
	if description != "" {
		inner["description"] = description
	}
	if defaultBranch != "" {
		inner["default_branch"] = defaultBranch
	}
	return c.Execute(msg)
}

// UpdateRef pushes a new tip for a ref.
func (c *Client) UpdateRef(owner, repo, refName, commitSha string, uris []string, expectedSha string, force bool) error {
	inner := map[string]any{
		"owner":      owner,
		"repo":       repo,
		"ref_name":   refName,
		"commit_sha": commitSha,
		"pack_uris":  uris,
		"force":      force,
	}
	if expectedSha != "" {
		inner["expected_sha"] = expectedSha
	}
	return c.Execute(map[string]any{"update_ref": inner})
}

// DeleteRef removes a ref.
func (c *Client) DeleteRef(owner, repo, refName string) error {
	return c.Execute(map[string]any{
		"delete_ref": map[string]any{
			"owner":    owner,
			"repo":     repo,
			"ref_name": refName,
		},
	})
}

// SetCollaborator adds/updates a collaborator role ("maintainer"|"reader");
// an empty role removes the collaborator. Owner only.
func (c *Client) SetCollaborator(repo, collaborator, role string) error {
	inner := map[string]any{
		"repo":         repo,
		"collaborator": collaborator,
	}
	if role != "" {
		inner["role"] = role
	}
	return c.Execute(map[string]any{"set_collaborator": inner})
}

// TransferOwnership moves a repo owned by the configured key to newOwner.
func (c *Client) TransferOwnership(repo, newOwner string) error {
	return c.Execute(map[string]any{
		"transfer_ownership": map[string]any{
			"repo":      repo,
			"new_owner": newOwner,
		},
	})
}

func (c *Client) CancelOwnershipTransfer(repo string) error {
	return c.Execute(map[string]any{"cancel_ownership_transfer": map[string]any{"repo": repo}})
}

func (c *Client) AcceptOwnership(owner, repo string) error {
	return c.Execute(map[string]any{"accept_ownership": map[string]any{"owner": owner, "repo": repo}})
}

func (c *Client) SetGuardians(repo string, guardians []string, threshold uint8) error {
	return c.Execute(map[string]any{"set_guardians": map[string]any{
		"repo": repo, "guardians": guardians, "threshold": threshold,
	}})
}

func (c *Client) ProposeRecovery(owner, repo, newOwner string) error {
	return c.Execute(map[string]any{"propose_recovery": map[string]any{
		"owner": owner, "repo": repo, "new_owner": newOwner,
	}})
}

func (c *Client) ApproveRecovery(owner, repo string) error {
	return c.Execute(map[string]any{"approve_recovery": map[string]any{"owner": owner, "repo": repo}})
}

func (c *Client) CancelRecovery(repo string) error {
	return c.Execute(map[string]any{"cancel_recovery": map[string]any{"repo": repo}})
}

func (c *Client) AcceptRecovery(owner, repo string) error {
	return c.Execute(map[string]any{"accept_recovery": map[string]any{"owner": owner, "repo": repo}})
}

func (c *Client) OwnershipSecurity(owner, repo string) (*OwnershipSecurityInfo, error) {
	var out OwnershipSecurityInfo
	if err := c.SmartQuery(map[string]any{
		"ownership_security": map[string]any{"owner": owner, "repo": repo},
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateRepoInfo updates description and/or default branch. Owner only;
// nil pointers leave the field unchanged.
func (c *Client) UpdateRepoInfo(repo string, description, defaultBranch *string) error {
	inner := map[string]any{"repo": repo}
	if description != nil {
		inner["description"] = *description
	}
	if defaultBranch != nil {
		inner["default_branch"] = *defaultBranch
	}
	return c.Execute(map[string]any{"update_repo_info": inner})
}

// SetModerationStatus changes a repo's moderation state
// ("active"|"delisted"|"frozen"). Committee/admin only.
func (c *Client) SetModerationStatus(owner, repo, status, reasonHash string) error {
	inner := map[string]any{
		"owner":  owner,
		"repo":   repo,
		"status": status,
	}
	if reasonHash != "" {
		inner["reason_hash"] = reasonHash
	}
	return c.Execute(map[string]any{"set_moderation_status": inner})
}

func (c *Client) SubmitModerationReport(owner, repo, reasonHash string) error {
	return c.Execute(map[string]any{"submit_moderation_report": map[string]any{
		"owner": owner, "repo": repo, "reason_hash": reasonHash,
	}})
}

func (c *Client) ResolveModerationReport(id uint64, status, reasonHash string) error {
	return c.Execute(map[string]any{"resolve_moderation_report": map[string]any{
		"report_id": id, "status": status, "reason_hash": reasonHash,
	}})
}

func (c *Client) AppealModerationReport(id uint64, reasonHash string) error {
	return c.Execute(map[string]any{"appeal_moderation_report": map[string]any{
		"report_id": id, "reason_hash": reasonHash,
	}})
}

func (c *Client) ResolveModerationAppeal(id uint64, status, reasonHash string) error {
	return c.Execute(map[string]any{"resolve_moderation_appeal": map[string]any{
		"report_id": id, "status": status, "reason_hash": reasonHash,
	}})
}

func (c *Client) ModerationReport(id uint64) (*ModerationReportInfo, error) {
	var out ModerationReportInfo
	if err := c.SmartQuery(map[string]any{
		"moderation_report": map[string]any{"report_id": id},
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Sponsor sends funds to a repo; the contract splits them instantly.
func (c *Client) Sponsor(owner, repo, message, amount string) error {
	inner := map[string]any{"owner": owner, "repo": repo}
	if message != "" {
		inner["message"] = message
	}
	return c.ExecuteWithFunds(map[string]any{"sponsor": inner}, amount)
}

// SetRevenueSplits replaces the repo's split table. Owner only.
func (c *Client) SetRevenueSplits(repo string, splits []SplitEntry) error {
	return c.Execute(map[string]any{
		"set_revenue_splits": map[string]any{"repo": repo, "splits": splits},
	})
}

// RegisterUsername claims a username, attaching the configured deposit.
func (c *Client) RegisterUsername(name, deposit string) error {
	return c.ExecuteWithFunds(map[string]any{
		"register_username": map[string]any{"name": name},
	}, deposit)
}

// ReleaseUsername frees the sender's username and refunds the deposit.
func (c *Client) ReleaseUsername() error {
	return c.Execute(map[string]any{"release_username": map[string]any{}})
}

// ForkRepo copies owner/repo (metadata + refs) into the sender's namespace.
func (c *Client) ForkRepo(owner, repo, newName string) error {
	inner := map[string]any{"owner": owner, "repo": repo}
	if newName != "" {
		inner["name"] = newName
	}
	return c.Execute(map[string]any{"fork_repo": inner})
}

// AwardBadge grants a non-transferable contribution badge. Owner only.
func (c *Client) AwardBadge(repo, recipient, reason string) error {
	return c.Execute(map[string]any{
		"award_badge": map[string]any{
			"repo":      repo,
			"recipient": recipient,
			"reason":    reason,
		},
	})
}

// RegisterRelease publishes immutable SHA-256 records for one release.
func (c *Client) RegisterRelease(version string, artifacts []ReleaseArtifact) error {
	inputs := make([]map[string]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		inputs = append(inputs, map[string]string{
			"platform": artifact.Platform,
			"sha256":   artifact.SHA256,
		})
	}
	return c.Execute(map[string]any{
		"register_release": map[string]any{
			"version":   version,
			"artifacts": inputs,
		},
	})
}

// ScheduleUpgrade announces an exact Wasm hash. The contract enforces its
// 14-day delay before a matching migrate transaction can execute.
func (c *Client) ScheduleUpgrade(wasmSHA256 string) error {
	return c.Execute(map[string]any{"schedule_upgrade": map[string]any{
		"wasm_sha256": strings.ToLower(wasmSHA256),
	}})
}

// CancelUpgrade removes the pending upgrade announcement.
func (c *Client) CancelUpgrade() error {
	return c.Execute(map[string]any{"cancel_upgrade": map[string]any{}})
}

// UpgradeSecurity returns the pending upgrade proposal and delay.
func (c *Client) UpgradeSecurity() (*UpgradeSecurityInfo, error) {
	var out UpgradeSecurityInfo
	if err := c.SmartQuery(map[string]any{"upgrade_security": map[string]any{}}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReleaseArtifacts returns the chain-registered checksums for a version.
func (c *Client) ReleaseArtifacts(version string) ([]ReleaseArtifact, error) {
	var out struct {
		Artifacts []ReleaseArtifact `json:"artifacts"`
	}
	if err := c.SmartQuery(map[string]any{
		"release_artifacts": map[string]any{"version": version},
	}, &out); err != nil {
		return nil, err
	}
	return out.Artifacts, nil
}

// Badge mirrors the contract's Badge struct.
type Badge struct {
	ID        uint64 `json:"id"`
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
	Recipient string `json:"recipient"`
	Reason    string `json:"reason"`
	AwardedAt uint64 `json:"awarded_at"`
}

// BadgesByRecipient lists a contributor's trophy wall.
func (c *Client) BadgesByRecipient(recipient string) ([]Badge, error) {
	var out struct {
		Badges []Badge `json:"badges"`
	}
	err := c.SmartQuery(map[string]any{
		"badges_by_recipient": map[string]any{"recipient": recipient, "limit": 100},
	}, &out)
	return out.Badges, err
}

// ConfigInfo fetches the contract-level config (treasury, fee, deposit).
func (c *Client) ConfigInfo() (*ConfigInfo, error) {
	var out ConfigInfo
	if err := c.SmartQuery(map[string]any{"config": map[string]any{}}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResolveUsername resolves a registered username to its owner address.
func (c *Client) ResolveUsername(name string) (string, error) {
	var out usernameResponse
	err := c.SmartQuery(map[string]any{
		"resolve_username": map[string]any{"name": name},
	}, &out)
	if err != nil {
		return "", fmt.Errorf("resolve username %q: %w", name, err)
	}
	return out.Owner, nil
}

// AddressUsername reverse-resolves an address to its username ("" if none).
func (c *Client) AddressUsername(address string) (string, error) {
	var out addressUsernameResponse
	err := c.SmartQuery(map[string]any{
		"address_username": map[string]any{"address": address},
	}, &out)
	if err != nil {
		return "", err
	}
	if out.Name == nil {
		return "", nil
	}
	return *out.Name, nil
}

// RevenueSplits fetches the split table of a repository.
func (c *Client) RevenueSplits(owner, repo string) ([]SplitEntry, error) {
	var out revenueSplitsResponse
	err := c.SmartQuery(map[string]any{
		"revenue_splits": map[string]any{"owner": owner, "repo": repo},
	}, &out)
	return out.Splits, err
}

// OwnerAddress returns the bech32 address of the configured key.
func (c *Client) OwnerAddress() (string, error) {
	bin := c.cfg.InjectivedBin
	if bin == "" {
		bin = "injectived"
	}
	cmd := exec.Command(bin,
		"keys", "show", c.cfg.KeyName,
		"--keyring-backend", c.cfg.KeyringBackend,
		"--address",
	)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("injectived keys show %s: %w", c.cfg.KeyName, err)
	}
	return strings.TrimSpace(string(out)), nil
}
