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
	RefName      string   `json:"ref_name"`
	CommitSha    string   `json:"commit_sha"`
	PackfileCids []string `json:"packfile_cids"`
	UpdatedAt    uint64   `json:"updated_at"`
	UpdatedBy    string   `json:"updated_by"`
}

// RepoInfo mirrors the contract's RepoInfoResponse.
type RepoInfo struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	CreatedAt     uint64 `json:"created_at"`
	UpdatedAt     uint64 `json:"updated_at"`
}

type listRefsResponse struct {
	Refs []RefInfo `json:"refs"`
}

type resolveRefResponse struct {
	RefName      string   `json:"ref_name"`
	CommitSha    string   `json:"commit_sha"`
	PackfileCids []string `json:"packfile_cids"`
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

// ResolveRef resolves one ref to (sha, cids).
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
	return out.CommitSha, out.PackfileCids, nil
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

// ---- transactions via injectived ----

// Execute signs and broadcasts a wasm execute tx through injectived.
func (c *Client) Execute(execMsg any) error {
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
	return nil
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
func (c *Client) UpdateRef(owner, repo, refName, commitSha string, cids []string, expectedSha string, force bool) error {
	inner := map[string]any{
		"owner":         owner,
		"repo":          repo,
		"ref_name":      refName,
		"commit_sha":    commitSha,
		"packfile_cids": cids,
		"force":         force,
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
