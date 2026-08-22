package chain

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type RepoMovedError struct {
	RepoID       [32]byte
	CurrentOwner string
	Name         string
}

func (e *RepoMovedError) CanonicalURL() string {
	return fmt.Sprintf("igit://%s/%s", e.CurrentOwner, e.Name)
}

func (e *RepoMovedError) Error() string {
	return fmt.Sprintf("repository moved to %s", e.CanonicalURL())
}

type LocatorNotFoundError struct {
	Owner string
	Name  string
}

func (e *LocatorNotFoundError) Error() string {
	return fmt.Sprintf("repository locator not found: igit://%s/%s", e.Owner, e.Name)
}

// EVMContractRevertError retains the RPC error while exposing a decoded
// Solidity custom error to errors.As callers.
type EVMContractRevertError struct {
	RPC    *RPCError
	Revert error
}

func (e *EVMContractRevertError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v: %v", e.RPC, e.Revert)
}

func (e *EVMContractRevertError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return []error{e.RPC, e.Revert}
}

type suiteCustomError struct {
	Name   string
	Values []any
}

func (e *suiteCustomError) Error() string {
	return fmt.Sprintf("EVM contract reverted: %s%v", e.Name, e.Values)
}

func wrapEVMRPCError(err error) error {
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Data == "" {
		return err
	}
	decoded := decodeSuiteRevert(rpcErr.Data)
	if decoded == nil {
		return err
	}
	return &EVMContractRevertError{RPC: rpcErr, Revert: decoded}
}

func decodeSuiteRevert(data string) error {
	raw, err := decodeHexBytes(data)
	if err != nil {
		return err
	}
	if reason, unpackErr := gethabi.UnpackRevert(raw); unpackErr == nil {
		return errors.New("EVM contract reverted: " + reason)
	}
	if len(raw) < 4 {
		return fmt.Errorf("EVM contract reverted with %s", normalizeHex(data))
	}
	for _, name := range []suiteABIName{
		suiteABIDirectory, suiteABICoordinator, suiteABICore, suiteABIRecovery,
		suiteABIModeration, suiteABIEconomic, suiteABIUsername, suiteABIBadge, suiteABIRelease,
	} {
		contractABI, loadErr := loadSuiteABI(name)
		if loadErr != nil {
			return loadErr
		}
		for _, custom := range contractABI.Errors {
			if !bytes.Equal(raw[:4], custom.ID[:4]) {
				continue
			}
			values, unpackErr := custom.Inputs.Unpack(raw[4:])
			if unpackErr != nil {
				return fmt.Errorf("decode Solidity error %s: %w", custom.Name, unpackErr)
			}
			if custom.Name == "LocatorNotFound" && len(values) == 2 {
				owner, ownerOK := values[0].(common.Address)
				locator, nameOK := values[1].(string)
				if ownerOK && nameOK {
					user, conversionErr := userAddressFromEVM(owner.Hex())
					if conversionErr != nil {
						return conversionErr
					}
					return &LocatorNotFoundError{Owner: user, Name: locator}
				}
			}
			return &suiteCustomError{Name: custom.Name, Values: values}
		}
	}
	return fmt.Errorf("EVM contract reverted with selector 0x%s", strings.ToLower(hexEncode(raw[:4])))
}
