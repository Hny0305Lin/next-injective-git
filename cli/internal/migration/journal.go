package migration

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	ReceiptJournalSchema          = "igit.evm-v2.receipt-journal.v1"
	PreparedJournalRecordSchema   = "igit.evm-v2.receipt-journal.prepared.v1"
	BroadcastJournalRecordSchema  = "igit.evm-v2.receipt-journal.broadcast.v1"
	MinedJournalRecordSchema      = "igit.evm-v2.receipt-journal.mined.v1"
	RevertedJournalRecordSchema   = "igit.evm-v2.receipt-journal.reverted.v1"
	receiptJournalHeaderFile      = "journal.json"
	receiptJournalTemporaryPrefix = ".igit-receipt-journal-"
)

// JournalPublishedUnsyncedError means the transition's final no-clobber hard
// link already exists, but syncing the containing directory failed. Callers
// must treat the transition as published, stop the current run, and reopen the
// same journal directory before deciding whether recovery is possible. The
// wrapped error is retained so callers can inspect the filesystem failure with
// errors.Is/errors.As.
type JournalPublishedUnsyncedError struct {
	Path string
	Err  error
}

func (err *JournalPublishedUnsyncedError) Error() string {
	if err == nil {
		return "receipt journal transition was published but directory sync failed"
	}
	return fmt.Sprintf(
		"receipt journal transition %s was published but directory sync failed: %v; reopen the journal before retrying",
		err.Path,
		err.Err,
	)
}

func (err *JournalPublishedUnsyncedError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

// ReceiptJournalHeader binds an append-only execution journal to one exact
// plan, canonical transaction manifest, chain, deployment, and signer.
type ReceiptJournalHeader struct {
	Schema           string   `json:"schema"`
	ImportScope      string   `json:"import_scope"`
	DeferredSections []string `json:"deferred_sections"`
	PlanSHA256       string   `json:"plan_sha256"`
	ManifestSHA256   string   `json:"manifest_sha256"`
	ChainID          uint64   `json:"chain_id"`
	Registry         string   `json:"registry"`
	Controller       string   `json:"controller,omitempty"`
	Signer           string   `json:"signer"`
	TransactionCount int      `json:"transaction_count"`
}

// SignedImportTransaction is the signed, replayable transaction persisted
// before eth_sendRawTransaction. RawTransaction contains no private key, but
// anyone who obtains it can broadcast it, so journal files are mode 0600.
type SignedImportTransaction struct {
	From            string `json:"from"`
	ChainID         uint64 `json:"chain_id"`
	Nonce           uint64 `json:"nonce"`
	GasLimit        uint64 `json:"gas_limit"`
	GasPrice        string `json:"gas_price"`
	Value           string `json:"value"`
	RawTransaction  string `json:"raw_transaction"`
	TransactionHash string `json:"transaction_hash"`
}

type PreparedJournalRecord struct {
	Schema         string                  `json:"schema"`
	PlanSHA256     string                  `json:"plan_sha256"`
	ManifestSHA256 string                  `json:"manifest_sha256"`
	Order          int                     `json:"order"`
	Transaction    SignedImportTransaction `json:"transaction"`
}

type BroadcastJournalRecord struct {
	Schema          string `json:"schema"`
	PlanSHA256      string `json:"plan_sha256"`
	ManifestSHA256  string `json:"manifest_sha256"`
	Order           int    `json:"order"`
	TransactionHash string `json:"transaction_hash"`
}

type MinedJournalRecord struct {
	Schema         string           `json:"schema"`
	PlanSHA256     string           `json:"plan_sha256"`
	ManifestSHA256 string           `json:"manifest_sha256"`
	Order          int              `json:"order"`
	Receipt        chain.EVMReceipt `json:"receipt"`
}

// RevertedJournalRecord preserves the first-class evidence for a mined
// transaction whose execution failed. A reverted transaction is terminal for
// this ordered import; the broadcast record remains alongside it so resume
// never signs or broadcasts a different transaction for the same nonce.
type RevertedJournalRecord struct {
	Schema         string           `json:"schema"`
	PlanSHA256     string           `json:"plan_sha256"`
	ManifestSHA256 string           `json:"manifest_sha256"`
	Order          int              `json:"order"`
	Receipt        chain.EVMReceipt `json:"receipt"`
}

// ReceiptJournal is an in-memory view of immutable transition files. It is
// safe for one process to use concurrently; filesystem no-clobber publication
// also prevents a second process from silently replacing a transition.
type ReceiptJournal struct {
	directory string
	plan      *Plan
	manifest  *TransactionManifest
	header    ReceiptJournalHeader

	// Tests replace this per-journal hook to exercise the point after the
	// no-clobber hard link exists but before directory durability is confirmed.
	syncDirectory func(string) error

	mu        sync.Mutex
	prepared  map[int]PreparedJournalRecord
	broadcast map[int]BroadcastJournalRecord
	mined     map[int]MinedJournalRecord
	reverted  map[int]RevertedJournalRecord
}

type ReceiptJournalStatus struct {
	Header                  ReceiptJournalHeader
	PreparedTransactions    int
	BroadcastTransactions   int
	MinedTransactions       int
	RevertedTransactions    int
	NextOrder               int
	FailedOrder             int
	Complete                bool
	FinalizeTransactionHash string
	RevertedTransactionHash string
}

// OpenReceiptJournal creates an empty journal or strictly validates and opens
// an existing one. The journal directory may not contain unrelated files.
func OpenReceiptJournal(
	directory string,
	plan *Plan,
	manifest *TransactionManifest,
	signer string,
) (*ReceiptJournal, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("receipt journal directory is required")
	}
	if err := VerifyTransactionManifest(plan, manifest); err != nil {
		return nil, fmt.Errorf("verify receipt journal manifest: %w", err)
	}
	planDigest, err := PlanSHA256(plan)
	if err != nil {
		return nil, fmt.Errorf("hash receipt journal plan: %w", err)
	}
	manifestDigest, err := TransactionManifestSHA256(manifest)
	if err != nil {
		return nil, fmt.Errorf("hash receipt journal manifest: %w", err)
	}
	normalizedSigner, err := chain.NormalizeEVMAddress(signer)
	if err != nil || normalizedSigner == zeroEVMAddress {
		return nil, fmt.Errorf("invalid receipt journal signer %q", signer)
	}
	expectedHeader := ReceiptJournalHeader{
		Schema: ReceiptJournalSchema, ImportScope: plan.ImportScope,
		DeferredSections: append([]string(nil), plan.DeferredSections...),
		PlanSHA256:       planDigest, ManifestSHA256: manifestDigest,
		ChainID: plan.Target.ChainID, Registry: plan.Target.Contract,
		Controller: plan.Target.Controller, Signer: normalizedSigner,
		TransactionCount: len(manifest.Transactions),
	}

	cleanDirectory, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return nil, fmt.Errorf("resolve receipt journal directory: %w", err)
	}
	if err := prepareReceiptJournalDirectory(cleanDirectory); err != nil {
		return nil, err
	}
	headerPath := filepath.Join(cleanDirectory, receiptJournalHeaderFile)
	var header ReceiptJournalHeader
	if _, err := os.Lstat(headerPath); errors.Is(err, os.ErrNotExist) {
		if err := requireJournalDirectoryEmpty(cleanDirectory); err != nil {
			return nil, err
		}
		if err := writeExclusiveJournalJSON(headerPath, expectedHeader); err != nil {
			return nil, fmt.Errorf("create receipt journal header: %w", err)
		}
		header = expectedHeader
	} else if err != nil {
		return nil, fmt.Errorf("inspect receipt journal header: %w", err)
	} else {
		loaded, err := readReceiptJournalHeaderAt(cleanDirectory)
		if err != nil {
			return nil, err
		}
		header = *loaded
		if !reflect.DeepEqual(header, expectedHeader) {
			return nil, errors.New("receipt journal header does not match the plan, manifest, target, or signer")
		}
	}

	journal := &ReceiptJournal{
		directory:     cleanDirectory,
		plan:          plan,
		manifest:      manifest,
		header:        header,
		syncDirectory: syncJournalDirectory,
		prepared:      make(map[int]PreparedJournalRecord),
		broadcast:     make(map[int]BroadcastJournalRecord),
		mined:         make(map[int]MinedJournalRecord),
		reverted:      make(map[int]RevertedJournalRecord),
	}
	if err := journal.loadRecords(); err != nil {
		return nil, err
	}
	return journal, nil
}

// ReadReceiptJournalHeader reads only the public execution identity. It does
// not create a directory, load a signer, or contact RPC.
func ReadReceiptJournalHeader(directory string) (*ReceiptJournalHeader, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("receipt journal directory is required")
	}
	cleanDirectory, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return nil, fmt.Errorf("resolve receipt journal directory: %w", err)
	}
	info, err := os.Lstat(cleanDirectory)
	if err != nil {
		return nil, fmt.Errorf("inspect receipt journal directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("receipt journal path is not a real directory: %s", cleanDirectory)
	}
	if err := validateJournalPermissions(cleanDirectory, info); err != nil {
		return nil, err
	}
	return readReceiptJournalHeaderAt(cleanDirectory)
}

// InspectReceiptJournal validates every local record and summarizes progress.
// It is offline evidence only: the header signer is journal metadata rather
// than an independently authenticated identity. ExecuteImport supplies the
// keystore signer and rechecks every terminal receipt and canonical block hash
// against RPC before resuming.
func InspectReceiptJournal(
	directory string,
	plan *Plan,
	manifest *TransactionManifest,
) (*ReceiptJournalStatus, error) {
	header, err := ReadReceiptJournalHeader(directory)
	if err != nil {
		return nil, err
	}
	journal, err := OpenReceiptJournal(directory, plan, manifest, header.Signer)
	if err != nil {
		return nil, err
	}
	status := &ReceiptJournalStatus{Header: journal.Header(), NextOrder: len(manifest.Transactions), FailedOrder: -1}
	for order := range manifest.Transactions {
		if _, ok := journal.Prepared(order); ok {
			status.PreparedTransactions++
		}
		if _, ok := journal.Broadcast(order); ok {
			status.BroadcastTransactions++
		}
		if mined, ok := journal.Mined(order); ok {
			status.MinedTransactions++
			if order == len(manifest.Transactions)-1 {
				status.FinalizeTransactionHash = mined.Receipt.TransactionHash
			}
			continue
		}
		if reverted, ok := journal.Reverted(order); ok {
			status.RevertedTransactions++
			if status.FailedOrder == -1 {
				status.FailedOrder = order
				status.RevertedTransactionHash = reverted.Receipt.TransactionHash
			}
			if status.NextOrder == len(manifest.Transactions) {
				status.NextOrder = order
			}
			continue
		}
		if status.NextOrder == len(manifest.Transactions) {
			status.NextOrder = order
		}
	}
	status.Complete = status.MinedTransactions == len(manifest.Transactions) && status.RevertedTransactions == 0
	return status, nil
}

func readReceiptJournalHeaderAt(directory string) (*ReceiptJournalHeader, error) {
	headerPath := filepath.Join(directory, receiptJournalHeaderFile)
	info, err := os.Lstat(headerPath)
	if err != nil {
		return nil, fmt.Errorf("inspect receipt journal header: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("receipt journal header is not a regular file")
	}
	if err := validateJournalPermissions(headerPath, info); err != nil {
		return nil, err
	}
	var header ReceiptJournalHeader
	if err := readStrictJournalJSON(headerPath, &header); err != nil {
		return nil, fmt.Errorf("read receipt journal header: %w", err)
	}
	header.DeferredSections = append([]string(nil), header.DeferredSections...)
	return &header, nil
}

func (journal *ReceiptJournal) Header() ReceiptJournalHeader {
	if journal == nil {
		return ReceiptJournalHeader{}
	}
	header := journal.header
	header.DeferredSections = append([]string(nil), journal.header.DeferredSections...)
	return header
}

func (journal *ReceiptJournal) Prepared(order int) (*PreparedJournalRecord, bool) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	record, ok := journal.prepared[order]
	if !ok {
		return nil, false
	}
	copyRecord := record
	return &copyRecord, true
}

func (journal *ReceiptJournal) Broadcast(order int) (*BroadcastJournalRecord, bool) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	record, ok := journal.broadcast[order]
	if !ok {
		return nil, false
	}
	copyRecord := record
	return &copyRecord, true
}

func (journal *ReceiptJournal) Mined(order int) (*MinedJournalRecord, bool) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	record, ok := journal.mined[order]
	if !ok {
		return nil, false
	}
	copyRecord := record
	copyRecord.Receipt = cloneEVMReceipt(record.Receipt)
	return &copyRecord, true
}

func (journal *ReceiptJournal) Reverted(order int) (*RevertedJournalRecord, bool) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	record, ok := journal.reverted[order]
	if !ok {
		return nil, false
	}
	copyRecord := record
	copyRecord.Receipt = cloneEVMReceipt(record.Receipt)
	return &copyRecord, true
}

func (journal *ReceiptJournal) RecordPrepared(order int, transaction SignedImportTransaction) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if err := journal.requireNextOrder(order); err != nil {
		return err
	}
	if _, exists := journal.prepared[order]; exists {
		return fmt.Errorf("transaction %d already has a prepared record", order)
	}
	record := PreparedJournalRecord{
		Schema: PreparedJournalRecordSchema, PlanSHA256: journal.header.PlanSHA256,
		ManifestSHA256: journal.header.ManifestSHA256, Order: order, Transaction: transaction,
	}
	if err := journal.validatePrepared(record); err != nil {
		return err
	}
	writeErr := writeExclusiveJournalJSONWithSync(
		journal.recordPath(order, "prepared"), record, journal.syncDirectory,
	)
	if writeErr != nil && !journalTransitionPublished(writeErr) {
		return fmt.Errorf("write prepared transaction %d: %w", order, writeErr)
	}
	journal.prepared[order] = record
	if writeErr != nil {
		return fmt.Errorf("write prepared transaction %d: %w", order, writeErr)
	}
	return nil
}

func (journal *ReceiptJournal) RecordBroadcast(order int, transactionHash string) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	prepared, exists := journal.prepared[order]
	if !exists {
		return fmt.Errorf("transaction %d must be prepared before broadcast", order)
	}
	if _, exists := journal.broadcast[order]; exists {
		return fmt.Errorf("transaction %d already has a broadcast record", order)
	}
	record := BroadcastJournalRecord{
		Schema: BroadcastJournalRecordSchema, PlanSHA256: journal.header.PlanSHA256,
		ManifestSHA256: journal.header.ManifestSHA256, Order: order,
		TransactionHash: strings.ToLower(strings.TrimSpace(transactionHash)),
	}
	if err := journal.validateBroadcast(record, prepared); err != nil {
		return err
	}
	writeErr := writeExclusiveJournalJSONWithSync(
		journal.recordPath(order, "broadcast"), record, journal.syncDirectory,
	)
	if writeErr != nil && !journalTransitionPublished(writeErr) {
		return fmt.Errorf("write broadcast transaction %d: %w", order, writeErr)
	}
	journal.broadcast[order] = record
	if writeErr != nil {
		return fmt.Errorf("write broadcast transaction %d: %w", order, writeErr)
	}
	return nil
}

func (journal *ReceiptJournal) RecordMined(order int, receipt *chain.EVMReceipt) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	broadcast, exists := journal.broadcast[order]
	if !exists {
		return fmt.Errorf("transaction %d must be broadcast before it is recorded as mined", order)
	}
	if _, exists := journal.mined[order]; exists {
		return fmt.Errorf("transaction %d already has a mined record", order)
	}
	if _, exists := journal.reverted[order]; exists {
		return fmt.Errorf("transaction %d already has a reverted record", order)
	}
	canonical, err := canonicalJournalReceipt(receipt)
	if err != nil {
		return fmt.Errorf("canonicalize transaction %d receipt: %w", order, err)
	}
	record := MinedJournalRecord{
		Schema: MinedJournalRecordSchema, PlanSHA256: journal.header.PlanSHA256,
		ManifestSHA256: journal.header.ManifestSHA256, Order: order, Receipt: *canonical,
	}
	if err := journal.validateMined(record, broadcast); err != nil {
		return err
	}
	writeErr := writeExclusiveJournalJSONWithSync(
		journal.recordPath(order, "mined"), record, journal.syncDirectory,
	)
	if writeErr != nil && !journalTransitionPublished(writeErr) {
		return fmt.Errorf("write mined transaction %d: %w", order, writeErr)
	}
	journal.mined[order] = record
	if writeErr != nil {
		return fmt.Errorf("write mined transaction %d: %w", order, writeErr)
	}
	return nil
}

// RecordReverted writes immutable failure evidence after a mined status=0x0
// receipt has been checked against the manifest. It is deliberately separate
// from mined success so status tools cannot mistake a failed transaction for
// completed import progress. This offline method does not contact RPC; callers
// that have a canonical block must use RecordRevertedAt.
func (journal *ReceiptJournal) RecordReverted(order int, receipt *chain.EVMReceipt) error {
	canonical, err := canonicalRevertedJournalReceipt(receipt)
	if err != nil {
		return fmt.Errorf("canonicalize transaction %d reverted receipt: %w", order, err)
	}
	return journal.recordRevertedCanonical(order, canonical)
}

// RecordRevertedAt binds failure evidence to the block object returned by the
// same RPC snapshot check used by the runner. It is the preferred method for
// any caller that has chain access.
func (journal *ReceiptJournal) RecordRevertedAt(
	order int,
	receipt *chain.EVMReceipt,
	block *chain.EVMBlock,
) error {
	canonical, err := canonicalRevertedJournalReceipt(receipt)
	if err != nil {
		return fmt.Errorf("canonicalize transaction %d reverted receipt: %w", order, err)
	}
	if err := validateCanonicalJournalBlock(canonical, block); err != nil {
		return fmt.Errorf("validate transaction %d reverted receipt block: %w", order, err)
	}
	return journal.recordRevertedCanonical(order, canonical)
}

func (journal *ReceiptJournal) recordRevertedCanonical(order int, canonical *chain.EVMReceipt) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	broadcast, exists := journal.broadcast[order]
	if !exists {
		return fmt.Errorf("transaction %d must be broadcast before it is recorded as reverted", order)
	}
	if _, exists := journal.mined[order]; exists {
		return fmt.Errorf("transaction %d already has a mined record", order)
	}
	if _, exists := journal.reverted[order]; exists {
		return fmt.Errorf("transaction %d already has a reverted record", order)
	}
	record := RevertedJournalRecord{
		Schema: RevertedJournalRecordSchema, PlanSHA256: journal.header.PlanSHA256,
		ManifestSHA256: journal.header.ManifestSHA256, Order: order, Receipt: *canonical,
	}
	if err := journal.validateReverted(record, broadcast); err != nil {
		return err
	}
	writeErr := writeExclusiveJournalJSONWithSync(
		journal.recordPath(order, "reverted"), record, journal.syncDirectory,
	)
	if writeErr != nil && !journalTransitionPublished(writeErr) {
		return fmt.Errorf("write reverted transaction %d: %w", order, writeErr)
	}
	journal.reverted[order] = record
	if writeErr != nil {
		return fmt.Errorf("write reverted transaction %d: %w", order, writeErr)
	}
	return nil
}

func journalTransitionPublished(err error) bool {
	var published *JournalPublishedUnsyncedError
	return errors.As(err, &published)
}

func (journal *ReceiptJournal) requireNextOrder(order int) error {
	if order < 0 || order >= journal.header.TransactionCount {
		return fmt.Errorf("transaction order %d is outside 0..%d", order, journal.header.TransactionCount-1)
	}
	for previous := 0; previous < order; previous++ {
		if _, complete := journal.mined[previous]; !complete {
			if _, reverted := journal.reverted[previous]; reverted {
				return fmt.Errorf("transaction %d cannot be prepared after reverted transaction %d", order, previous)
			}
			return fmt.Errorf("transaction %d cannot be prepared before transaction %d is mined", order, previous)
		}
	}
	return nil
}

func (journal *ReceiptJournal) loadRecords() error {
	entries, err := os.ReadDir(journal.directory)
	if err != nil {
		return fmt.Errorf("read receipt journal directory: %w", err)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	for _, entry := range entries {
		name := entry.Name()
		if name == receiptJournalHeaderFile {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect receipt journal file %s: %w", name, err)
		}
		if strings.HasPrefix(name, receiptJournalTemporaryPrefix) && info.Mode().IsRegular() {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("receipt journal contains unexpected non-regular file %s", name)
		}
		if err := validateJournalPermissions(filepath.Join(journal.directory, name), info); err != nil {
			return err
		}
		order, state, ok := parseReceiptJournalRecordName(name)
		if !ok || order < 0 || order >= journal.header.TransactionCount {
			return fmt.Errorf("receipt journal contains unexpected file %s", name)
		}
		path := filepath.Join(journal.directory, name)
		switch state {
		case "prepared":
			var record PreparedJournalRecord
			if err := readStrictJournalJSON(path, &record); err != nil {
				return fmt.Errorf("read prepared transaction %d: %w", order, err)
			}
			if record.Order != order {
				return fmt.Errorf("prepared transaction file %s contains order %d", name, record.Order)
			}
			if err := journal.validatePrepared(record); err != nil {
				return err
			}
			journal.prepared[order] = record
		case "broadcast":
			var record BroadcastJournalRecord
			if err := readStrictJournalJSON(path, &record); err != nil {
				return fmt.Errorf("read broadcast transaction %d: %w", order, err)
			}
			if record.Order != order {
				return fmt.Errorf("broadcast transaction file %s contains order %d", name, record.Order)
			}
			journal.broadcast[order] = record
		case "mined":
			var record MinedJournalRecord
			if err := readStrictJournalJSON(path, &record); err != nil {
				return fmt.Errorf("read mined transaction %d: %w", order, err)
			}
			if record.Order != order {
				return fmt.Errorf("mined transaction file %s contains order %d", name, record.Order)
			}
			journal.mined[order] = record
		case "reverted":
			var record RevertedJournalRecord
			if err := readStrictJournalJSON(path, &record); err != nil {
				return fmt.Errorf("read reverted transaction %d: %w", order, err)
			}
			if record.Order != order {
				return fmt.Errorf("reverted transaction file %s contains order %d", name, record.Order)
			}
			if err := journal.validateReverted(record, journal.broadcast[order]); err != nil {
				return err
			}
			journal.reverted[order] = record
		}
	}
	for order := 0; order < journal.header.TransactionCount; order++ {
		prepared, hasPrepared := journal.prepared[order]
		broadcast, hasBroadcast := journal.broadcast[order]
		mined, hasMined := journal.mined[order]
		reverted, hasReverted := journal.reverted[order]
		if (hasBroadcast || hasMined || hasReverted) && !hasPrepared {
			return fmt.Errorf("transaction %d journal state exists without a prepared record", order)
		}
		if (hasMined || hasReverted) && !hasBroadcast {
			return fmt.Errorf("transaction %d terminal receipt record exists without a broadcast record", order)
		}
		if hasMined && hasReverted {
			return fmt.Errorf("transaction %d cannot have both mined and reverted records", order)
		}
		if hasPrepared {
			if err := journal.requireNextOrder(order); err != nil {
				return err
			}
		}
		if hasBroadcast {
			if err := journal.validateBroadcast(broadcast, prepared); err != nil {
				return err
			}
		}
		if hasMined {
			if err := journal.validateMined(mined, broadcast); err != nil {
				return err
			}
		}
		if hasReverted {
			if err := journal.validateReverted(reverted, broadcast); err != nil {
				return err
			}
		}
	}
	return nil
}

func (journal *ReceiptJournal) validatePrepared(record PreparedJournalRecord) error {
	if record.Schema != PreparedJournalRecordSchema ||
		record.PlanSHA256 != journal.header.PlanSHA256 ||
		record.ManifestSHA256 != journal.header.ManifestSHA256 {
		return fmt.Errorf("prepared transaction %d is not bound to this receipt journal", record.Order)
	}
	if record.Order < 0 || record.Order >= len(journal.manifest.Transactions) {
		return fmt.Errorf("prepared transaction order %d is out of range", record.Order)
	}
	return validateSignedImportTransaction(journal.header, journal.manifest.Transactions[record.Order], record.Transaction)
}

func (journal *ReceiptJournal) validateBroadcast(record BroadcastJournalRecord, prepared PreparedJournalRecord) error {
	if record.Schema != BroadcastJournalRecordSchema ||
		record.PlanSHA256 != journal.header.PlanSHA256 ||
		record.ManifestSHA256 != journal.header.ManifestSHA256 || record.Order != prepared.Order {
		return fmt.Errorf("broadcast transaction %d is not bound to its prepared record", record.Order)
	}
	if record.TransactionHash != prepared.Transaction.TransactionHash {
		return fmt.Errorf("broadcast transaction %d hash does not match its prepared transaction", record.Order)
	}
	return nil
}

func (journal *ReceiptJournal) validateMined(record MinedJournalRecord, broadcast BroadcastJournalRecord) error {
	if record.Schema != MinedJournalRecordSchema ||
		record.PlanSHA256 != journal.header.PlanSHA256 ||
		record.ManifestSHA256 != journal.header.ManifestSHA256 || record.Order != broadcast.Order {
		return fmt.Errorf("mined transaction %d is not bound to its broadcast record", record.Order)
	}
	canonical, err := canonicalJournalReceipt(&record.Receipt)
	if err != nil {
		return fmt.Errorf("mined transaction %d receipt is invalid: %w", record.Order, err)
	}
	if !reflect.DeepEqual(*canonical, record.Receipt) {
		return fmt.Errorf("mined transaction %d receipt is not canonical", record.Order)
	}
	return validateManifestReceipt(journal.plan, journal.manifest, record.Order, broadcast.TransactionHash, &record.Receipt)
}

func (journal *ReceiptJournal) validateReverted(record RevertedJournalRecord, broadcast BroadcastJournalRecord) error {
	if record.Schema != RevertedJournalRecordSchema ||
		record.PlanSHA256 != journal.header.PlanSHA256 ||
		record.ManifestSHA256 != journal.header.ManifestSHA256 || record.Order != broadcast.Order {
		return fmt.Errorf("reverted transaction %d is not bound to its broadcast record", record.Order)
	}
	canonical, err := canonicalRevertedJournalReceipt(&record.Receipt)
	if err != nil {
		return fmt.Errorf("reverted transaction %d receipt is invalid: %w", record.Order, err)
	}
	if !reflect.DeepEqual(*canonical, record.Receipt) {
		return fmt.Errorf("reverted transaction %d receipt is not canonical", record.Order)
	}
	if record.Receipt.TransactionHash != broadcast.TransactionHash {
		return fmt.Errorf("reverted transaction %d hash does not match its broadcast record", record.Order)
	}
	if record.Order < 0 || record.Order >= len(journal.manifest.Transactions) {
		return fmt.Errorf("reverted transaction order %d is out of range", record.Order)
	}
	expectedTarget, err := chain.NormalizeEVMAddress(journal.manifest.Transactions[record.Order].To)
	if err != nil {
		return fmt.Errorf("reverted transaction %d manifest target: %w", record.Order, err)
	}
	if record.Receipt.To != expectedTarget {
		return fmt.Errorf("reverted transaction %d target %s does not match manifest %s", record.Order, record.Receipt.To, expectedTarget)
	}
	return nil
}

func validateSignedImportTransaction(
	header ReceiptJournalHeader,
	manifestTransaction ManifestTransaction,
	record SignedImportTransaction,
) error {
	from, err := chain.NormalizeEVMAddress(record.From)
	if err != nil || from != header.Signer || record.From != from {
		return errors.New("signed import transaction sender does not match the journal signer")
	}
	if record.ChainID != header.ChainID {
		return errors.New("signed import transaction chain ID does not match the journal")
	}
	if err := validateCanonicalHash32("signed transaction hash", record.TransactionHash); err != nil {
		return err
	}
	raw, err := decodeCanonicalJournalHex("raw transaction", record.RawTransaction, false)
	if err != nil {
		return err
	}
	var transaction types.Transaction
	if err := transaction.UnmarshalBinary(raw); err != nil {
		return fmt.Errorf("decode signed import transaction: %w", err)
	}
	if transaction.Hash().Hex() != record.TransactionHash {
		return errors.New("signed import transaction hash does not match its raw bytes")
	}
	chainID := transaction.ChainId()
	if chainID == nil || !chainID.IsUint64() || chainID.Uint64() != header.ChainID {
		return errors.New("signed raw transaction chain ID does not match the journal")
	}
	signer := types.LatestSignerForChainID(new(big.Int).SetUint64(header.ChainID))
	sender, err := types.Sender(signer, &transaction)
	if err != nil || strings.ToLower(sender.Hex()) != header.Signer {
		return errors.New("signed raw transaction does not recover to the journal signer")
	}
	if transaction.Nonce() != record.Nonce || transaction.Gas() != record.GasLimit {
		return errors.New("signed raw transaction nonce or gas limit does not match its journal record")
	}
	if record.GasLimit == 0 {
		return errors.New("signed import transaction gas limit must be positive")
	}
	gasPrice, err := parseJournalQuantity("gas price", record.GasPrice)
	if err != nil || transaction.GasPrice().Cmp(gasPrice) != 0 {
		return errors.New("signed raw transaction gas price does not match its journal record")
	}
	value, err := parseJournalQuantity("value", record.Value)
	if err != nil || transaction.Value().Cmp(value) != 0 {
		return errors.New("signed raw transaction value does not match its journal record")
	}
	manifestValue, err := parseJournalQuantity("manifest value", manifestTransaction.Value)
	if err != nil || transaction.Value().Cmp(manifestValue) != 0 {
		return errors.New("signed raw transaction value does not match the transaction manifest")
	}
	if transaction.To() == nil {
		return errors.New("signed import transaction cannot create a contract")
	}
	target, err := chain.NormalizeEVMAddress(manifestTransaction.To)
	if err != nil || strings.ToLower(transaction.To().Hex()) != target {
		return errors.New("signed raw transaction target does not match the transaction manifest")
	}
	data, err := decodeCanonicalJournalHex("manifest calldata", manifestTransaction.Data, true)
	if err != nil || !bytes.Equal(transaction.Data(), data) {
		return errors.New("signed raw transaction calldata does not match the transaction manifest")
	}
	return nil
}

func validateManifestReceipt(
	plan *Plan,
	manifest *TransactionManifest,
	order int,
	transactionHash string,
	receipt *chain.EVMReceipt,
) error {
	if receipt == nil {
		return errors.New("mined transaction receipt is nil")
	}
	if order < 0 || order >= len(manifest.Transactions) {
		return fmt.Errorf("receipt transaction order %d is out of range", order)
	}
	transaction := manifest.Transactions[order]
	if receipt.TransactionHash != transactionHash {
		return fmt.Errorf("transaction %d receipt hash does not match its broadcast hash", order)
	}
	if receipt.Status != "0x1" {
		return fmt.Errorf("transaction %d receipt status %q is not 0x1", order, receipt.Status)
	}
	target, err := chain.NormalizeEVMAddress(receipt.To)
	if err != nil || target != transaction.To {
		return fmt.Errorf("transaction %d receipt target does not match the manifest", order)
	}
	if _, err := canonicalReceiptBlockTag(receipt.BlockNumber); err != nil {
		return fmt.Errorf("transaction %d receipt block number: %w", order, err)
	}
	if err := validateCanonicalHash32("receipt block hash", receipt.BlockHash); err != nil {
		return fmt.Errorf("transaction %d: %w", order, err)
	}
	if transaction.Phase == "finalize" {
		return validateFinalizationLog(plan, receipt.Logs, receipt.TransactionHash, receipt.BlockNumber, receipt.BlockHash)
	}
	for _, log := range receipt.Logs {
		if journalLogMatchesManifest(plan, manifest, transaction, receipt, log) {
			return nil
		}
	}
	return fmt.Errorf("transaction %d receipt does not contain %s from %s", order, transaction.ExpectedEvent, transaction.EventEmitter)
}

func journalLogMatchesManifest(
	plan *Plan,
	manifest *TransactionManifest,
	transaction ManifestTransaction,
	receipt *chain.EVMReceipt,
	log chain.EVMLog,
) bool {
	if log.Removed || len(log.Topics) < 2 || log.Address != transaction.EventEmitter ||
		log.Topics[0] != transaction.ExpectedTopic || log.Topics[1] != manifest.SessionID ||
		log.TransactionHash != receipt.TransactionHash || log.BlockNumber != receipt.BlockNumber ||
		log.BlockHash != receipt.BlockHash {
		return false
	}
	switch transaction.Phase {
	case "create_session":
		if len(log.Topics) != 4 || log.Topics[2] != manifest.SessionID {
			return false
		}
		if plan.Target.Controller != "" {
			return log.Topics[3] == manifest.ImportCommitment
		}
		sourceContract := "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(plan.Snapshot.ContractEVM, "0x")
		return log.Topics[3] == sourceContract
	case "import_repo", "import_refs", "import_collaborators":
		if transaction.Sequence == nil || len(log.Topics) != 4 ||
			log.Topics[2] != uint256Topic(uint64(*transaction.Sequence)) || log.Topics[3] != transaction.RepoID {
			return false
		}
		data, err := decodeCanonicalJournalHex("import event data", log.Data, true)
		if err != nil || len(data) < 32 {
			return false
		}
		return "0x"+hex.EncodeToString(data[:32]) == "0x"+transaction.PayloadSHA256
	default:
		return false
	}
}

func canonicalJournalReceipt(receipt *chain.EVMReceipt) (*chain.EVMReceipt, error) {
	return canonicalJournalReceiptWithStatus(receipt, false)
}

func canonicalRevertedJournalReceipt(receipt *chain.EVMReceipt) (*chain.EVMReceipt, error) {
	canonical, err := canonicalJournalReceiptWithStatus(receipt, true)
	if err != nil {
		return nil, err
	}
	if canonical.Status != "0x0" {
		return nil, fmt.Errorf("receipt status %q is not 0x0", canonical.Status)
	}
	return canonical, nil
}

func canonicalJournalReceiptWithStatus(receipt *chain.EVMReceipt, allowReverted bool) (*chain.EVMReceipt, error) {
	if receipt == nil {
		return nil, errors.New("receipt is nil")
	}
	canonical := cloneEVMReceipt(*receipt)
	canonical.TransactionHash = strings.ToLower(strings.TrimSpace(canonical.TransactionHash))
	if err := validateCanonicalHash32("receipt transaction hash", canonical.TransactionHash); err != nil {
		return nil, err
	}
	blockTag, err := canonicalReceiptBlockTag(canonical.BlockNumber)
	if err != nil {
		return nil, err
	}
	canonical.BlockNumber = blockTag
	canonical.BlockHash = strings.ToLower(strings.TrimSpace(canonical.BlockHash))
	if err := validateCanonicalHash32("receipt block hash", canonical.BlockHash); err != nil {
		return nil, err
	}
	canonical.To, err = chain.NormalizeEVMAddress(canonical.To)
	if err != nil {
		return nil, fmt.Errorf("receipt target: %w", err)
	}
	canonical.Status = strings.ToLower(strings.TrimSpace(canonical.Status))
	if canonical.Status == "0" {
		canonical.Status = "0x0"
	}
	if canonical.Status != "0x1" && (!allowReverted || canonical.Status != "0x0") {
		return nil, fmt.Errorf("receipt status %q is not an accepted terminal status", canonical.Status)
	}
	gasUsed, err := canonicalJournalQuantity("receipt gas used", canonical.GasUsed)
	if err != nil {
		return nil, err
	}
	canonical.GasUsed = gasUsed
	for index := range canonical.Logs {
		log := &canonical.Logs[index]
		log.Address, err = chain.NormalizeEVMAddress(log.Address)
		if err != nil {
			return nil, fmt.Errorf("receipt log %d address: %w", index, err)
		}
		for topicIndex := range log.Topics {
			log.Topics[topicIndex] = strings.ToLower(strings.TrimSpace(log.Topics[topicIndex]))
			if err := validateCanonicalJournalWord("receipt log topic", log.Topics[topicIndex]); err != nil {
				return nil, fmt.Errorf("receipt log %d topic %d: %w", index, topicIndex, err)
			}
		}
		data := strings.ToLower(strings.TrimSpace(log.Data))
		if data == "" {
			data = "0x"
		}
		if _, err := decodeCanonicalJournalHex("receipt log data", data, true); err != nil {
			return nil, fmt.Errorf("receipt log %d: %w", index, err)
		}
		log.Data = data
		log.BlockNumber, err = canonicalReceiptBlockTag(log.BlockNumber)
		if err != nil {
			return nil, fmt.Errorf("receipt log %d block number: %w", index, err)
		}
		log.BlockHash = strings.ToLower(strings.TrimSpace(log.BlockHash))
		if err := validateCanonicalHash32("receipt log block hash", log.BlockHash); err != nil {
			return nil, fmt.Errorf("receipt log %d: %w", index, err)
		}
		log.TransactionHash = strings.ToLower(strings.TrimSpace(log.TransactionHash))
		if err := validateCanonicalHash32("receipt log transaction hash", log.TransactionHash); err != nil {
			return nil, fmt.Errorf("receipt log %d: %w", index, err)
		}
		if log.BlockNumber != canonical.BlockNumber || log.BlockHash != canonical.BlockHash || log.TransactionHash != canonical.TransactionHash {
			return nil, fmt.Errorf("receipt log %d does not match the receipt block or transaction", index)
		}
	}
	return &canonical, nil
}

func cloneEVMReceipt(receipt chain.EVMReceipt) chain.EVMReceipt {
	cloned := receipt
	cloned.Logs = append([]chain.EVMLog(nil), receipt.Logs...)
	for index := range cloned.Logs {
		cloned.Logs[index].Topics = append([]string(nil), receipt.Logs[index].Topics...)
	}
	return cloned
}

func validateCanonicalJournalBlock(receipt *chain.EVMReceipt, block *chain.EVMBlock) error {
	if receipt == nil {
		return errors.New("receipt is nil")
	}
	if block == nil {
		return errors.New("canonical block is nil")
	}
	blockNumber, err := canonicalReceiptBlockTag(block.Number)
	if err != nil {
		return fmt.Errorf("canonical block number: %w", err)
	}
	if blockNumber != receipt.BlockNumber {
		return fmt.Errorf("canonical block number %q does not match receipt %s", block.Number, receipt.BlockNumber)
	}
	blockHash := strings.ToLower(strings.TrimSpace(block.Hash))
	if err := validateCanonicalHash32("receipt canonical block hash", blockHash); err != nil {
		return err
	}
	if blockHash != receipt.BlockHash {
		return fmt.Errorf("canonical hash %s does not match receipt %s", blockHash, receipt.BlockHash)
	}
	return nil
}

func prepareReceiptJournalDirectory(directory string) error {
	return prepareReceiptJournalDirectoryWithSync(directory, syncJournalDirectory)
}

func prepareReceiptJournalDirectoryWithSync(directory string, syncDirectory func(string) error) error {
	missing, err := missingReceiptJournalDirectories(directory)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		info, err := os.Lstat(directory)
		if err != nil {
			return fmt.Errorf("inspect receipt journal directory: %w", err)
		}
		return validateJournalPermissions(directory, info)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create receipt journal directory: %w", err)
	}
	// MkdirAll honors the process umask but does not repair an existing mode
	// when a race creates the final component between the Lstat and mkdir.
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("restrict receipt journal directory: %w", err)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if syncDirectory == nil {
		syncDirectory = syncJournalDirectory
	}
	// MkdirAll can create several missing ancestors. Sync each newly created
	// directory's parent from the deepest level upward so a crash cannot leave
	// the journal path itself unreachable despite a successful file fsync.
	for _, current := range missing {
		parent := filepath.Dir(current)
		if err := syncDirectory(parent); err != nil {
			return fmt.Errorf("sync receipt journal parent directory %s: %w", parent, err)
		}
	}
	return nil
}

// missingReceiptJournalDirectories returns missing path components from the
// requested directory upward, deepest first. Existing ancestors are checked
// with Lstat so symlinks cannot be traversed accidentally.
func missingReceiptJournalDirectories(directory string) ([]string, error) {
	var missing []string
	requested := filepath.Clean(directory)
	for current := requested; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return nil, fmt.Errorf("receipt journal path is not a real directory: %s", current)
			}
			if current == requested {
				if err := validateJournalPermissions(current, info); err != nil {
					return nil, err
				}
			}
			return missing, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect receipt journal directory %s: %w", current, err)
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return missing, nil
		}
	}
}

func validateJournalPermissions(path string, info os.FileInfo) error {
	// Windows ACLs are not represented faithfully by os.FileMode. The release
	// runbook requires a user-owned restricted directory there; POSIX modes can
	// be checked exactly and must not expose broadcastable raw transactions.
	if runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("receipt journal path %s is accessible by group or others (mode %04o)", path, info.Mode().Perm())
	}
	return nil
}

func requireJournalDirectoryEmpty(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), receiptJournalTemporaryPrefix) {
			continue
		}
		return fmt.Errorf("receipt journal directory is not empty and has no %s header", receiptJournalHeaderFile)
	}
	return nil
}

func (journal *ReceiptJournal) recordPath(order int, state string) string {
	return filepath.Join(journal.directory, fmt.Sprintf("%06d.%s.json", order, state))
}

func parseReceiptJournalRecordName(name string) (int, string, bool) {
	parts := strings.Split(name, ".")
	if len(parts) != 3 || len(parts[0]) != 6 || parts[2] != "json" {
		return 0, "", false
	}
	if parts[1] != "prepared" && parts[1] != "broadcast" && parts[1] != "mined" && parts[1] != "reverted" {
		return 0, "", false
	}
	order, err := strconv.Atoi(parts[0])
	if err != nil || fmt.Sprintf("%06d", order) != parts[0] {
		return 0, "", false
	}
	return order, parts[1], true
}

func writeExclusiveJournalJSON(path string, value any) error {
	return writeExclusiveJournalJSONWithSync(path, value, syncJournalDirectory)
}

func writeExclusiveJournalJSONWithSync(path string, value any, syncDirectory func(string) error) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, receiptJournalTemporaryPrefix)
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryName, path); err != nil {
		return err
	}
	if syncDirectory == nil {
		syncDirectory = syncJournalDirectory
	}
	if err := syncDirectory(directory); err != nil {
		return &JournalPublishedUnsyncedError{Path: path, Err: err}
	}
	return nil
}

// syncJournalDirectory closes the durability gap between writing a temporary
// file and linking its final no-clobber name. POSIX filesystems support
// fsync-style directory synchronization. Windows has no portable directory
// handle Sync contract; file Sync plus the atomic hard link remains the
// strongest cross-platform guarantee and is called out in the operator docs.
func syncJournalDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer directoryHandle.Close()
	return directoryHandle.Sync()
}

func readStrictJournalJSON(path string, output any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("file contains trailing JSON data")
		}
		return err
	}
	return nil
}

func decodeCanonicalJournalHex(label, value string, allowEmpty bool) ([]byte, error) {
	if value != strings.ToLower(strings.TrimSpace(value)) || !strings.HasPrefix(value, "0x") {
		return nil, fmt.Errorf("%s must be canonical lowercase 0x-prefixed hex", label)
	}
	digits := value[2:]
	if digits == "" && !allowEmpty {
		return nil, fmt.Errorf("%s must not be empty", label)
	}
	if len(digits)%2 != 0 {
		return nil, fmt.Errorf("%s must contain complete bytes", label)
	}
	decoded, err := hex.DecodeString(digits)
	if err != nil {
		return nil, fmt.Errorf("%s contains invalid hex: %w", label, err)
	}
	return decoded, nil
}

func validateCanonicalJournalWord(label, value string) error {
	decoded, err := decodeCanonicalJournalHex(label, value, false)
	if err != nil {
		return err
	}
	if len(decoded) != 32 {
		return fmt.Errorf("%s must contain exactly 32 bytes", label)
	}
	return nil
}

func parseJournalQuantity(label, value string) (*big.Int, error) {
	canonical, err := canonicalJournalQuantity(label, value)
	if err != nil {
		return nil, err
	}
	parsed, ok := new(big.Int).SetString(canonical[2:], 16)
	if !ok {
		return nil, fmt.Errorf("%s is invalid", label)
	}
	return parsed, nil
}

func canonicalJournalQuantity(label, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if value != trimmed || trimmed != strings.ToLower(trimmed) {
		return "", fmt.Errorf("%s must be a canonical lowercase JSON-RPC quantity", label)
	}
	value = trimmed
	if !strings.HasPrefix(value, "0x") || len(value) == 2 {
		return "", fmt.Errorf("%s must be a hexadecimal JSON-RPC quantity", label)
	}
	digits := value[2:]
	if len(digits) > 1 && digits[0] == '0' {
		return "", fmt.Errorf("%s must use canonical JSON-RPC quantity encoding", label)
	}
	if _, ok := new(big.Int).SetString(digits, 16); !ok {
		return "", fmt.Errorf("%s is not hexadecimal", label)
	}
	return value, nil
}

func uint256Topic(value uint64) string {
	return fmt.Sprintf("0x%064x", value)
}

const zeroEVMAddress = "0x0000000000000000000000000000000000000000"
