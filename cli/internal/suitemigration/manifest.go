package suitemigration

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

//go:embed abi/BootstrapCoordinator.json
var coordinatorABIJSON string

var coordinatorABI = func() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(coordinatorABIJSON))
	if err != nil {
		panic(err)
	}
	return parsed
}()

func BuildCalldataManifest(plan *Plan) (*CalldataManifest, error) {
	if err := ValidatePlan(plan); err != nil {
		return nil, fmt.Errorf("validate suite plan: %w", err)
	}
	planSHA, err := PlanSHA256(plan)
	if err != nil {
		return nil, err
	}
	manifest := &CalldataManifest{
		Schema: ManifestSchema, CalldataReady: true, Signed: false, Broadcast: false,
		PlanSHA256: planSHA, SnapshotRoot: plan.Snapshot.Root, Target: plan.Target,
	}
	order := 0
	appendTransaction := func(module *ModulePlan, sequence *uint64, phase, functionName, eventName string, arguments ...any) error {
		data, err := coordinatorABI.Pack(functionName, arguments...)
		if err != nil {
			return fmt.Errorf("encode %s: %w", functionName, err)
		}
		transaction := ManifestTransaction{
			Order: order, Sequence: sequence, Phase: phase,
			Function:      coordinatorABI.Methods[functionName].Sig,
			ExpectedEvent: eventName, ExpectedTopic: coordinatorABI.Events[eventName].ID.Hex(),
			To: plan.Target.Coordinator, Value: "0x0", Data: hexBytes(data),
		}
		if module != nil {
			moduleOrder := module.Order
			transaction.ModuleOrder = &moduleOrder
			transaction.Module = module.Name
			transaction.ModuleID = module.ID
		}
		manifest.Transactions = append(manifest.Transactions, transaction)
		order++
		return nil
	}
	for moduleIndex := range plan.Modules {
		module := &plan.Modules[moduleIndex]
		moduleID, _ := parseHash("module ID", module.ID)
		root, _ := parseHash("module root", module.ExpectedRoot)
		if err := appendTransaction(module, nil, "begin_module", "beginNextModule", "ModuleImportStarted",
			moduleID, new(big.Int).SetUint64(module.ExpectedCount), new(big.Int).SetUint64(module.ExpectedBatches), root,
		); err != nil {
			return nil, err
		}
		for batchIndex := range module.Batches {
			batch := &module.Batches[batchIndex]
			payload, _ := parseHexBytes("batch payload", batch.Payload)
			payloadHash, _ := parseHash("batch payload hash", batch.PayloadHash)
			sequence := batch.Sequence
			if err := appendTransaction(module, &sequence, "import_batch", "importBatch", "ModuleBatchImported",
				moduleID, new(big.Int).SetUint64(batch.Sequence), new(big.Int).SetUint64(batch.Count), payloadHash, payload,
			); err != nil {
				return nil, err
			}
		}
		if module.Name == "username" {
			evidenceHash, _ := parseHash("username escrow evidence hash", plan.UsernameEscrowEvidenceHash)
			if err := appendTransaction(module, nil, "attest_username_escrow", "attestUsernameEscrowReleased", "UsernameEscrowReleaseAttested", evidenceHash); err != nil {
				return nil, err
			}
		}
		if err := appendTransaction(module, nil, "finalize_module", "finalizeCurrentModule", "ModuleImportFinalized", moduleID); err != nil {
			return nil, err
		}
	}
	if err := appendTransaction(nil, nil, "activate_suite", "activateSuite", "SuiteActivationRequested"); err != nil {
		return nil, err
	}
	if len(manifest.Transactions) != plan.Summary.TransactionCount {
		return nil, fmt.Errorf("generated %d transactions; plan requires %d", len(manifest.Transactions), plan.Summary.TransactionCount)
	}
	for index := range manifest.Transactions {
		transaction := &manifest.Transactions[index]
		method, _, err := calldataMethod(transaction.Data)
		if err != nil {
			return nil, fmt.Errorf("verify transaction %d calldata: %w", index, err)
		}
		if transaction.Order != index || transaction.Function != method.Sig || transaction.To != plan.Target.Coordinator || transaction.Value != "0x0" {
			return nil, fmt.Errorf("transaction %d metadata does not match canonical calldata target", index)
		}
	}
	return manifest, nil
}

func ValidateCalldataManifest(plan *Plan, manifest *CalldataManifest) error {
	if manifest == nil {
		return errors.New("calldata manifest is nil")
	}
	if manifest.Schema != ManifestSchema || !manifest.CalldataReady || manifest.Signed || manifest.Broadcast {
		return errors.New("unsupported, signed, broadcast, or non-ready calldata manifest")
	}
	expected, err := BuildCalldataManifest(plan)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(manifest, expected) {
		return errors.New("calldata manifest does not exactly match the validated suite plan")
	}
	return nil
}

func MarshalCalldataManifest(plan *Plan, manifest *CalldataManifest) ([]byte, error) {
	if err := ValidateCalldataManifest(plan, manifest); err != nil {
		return nil, err
	}
	return json.MarshalIndent(manifest, "", "  ")
}

func ReadCalldataManifest(path string, plan *Plan) (*CalldataManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, fmt.Errorf("decode calldata manifest: %w", err)
	}
	var manifest CalldataManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("calldata manifest contains trailing JSON data")
	}
	if err := ValidateCalldataManifest(plan, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func CoordinatorABI() abi.ABI { return coordinatorABI }

func calldataMethod(data string) (*abi.Method, []any, error) {
	raw, err := parseHexBytes("transaction calldata", data)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) < 4 {
		return nil, nil, errors.New("transaction calldata is shorter than a selector")
	}
	method, err := coordinatorABI.MethodById(raw[:4])
	if err != nil {
		return nil, nil, err
	}
	values, err := method.Inputs.Unpack(raw[4:])
	if err != nil {
		return nil, nil, err
	}
	repacked, err := method.Inputs.Pack(values...)
	if err != nil || !bytes.Equal(repacked, raw[4:]) {
		return nil, nil, errors.New("transaction calldata is not canonical ABI")
	}
	return method, values, nil
}
