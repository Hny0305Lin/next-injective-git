package chain

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// EVMModerationModule implements report, resolution, and appeal records in an
// independently deployed module keyed by the core registry's stable repo ID.
// Its effective status is advisory until a reviewed enforcement bridge makes
// the core registry consume it; callers must not describe module-only frozen
// decisions as blocking direct core ref writes.
type EVMModerationModule struct {
	cfg      config.Config
	registry *EVMRegistryV2
	rpc      *EVMRPC
	signer   EVMSigner
	address  string
}

var _ ModerationBackend = (*EVMModerationModule)(nil)

func NewEVMModerationModule(cfg config.Config) *EVMModerationModule {
	registry := NewEVMRegistryV2(cfg)
	return NewEVMModerationModuleWithDependencies(cfg, registry, registry.rpc, registry.signer)
}

func NewEVMModerationModuleWithDependencies(
	cfg config.Config,
	registry *EVMRegistryV2,
	rpc *EVMRPC,
	signer EVMSigner,
) *EVMModerationModule {
	if registry == nil {
		registry = NewEVMRegistryV2WithDependencies(cfg, rpc, signer)
	}
	if rpc == nil {
		rpc = registry.rpc
	}
	if signer == nil {
		signer = registry.signer
	}
	return &EVMModerationModule{
		cfg: cfg, registry: registry, rpc: rpc, signer: signer,
		address: cfg.EffectiveEVMModerationModuleAddress(),
	}
}

func (m *EVMModerationModule) contract() (string, error) {
	if strings.TrimSpace(m.address) == "" {
		return "", fmt.Errorf("EVM V2 moderation module address is not configured")
	}
	return normalizeEVMAddress(m.address)
}

func moderationStatusCode(status string) (uint64, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return 0, nil
	case "frozen":
		return 1, nil
	case "delisted":
		return 2, nil
	default:
		return 0, fmt.Errorf("invalid moderation status %q (expected active, frozen, or delisted)", status)
	}
}

func moderationStatusName(status uint64) (string, error) {
	switch status {
	case 0:
		return "active", nil
	case 1:
		return "frozen", nil
	case 2:
		return "delisted", nil
	default:
		return "", fmt.Errorf("EVM moderation module returned invalid status %d", status)
	}
}

func reportStatusName(status uint64) (string, error) {
	switch status {
	case 0:
		return "open", nil
	case 1:
		return "resolved", nil
	case 2:
		return "appealed", nil
	case 3:
		return "appeal_resolved", nil
	default:
		return "", fmt.Errorf("EVM moderation module returned invalid report status %d", status)
	}
}

func (m *EVMModerationModule) resolveForWrite(owner, repo string) (*ResolvedRepo, error) {
	resolved, err := m.registry.ResolveRepo(owner, repo)
	if err != nil {
		return nil, err
	}
	if resolved.Backend != BackendEVM || resolved.WriteDisabled {
		return nil, fmt.Errorf("%w: %s", ErrLegacyWriteFallbackDisabled, resolved.CanonicalURL())
	}
	if !resolved.IsCanonical {
		return nil, &RepoMovedError{
			RepoID: resolved.RepoID, CurrentOwner: resolved.Canonical.Owner, Name: resolved.Canonical.Name,
		}
	}
	return resolved, nil
}

func (m *EVMModerationModule) send(data []byte) error {
	contract, err := m.contract()
	if err != nil {
		return err
	}
	return m.registry.sendTo(context.Background(), contract, data)
}

func (m *EVMModerationModule) SetModerationStatus(owner, repo, status, reasonHash string) error {
	resolved, err := m.resolveForWrite(owner, repo)
	if err != nil {
		return err
	}
	code, err := moderationStatusCode(status)
	if err != nil {
		return err
	}
	data, err := encodeABICall(
		"setModerationStatus(bytes32,uint8,string)",
		abiBytes32Value(resolved.RepoID), abiUintValue(code), abiStringValue(reasonHash),
	)
	if err != nil {
		return err
	}
	return m.send(data)
}

func (m *EVMModerationModule) SubmitModerationReport(owner, repo, reasonHash string) error {
	resolved, err := m.resolveForWrite(owner, repo)
	if err != nil {
		return err
	}
	data, err := encodeABICall(
		"submitReport(bytes32,string)", abiBytes32Value(resolved.RepoID), abiStringValue(reasonHash),
	)
	if err != nil {
		return err
	}
	return m.send(data)
}

func (m *EVMModerationModule) ResolveModerationReport(id uint64, status, reasonHash string) error {
	return m.sendReportDecision("resolveReport(uint256,uint8,string)", id, status, reasonHash)
}

func (m *EVMModerationModule) ResolveModerationAppeal(id uint64, status, reasonHash string) error {
	return m.sendReportDecision("resolveAppeal(uint256,uint8,string)", id, status, reasonHash)
}

func (m *EVMModerationModule) sendReportDecision(signature string, id uint64, status, reasonHash string) error {
	code, err := moderationStatusCode(status)
	if err != nil {
		return err
	}
	data, err := encodeABICall(
		signature, abiUintValue(id), abiUintValue(code), abiStringValue(reasonHash),
	)
	if err != nil {
		return err
	}
	return m.send(data)
}

func (m *EVMModerationModule) AppealModerationReport(id uint64, reasonHash string) error {
	data, err := encodeABICall(
		"appealReport(uint256,string)", abiUintValue(id), abiStringValue(reasonHash),
	)
	if err != nil {
		return err
	}
	return m.send(data)
}

func (m *EVMModerationModule) ModerationReport(id uint64) (*ModerationReportInfo, error) {
	contract, err := m.contract()
	if err != nil {
		return nil, err
	}
	data, err := encodeABICall("getReport(uint256)", abiUintValue(id))
	if err != nil {
		return nil, err
	}
	result, err := m.rpc.CallContractAt(context.Background(), contract, "0x"+hexEncode(data), "latest")
	if err != nil {
		return nil, wrapEVMRPCError(err)
	}
	return decodeEVMModerationReport(result)
}

func decodeEVMModerationReport(data []byte) (*ModerationReportInfo, error) {
	base, err := readABIOffset(data, 0, 0)
	if err != nil {
		return nil, err
	}
	id, err := readABIUint(data, base)
	if err != nil || id == 0 {
		if err == nil {
			err = fmt.Errorf("EVM moderation report returned zero ID")
		}
		return nil, err
	}
	owner, err := readABIAddress(data, base+64)
	if err != nil {
		return nil, err
	}
	repoNameOffset, err := readABIOffset(data, base+96, base)
	if err != nil {
		return nil, err
	}
	repoName, err := readABIString(data, repoNameOffset)
	if err != nil {
		return nil, err
	}
	reporter, err := readABIAddress(data, base+128)
	if err != nil {
		return nil, err
	}
	reasonOffset, err := readABIOffset(data, base+160, base)
	if err != nil {
		return nil, err
	}
	reason, err := readABIString(data, reasonOffset)
	if err != nil {
		return nil, err
	}
	statusCode, err := readABIUint(data, base+192)
	if err != nil {
		return nil, err
	}
	status, err := reportStatusName(statusCode)
	if err != nil {
		return nil, err
	}
	resolutionCode, err := readABIUint(data, base+224)
	if err != nil {
		return nil, err
	}
	hasResolution, err := readABIBool(data, base+256)
	if err != nil {
		return nil, err
	}
	resolutionOffset, err := readABIOffset(data, base+288, base)
	if err != nil {
		return nil, err
	}
	resolutionHash, err := readABIString(data, resolutionOffset)
	if err != nil {
		return nil, err
	}
	appealOffset, err := readABIOffset(data, base+320, base)
	if err != nil {
		return nil, err
	}
	appealHash, err := readABIString(data, appealOffset)
	if err != nil {
		return nil, err
	}
	hasAppeal, err := readABIBool(data, base+352)
	if err != nil {
		return nil, err
	}
	createdAt, err := readABIUint(data, base+384)
	if err != nil {
		return nil, err
	}
	updatedAt, err := readABIUint(data, base+416)
	if err != nil {
		return nil, err
	}

	resolution := ""
	var resolutionHashPtr *string
	if hasResolution {
		resolution, err = moderationStatusName(resolutionCode)
		if err != nil {
			return nil, err
		}
		resolutionHashPtr = &resolutionHash
	}
	var appealHashPtr *string
	if hasAppeal {
		appealHashPtr = &appealHash
	}
	owner, err = userAddressFromEVM(owner)
	if err != nil {
		return nil, fmt.Errorf("decode moderation report owner: %w", err)
	}
	reporter, err = userAddressFromEVM(reporter)
	if err != nil {
		return nil, fmt.Errorf("decode moderation report reporter: %w", err)
	}
	return &ModerationReportInfo{
		ID: id, Owner: owner, Repo: repoName, Reporter: reporter, ReasonHash: reason,
		Status: status, Resolution: resolution, ResolutionHash: resolutionHashPtr,
		AppealHash: appealHashPtr, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}
