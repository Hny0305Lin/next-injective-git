package archivev1

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	InventorySchema = "igit.cosmwasm-v1.inventory.v2"
	SnapshotSchema  = "igit.cosmwasm-v1.snapshot.v1"
)

type Inventory struct {
	Schema           string                `json:"schema"`
	ChainID          string                `json:"chain_id"`
	Contract         string                `json:"contract"`
	CutoverHeight    uint64                `json:"cutover_height"`
	CutoverBlockHash string                `json:"cutover_block_hash"`
	TxSearchQuery    string                `json:"tx_search_query"`
	SourceSHA256     string                `json:"tx_search_sha256"`
	BlockSHA256      string                `json:"block_evidence_sha256"`
	EventCommitment  string                `json:"event_commitment_sha256"`
	ScannedTxs       uint64                `json:"scanned_transactions"`
	SuccessfulTxs    uint64                `json:"successful_transactions"`
	Blocks           []InventoryBlock      `json:"blocks"`
	Repositories     []InventoryRepository `json:"repositories"`
	Owners           []string              `json:"owners"`
	ReportIDs        []uint64              `json:"report_ids"`
	ModerationTrail  []InventoryModeration `json:"moderation_trail"`
	Usernames        []InventoryUsername   `json:"usernames"`
	BadgeRecipients  []string              `json:"badge_recipients"`
	ReleaseVersions  []string              `json:"release_versions"`
}

type InventoryBlock struct {
	Height    uint64 `json:"height"`
	Hash      string `json:"hash"`
	Timestamp uint64 `json:"timestamp"`
}

type InventoryRepository struct {
	Identity       string   `json:"identity"`
	Owner          string   `json:"owner"`
	Name           string   `json:"name"`
	Aliases        []string `json:"aliases"`
	ForkedFrom     string   `json:"forked_from,omitempty"`
	CreatedTxHash  string   `json:"created_tx_hash"`
	CreatedHeight  uint64   `json:"created_height"`
	CreatedTxIndex uint32   `json:"created_tx_index"`
}

type InventoryUsername struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

type InventoryModeration struct {
	ReportID   uint64 `json:"report_id,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Action     string `json:"action"`
	Actor      string `json:"actor"`
	Status     string `json:"status,omitempty"`
	ReasonHash string `json:"reason_hash,omitempty"`
	Timestamp  uint64 `json:"timestamp"`
	Height     uint64 `json:"height"`
	TxIndex    uint32 `json:"tx_index"`
	EventIndex uint32 `json:"event_index"`
}

type txSearchEnvelope struct {
	Error  json.RawMessage `json:"error"`
	Result *txSearchResult `json:"result"`
}

type txSearchResult struct {
	Query      string          `json:"query"`
	TotalCount json.RawMessage `json:"total_count"`
	Txs        []txSearchTx    `json:"txs"`
}

type txSearchTx struct {
	Hash     string              `json:"hash"`
	Height   json.RawMessage     `json:"height"`
	Index    json.RawMessage     `json:"index"`
	Tx       string              `json:"tx"`
	TxResult *txSearchResultData `json:"tx_result"`
}

type txSearchResultData struct {
	Code   *uint32     `json:"code"`
	Events []abciEvent `json:"events"`
}

type abciEvent struct {
	Type       string          `json:"type"`
	Attributes []abciAttribute `json:"attributes"`
}

type abciAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type orderedTx struct {
	txSearchTx
	height uint64
	index  uint32
}

type blockEvidenceEnvelope struct {
	Error  json.RawMessage      `json:"error"`
	Result *blockEvidenceResult `json:"result"`
}

type blockEvidenceResult struct {
	BlockID struct {
		Hash string `json:"hash"`
	} `json:"block_id"`
	Block struct {
		Header struct {
			ChainID string          `json:"chain_id"`
			Height  json.RawMessage `json:"height"`
			Time    string          `json:"time"`
		} `json:"header"`
		Data struct {
			Txs []string `json:"txs"`
		} `json:"data"`
	} `json:"block"`
}

type sourceBlock struct {
	chainID   string
	height    uint64
	hash      string
	timestamp uint64
	txHashes  []string
}

// BlockEvidence is a parsed collection of CometBFT /block responses. The raw
// document digest is retained so the generated inventory is bound to the exact
// operator-reviewed evidence file.
type BlockEvidence struct {
	sha256 string
	blocks map[uint64]sourceBlock
}

type repositoryState struct {
	record InventoryRepository
}

type inventoryBuilder struct {
	contract       string
	repositories   map[string]*repositoryState
	locators       map[string]string
	reports        map[uint64]struct{}
	usernameByName map[string]string
	usernameByUser map[string]string
	recipients     map[string]struct{}
	releases       map[string]struct{}
	moderation     []InventoryModeration
	blocks         map[uint64]sourceBlock
	commitment     [32]byte
	successfulTxs  uint64
}

// BuildInventory reconstructs every enumerable V1 key from complete CometBFT
// tx_search pages and proves each returned transaction against saved /block
// responses. Both evidence documents are required and bound into the result.
func BuildInventory(raw []byte, chainID, contract string, cutoverHeight uint64, evidence *BlockEvidence) (*Inventory, error) {
	chainID = strings.TrimSpace(chainID)
	contract = strings.TrimSpace(contract)
	if chainID == "" || contract == "" || cutoverHeight == 0 {
		return nil, errors.New("chain ID, contract, and positive cutover height are required")
	}
	if strings.ContainsAny(contract, "'\" \\\t\r\n") {
		return nil, errors.New("contract address cannot be represented safely in a tx_search query")
	}
	if evidence == nil || len(evidence.blocks) == 0 || evidence.sha256 == "" {
		return nil, errors.New("complete CometBFT block evidence is required")
	}
	transactions, totalCount, query, err := decodeTxSearch(raw, contract, cutoverHeight)
	if err != nil {
		return nil, err
	}
	if uint64(len(transactions)) != totalCount {
		return nil, fmt.Errorf("tx_search decoded %d transactions but total_count is %d", len(transactions), totalCount)
	}
	sort.Slice(transactions, func(i, j int) bool {
		if transactions[i].height != transactions[j].height {
			return transactions[i].height < transactions[j].height
		}
		if transactions[i].index != transactions[j].index {
			return transactions[i].index < transactions[j].index
		}
		return transactions[i].Hash < transactions[j].Hash
	})
	seenPosition := make(map[string]string, len(transactions))
	seenHash := make(map[string]struct{}, len(transactions))
	requiredBlocks := map[uint64]sourceBlock{}
	cutoverBlock, exists := evidence.blocks[cutoverHeight]
	if !exists {
		return nil, fmt.Errorf("block evidence is missing cutover height %d", cutoverHeight)
	}
	if cutoverBlock.chainID != chainID {
		return nil, fmt.Errorf("cutover block chain ID %q does not match %q", cutoverBlock.chainID, chainID)
	}
	requiredBlocks[cutoverHeight] = cutoverBlock
	builder := inventoryBuilder{
		contract: contract, repositories: map[string]*repositoryState{}, locators: map[string]string{},
		reports: map[uint64]struct{}{}, usernameByName: map[string]string{}, usernameByUser: map[string]string{},
		recipients: map[string]struct{}{}, releases: map[string]struct{}{}, blocks: evidence.blocks,
	}
	for _, transaction := range transactions {
		if transaction.height > cutoverHeight {
			return nil, fmt.Errorf("transaction %s height %d exceeds cutover height %d", transaction.Hash, transaction.height, cutoverHeight)
		}
		position := fmt.Sprintf("%d/%d", transaction.height, transaction.index)
		if previous, exists := seenPosition[position]; exists {
			return nil, fmt.Errorf("duplicate transaction position %s (%s and %s)", position, previous, transaction.Hash)
		}
		seenPosition[position] = transaction.Hash
		if _, exists := seenHash[transaction.Hash]; exists {
			return nil, fmt.Errorf("duplicate transaction hash %s", transaction.Hash)
		}
		seenHash[transaction.Hash] = struct{}{}
		block, exists := evidence.blocks[transaction.height]
		if !exists {
			return nil, fmt.Errorf("block evidence is missing transaction height %d", transaction.height)
		}
		if block.chainID != chainID {
			return nil, fmt.Errorf("block %d chain ID %q does not match %q", transaction.height, block.chainID, chainID)
		}
		if uint64(transaction.index) >= uint64(len(block.txHashes)) || block.txHashes[transaction.index] != transaction.Hash {
			return nil, fmt.Errorf("transaction %s is not at block %d index %d", transaction.Hash, transaction.height, transaction.index)
		}
		requiredBlocks[transaction.height] = block
		if *transaction.TxResult.Code != 0 {
			continue
		}
		matched := false
		sender := transactionSender(transaction.TxResult.Events)
		for eventIndex, event := range transaction.TxResult.Events {
			attributes := eventAttributes(event)
			if !isContractEvent(event.Type, attributes, contract) {
				continue
			}
			action := attributes["action"]
			if action == "" {
				continue
			}
			matched = true
			if err := builder.apply(transaction, eventIndex, action, sender, attributes); err != nil {
				return nil, fmt.Errorf("transaction %s event %d action %s: %w", transaction.Hash, eventIndex, action, err)
			}
		}
		if matched {
			builder.successfulTxs++
		} else {
			return nil, fmt.Errorf("successful transaction %s has no recognized target-contract action", transaction.Hash)
		}
	}
	if len(evidence.blocks) != len(requiredBlocks) {
		return nil, fmt.Errorf("block evidence contains %d responses but exactly %d transaction/cutover heights are required", len(evidence.blocks), len(requiredBlocks))
	}
	inventory := builder.inventory(chainID, contract, cutoverHeight)
	sourceDigest := sha256.Sum256(raw)
	inventory.SourceSHA256 = hex.EncodeToString(sourceDigest[:])
	inventory.BlockSHA256 = evidence.sha256
	inventory.TxSearchQuery = query
	inventory.ScannedTxs = totalCount
	inventory.CutoverBlockHash = cutoverBlock.hash
	heights := make([]uint64, 0, len(requiredBlocks))
	for height := range requiredBlocks {
		heights = append(heights, height)
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })
	for _, height := range heights {
		block := requiredBlocks[height]
		inventory.Blocks = append(inventory.Blocks, InventoryBlock{Height: height, Hash: block.hash, Timestamp: block.timestamp})
	}
	return inventory, nil
}

func transactionSender(events []abciEvent) string {
	for _, event := range events {
		if event.Type != "message" {
			continue
		}
		attributes := eventAttributes(event)
		if sender := attributes["sender"]; sender != "" {
			return sender
		}
	}
	return ""
}

func decodeTxSearch(raw []byte, contract string, cutoverHeight uint64) ([]orderedTx, uint64, string, error) {
	documents, err := decodeEvidenceDocuments(raw, "tx_search")
	if err != nil {
		return nil, 0, "", err
	}
	if len(documents) == 0 {
		return nil, 0, "", errors.New("tx_search evidence contains no pages")
	}
	var result []orderedTx
	var expectedCount uint64
	var expectedQuery string
	for pageIndex, document := range documents {
		var envelope txSearchEnvelope
		pageDecoder := json.NewDecoder(bytes.NewReader(document))
		if err := pageDecoder.Decode(&envelope); err != nil {
			return nil, 0, "", fmt.Errorf("decode tx_search page %d: %w", pageIndex, err)
		}
		if hasJSONValue(envelope.Error) {
			return nil, 0, "", fmt.Errorf("tx_search page %d contains an RPC error", pageIndex)
		}
		if envelope.Result == nil {
			return nil, 0, "", fmt.Errorf("tx_search page %d has no result", pageIndex)
		}
		query := strings.TrimSpace(envelope.Result.Query)
		if err := validateTxSearchQuery(query, contract, cutoverHeight); err != nil {
			return nil, 0, "", fmt.Errorf("tx_search page %d: %w", pageIndex, err)
		}
		count, err := rawUint64(envelope.Result.TotalCount)
		if err != nil {
			return nil, 0, "", fmt.Errorf("tx_search page %d has invalid total_count: %w", pageIndex, err)
		}
		if pageIndex == 0 {
			expectedCount, expectedQuery = count, query
		} else if count != expectedCount {
			return nil, 0, "", fmt.Errorf("tx_search page %d total_count %d does not match %d", pageIndex, count, expectedCount)
		} else if query != expectedQuery {
			return nil, 0, "", fmt.Errorf("tx_search page %d query does not match the first page", pageIndex)
		}
		for _, transaction := range envelope.Result.Txs {
			height, err := rawUint64(transaction.Height)
			if err != nil || height == 0 {
				return nil, 0, "", fmt.Errorf("transaction %s has invalid height: %w", transaction.Hash, err)
			}
			transaction.Hash = strings.ToUpper(strings.TrimSpace(transaction.Hash))
			if err := validateUpperHash(transaction.Hash); err != nil {
				return nil, 0, "", fmt.Errorf("tx_search transaction hash: %w", err)
			}
			if transaction.TxResult == nil {
				return nil, 0, "", fmt.Errorf("transaction %s has no tx_result", transaction.Hash)
			}
			if transaction.TxResult.Code == nil {
				return nil, 0, "", fmt.Errorf("transaction %s has no result code", transaction.Hash)
			}
			indexValue, err := rawUint64(transaction.Index)
			if err != nil || indexValue > uint64(^uint32(0)) {
				return nil, 0, "", fmt.Errorf("transaction %s has invalid index", transaction.Hash)
			}
			encodedTx, err := base64.StdEncoding.DecodeString(transaction.Tx)
			if err != nil || len(encodedTx) == 0 {
				return nil, 0, "", fmt.Errorf("transaction %s has invalid transaction bytes", transaction.Hash)
			}
			digest := sha256.Sum256(encodedTx)
			if hex.EncodeToString(digest[:]) != strings.ToLower(transaction.Hash) {
				return nil, 0, "", fmt.Errorf("transaction %s hash does not match transaction bytes", transaction.Hash)
			}
			result = append(result, orderedTx{txSearchTx: transaction, height: height, index: uint32(indexValue)})
		}
	}
	return result, expectedCount, expectedQuery, nil
}

func decodeEvidenceDocuments(raw []byte, label string) ([]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%s evidence is empty", label)
	}
	decode := func(target any) error {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if err := decoder.Decode(target); err != nil {
			return err
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			if err == nil {
				return fmt.Errorf("%s evidence contains trailing JSON data", label)
			}
			return fmt.Errorf("read %s evidence trailer: %w", label, err)
		}
		return nil
	}
	var documents []json.RawMessage
	if err := decode(&documents); err == nil {
		return documents, nil
	}
	var single json.RawMessage
	if err := decode(&single); err != nil {
		return nil, fmt.Errorf("decode CometBFT %s response: %w", label, err)
	}
	return []json.RawMessage{single}, nil
}

func hasJSONValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null"))
}

func expectedTxSearchQuery(contract string, cutoverHeight uint64) string {
	return fmt.Sprintf("wasm._contract_address='%s' AND tx.height<=%d", contract, cutoverHeight)
}

func validateTxSearchQuery(query, contract string, cutoverHeight uint64) error {
	expected := expectedTxSearchQuery(contract, cutoverHeight)
	if query != expected {
		return fmt.Errorf("query %q must exactly equal %q", query, expected)
	}
	return nil
}

func validateUpperHash(value string) error {
	if len(value) != sha256.Size*2 || strings.ToUpper(value) != value {
		return errors.New("must be an uppercase 32-byte hex hash")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("must be an uppercase 32-byte hex hash")
	}
	return nil
}

// ReadBlockEvidence parses saved CometBFT /block responses. Input may be one
// response object or an array, and duplicate heights are rejected even when
// their hashes happen to match.
func ReadBlockEvidence(path string) (*BlockEvidence, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("block evidence path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	documents, err := decodeEvidenceDocuments(raw, "block")
	if err != nil {
		return nil, err
	}
	if len(documents) == 0 {
		return nil, errors.New("block evidence contains no responses")
	}
	evidence := &BlockEvidence{blocks: make(map[uint64]sourceBlock, len(documents))}
	digest := sha256.Sum256(raw)
	evidence.sha256 = hex.EncodeToString(digest[:])
	for documentIndex, document := range documents {
		var envelope blockEvidenceEnvelope
		if err := json.Unmarshal(document, &envelope); err != nil {
			return nil, fmt.Errorf("decode block response %d: %w", documentIndex, err)
		}
		if hasJSONValue(envelope.Error) {
			return nil, fmt.Errorf("block response %d contains an RPC error", documentIndex)
		}
		if envelope.Result == nil {
			return nil, fmt.Errorf("block response %d has no result", documentIndex)
		}
		height, err := rawUint64(envelope.Result.Block.Header.Height)
		if err != nil || height == 0 {
			return nil, fmt.Errorf("block response %d has an invalid height", documentIndex)
		}
		if _, duplicate := evidence.blocks[height]; duplicate {
			return nil, fmt.Errorf("block evidence contains duplicate height %d", height)
		}
		chainID := strings.TrimSpace(envelope.Result.Block.Header.ChainID)
		if chainID == "" {
			return nil, fmt.Errorf("block %d has an empty chain ID", height)
		}
		blockHash := strings.ToUpper(strings.TrimSpace(envelope.Result.BlockID.Hash))
		if err := validateUpperHash(blockHash); err != nil {
			return nil, fmt.Errorf("block %d hash %w", height, err)
		}
		parsedTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(envelope.Result.Block.Header.Time))
		if err != nil || parsedTime.Unix() <= 0 {
			return nil, fmt.Errorf("block %d has an invalid timestamp", height)
		}
		block := sourceBlock{
			chainID: chainID, height: height, hash: strings.ToLower(blockHash), timestamp: uint64(parsedTime.Unix()),
			txHashes: make([]string, len(envelope.Result.Block.Data.Txs)),
		}
		for txIndex, encoded := range envelope.Result.Block.Data.Txs {
			txBytes, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || len(txBytes) == 0 {
				return nil, fmt.Errorf("block %d transaction %d is not valid base64 transaction bytes", height, txIndex)
			}
			txDigest := sha256.Sum256(txBytes)
			block.txHashes[txIndex] = strings.ToUpper(hex.EncodeToString(txDigest[:]))
		}
		evidence.blocks[height] = block
	}
	return evidence, nil
}

func rawUint64(raw json.RawMessage) (uint64, error) {
	value := strings.TrimSpace(string(raw))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func eventAttributes(event abciEvent) map[string]string {
	attributes := make(map[string]string, len(event.Attributes))
	for _, attribute := range event.Attributes {
		key, encoded := decodeAttributeKey(attribute.Key)
		value := attribute.Value
		if encoded {
			decoded, err := base64.StdEncoding.DecodeString(attribute.Value)
			if err == nil && utf8.Valid(decoded) {
				value = string(decoded)
			}
		}
		attributes[key] = value
	}
	return attributes
}

func decodeAttributeKey(value string) (string, bool) {
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) != 0 && utf8.Valid(decoded) {
		candidate := string(decoded)
		printable := true
		for _, character := range candidate {
			if character < 0x20 || character == 0x7f {
				printable = false
				break
			}
		}
		if printable {
			return candidate, true
		}
	}
	return value, false
}

func isContractEvent(eventType string, attributes map[string]string, contract string) bool {
	if eventType != "wasm" && !strings.HasPrefix(eventType, "wasm-") {
		return false
	}
	address := attributes["_contract_address"]
	if address == "" {
		address = attributes["contract_address"]
	}
	return address == contract
}

func (b *inventoryBuilder) apply(transaction orderedTx, eventIndex int, action, sender string, attributes map[string]string) error {
	canonical := canonicalEvent(transaction, eventIndex, action, attributes)
	b.commitment = sha256.Sum256(append(b.commitment[:], canonical...))
	switch action {
	case "create_repo", "fork_repo":
		owner, name := attributes["owner"], attributes["repo"]
		if owner == "" || name == "" {
			return errors.New("repository event is missing owner or repo")
		}
		locator := owner + "/" + name
		if _, exists := b.locators[locator]; exists {
			return fmt.Errorf("repository locator %s already exists", locator)
		}
		identity := strings.ToLower(transaction.Hash) + fmt.Sprintf(":%d", eventIndex)
		record := InventoryRepository{
			Identity: identity, Owner: owner, Name: name, Aliases: []string{locator},
			CreatedTxHash: strings.ToLower(transaction.Hash), CreatedHeight: transaction.height,
			CreatedTxIndex: transaction.index,
		}
		if action == "fork_repo" {
			source := attributes["source_owner"] + "/" + attributes["source_repo"]
			if source == "/" {
				return errors.New("fork event is missing source locator")
			}
			record.ForkedFrom = source
		}
		b.repositories[identity] = &repositoryState{record: record}
		b.locators[locator] = identity
	case "transfer_ownership":
		oldOwner, newOwner, name := attributes["old_owner"], attributes["new_owner"], attributes["repo"]
		oldLocator, newLocator := oldOwner+"/"+name, newOwner+"/"+name
		identity, exists := b.locators[oldLocator]
		if !exists {
			return fmt.Errorf("ownership transfer source %s is unknown", oldLocator)
		}
		if occupied, exists := b.locators[newLocator]; exists && occupied != identity {
			return fmt.Errorf("ownership transfer target %s is occupied", newLocator)
		}
		repository := b.repositories[identity]
		repository.record.Owner = newOwner
		repository.record.Aliases = appendUnique(repository.record.Aliases, newLocator)
		b.locators[newLocator] = identity
	case "submit_moderation_report":
		id, err := strconv.ParseUint(attributes["report_id"], 10, 64)
		if err != nil || id == 0 {
			return errors.New("moderation report event has an invalid report_id")
		}
		if _, exists := b.reports[id]; exists {
			return fmt.Errorf("duplicate moderation report ID %d", id)
		}
		b.reports[id] = struct{}{}
		actor := attributes["reporter"]
		if actor == "" {
			actor = sender
		}
		return b.addModeration(transaction, eventIndex, InventoryModeration{
			ReportID: id, Owner: attributes["owner"], Repo: attributes["repo"],
			Action: "submitted", Actor: actor, ReasonHash: attributes["reason_hash"],
		})
	case "resolve_moderation_report":
		id, err := strconv.ParseUint(attributes["report_id"], 10, 64)
		if err != nil || id == 0 {
			return errors.New("moderation resolution has an invalid report_id")
		}
		if _, exists := b.reports[id]; !exists {
			return fmt.Errorf("moderation resolution references unknown report ID %d", id)
		}
		return b.addModeration(transaction, eventIndex, InventoryModeration{
			ReportID: id, Action: "resolved", Actor: sender, Status: attributes["status"],
			ReasonHash: attributes["reason_hash"],
		})
	case "appeal_moderation_report":
		id, err := strconv.ParseUint(attributes["report_id"], 10, 64)
		if err != nil || id == 0 {
			return errors.New("moderation appeal has an invalid report_id")
		}
		if _, exists := b.reports[id]; !exists {
			return fmt.Errorf("moderation appeal references unknown report ID %d", id)
		}
		actor := attributes["owner"]
		if actor == "" {
			actor = sender
		}
		return b.addModeration(transaction, eventIndex, InventoryModeration{
			ReportID: id, Action: "appealed", Actor: actor, ReasonHash: attributes["reason_hash"],
		})
	case "resolve_moderation_appeal":
		id, err := strconv.ParseUint(attributes["report_id"], 10, 64)
		if err != nil || id == 0 {
			return errors.New("moderation appeal resolution has an invalid report_id")
		}
		if _, exists := b.reports[id]; !exists {
			return fmt.Errorf("moderation appeal resolution references unknown report ID %d", id)
		}
		return b.addModeration(transaction, eventIndex, InventoryModeration{
			ReportID: id, Action: "appeal_resolved", Actor: sender, Status: attributes["status"],
			ReasonHash: attributes["reason_hash"],
		})
	case "set_moderation_status":
		return b.addModeration(transaction, eventIndex, InventoryModeration{
			Owner: attributes["owner"], Repo: attributes["repo"], Action: "status_set",
			Actor: sender, Status: attributes["status"], ReasonHash: attributes["reason_hash"],
		})
	case "register_username":
		name, owner := attributes["name"], attributes["owner"]
		if name == "" || owner == "" {
			return errors.New("username registration is missing name or owner")
		}
		if existing := b.usernameByName[name]; existing != "" {
			return fmt.Errorf("username %s was already registered", name)
		}
		if existing := b.usernameByUser[owner]; existing != "" {
			return fmt.Errorf("owner %s already holds username %s", owner, existing)
		}
		b.usernameByName[name], b.usernameByUser[owner] = owner, name
	case "release_username":
		name, owner := attributes["name"], attributes["owner"]
		if b.usernameByName[name] != owner || b.usernameByUser[owner] != name {
			return fmt.Errorf("username release %s/%s does not match reconstructed state", name, owner)
		}
		delete(b.usernameByName, name)
		delete(b.usernameByUser, owner)
	case "award_badge":
		if recipient := attributes["recipient"]; recipient == "" {
			return errors.New("badge event is missing recipient")
		} else {
			b.recipients[recipient] = struct{}{}
		}
	case "register_release":
		if version := attributes["version"]; version == "" {
			return errors.New("release event is missing version")
		} else {
			b.releases[version] = struct{}{}
		}
	case "instantiate", "migrate", "update_ref", "delete_ref", "set_collaborator", "remove_collaborator",
		"transfer_ownership_started", "transfer_ownership_cancelled", "set_guardians", "recovery_proposed",
		"recovery_approved", "recovery_cancelled", "update_repo_info", "set_moderation_committee", "sponsor",
		"set_revenue_splits", "set_fee_config", "set_username_policy", "schedule_upgrade", "cancel_upgrade":
		// These known V1 actions do not change the inventory's enumerable key
		// sets. Their final state is cross-checked through the fixed-height
		// snapshot before a migration plan may be emitted.
	default:
		return fmt.Errorf("unknown target-contract action %q", action)
	}
	return nil
}

func (b *inventoryBuilder) addModeration(transaction orderedTx, eventIndex int, entry InventoryModeration) error {
	timestamp := b.blocks[transaction.height].timestamp
	if timestamp == 0 {
		return fmt.Errorf("moderation history requires a non-zero block timestamp for height %d", transaction.height)
	}
	if entry.Actor == "" || entry.Action == "" {
		return errors.New("moderation event is missing actor or action")
	}
	if entry.Action == "submitted" && (entry.Owner == "" || entry.Repo == "" || entry.ReasonHash == "") {
		return errors.New("moderation submission is missing repository or reason")
	}
	if (entry.Action == "resolved" || entry.Action == "appeal_resolved" || entry.Action == "status_set") && entry.Status == "" {
		return errors.New("moderation decision is missing status")
	}
	entry.Timestamp = timestamp
	entry.Height = transaction.height
	entry.TxIndex = transaction.index
	entry.EventIndex = uint32(eventIndex)
	b.moderation = append(b.moderation, entry)
	return nil
}

func canonicalEvent(transaction orderedTx, eventIndex int, action string, attributes map[string]string) []byte {
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var value strings.Builder
	fmt.Fprintf(&value, "%d\x00%d\x00%s\x00%d\x00%s", transaction.height, transaction.index, transaction.Hash, eventIndex, action)
	for _, key := range keys {
		value.WriteByte(0)
		value.WriteString(key)
		value.WriteByte(0)
		value.WriteString(attributes[key])
	}
	return []byte(value.String())
}

func (b *inventoryBuilder) inventory(chainID, contract string, height uint64) *Inventory {
	inventory := &Inventory{
		Schema: InventorySchema, ChainID: chainID, Contract: contract, CutoverHeight: height,
		EventCommitment: hex.EncodeToString(b.commitment[:]), SuccessfulTxs: b.successfulTxs,
	}
	ownerSet := map[string]struct{}{}
	for _, repository := range b.repositories {
		sort.Strings(repository.record.Aliases)
		inventory.Repositories = append(inventory.Repositories, repository.record)
		ownerSet[repository.record.Owner] = struct{}{}
	}
	sort.Slice(inventory.Repositories, func(i, j int) bool {
		if inventory.Repositories[i].Owner != inventory.Repositories[j].Owner {
			return inventory.Repositories[i].Owner < inventory.Repositories[j].Owner
		}
		if inventory.Repositories[i].Name != inventory.Repositories[j].Name {
			return inventory.Repositories[i].Name < inventory.Repositories[j].Name
		}
		return inventory.Repositories[i].Identity < inventory.Repositories[j].Identity
	})
	inventory.Owners = sortedKeys(ownerSet)
	for id := range b.reports {
		inventory.ReportIDs = append(inventory.ReportIDs, id)
	}
	sort.Slice(inventory.ReportIDs, func(i, j int) bool { return inventory.ReportIDs[i] < inventory.ReportIDs[j] })
	for name, owner := range b.usernameByName {
		inventory.Usernames = append(inventory.Usernames, InventoryUsername{Name: name, Owner: owner})
	}
	sort.Slice(inventory.Usernames, func(i, j int) bool { return inventory.Usernames[i].Name < inventory.Usernames[j].Name })
	inventory.BadgeRecipients = sortedKeys(b.recipients)
	inventory.ReleaseVersions = sortedKeys(b.releases)
	inventory.ModerationTrail = append([]InventoryModeration(nil), b.moderation...)
	return inventory
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func MarshalInventory(inventory *Inventory) ([]byte, error) {
	if inventory == nil {
		return nil, errors.New("inventory is nil")
	}
	if err := ValidateInventory(inventory); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func ReadInventory(path string) (*Inventory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var inventory Inventory
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		return nil, fmt.Errorf("decode V1 inventory: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("V1 inventory contains trailing JSON data")
	}
	if err := ValidateInventory(&inventory); err != nil {
		return nil, err
	}
	return &inventory, nil
}

func ValidateInventory(inventory *Inventory) error {
	if inventory == nil {
		return errors.New("inventory is nil")
	}
	if inventory.Schema != InventorySchema {
		return fmt.Errorf("unsupported V1 inventory schema %q", inventory.Schema)
	}
	if inventory.ChainID == "" || inventory.Contract == "" || inventory.CutoverHeight == 0 {
		return errors.New("V1 inventory source is incomplete")
	}
	if err := validateTxSearchQuery(inventory.TxSearchQuery, inventory.Contract, inventory.CutoverHeight); err != nil {
		return fmt.Errorf("V1 inventory tx_search %w", err)
	}
	if inventory.SuccessfulTxs > inventory.ScannedTxs {
		return errors.New("V1 inventory transaction counts are invalid")
	}
	if err := validateLowerHash("cutover block hash", inventory.CutoverBlockHash); err != nil {
		return fmt.Errorf("V1 inventory %w", err)
	}
	for label, digest := range map[string]string{
		"tx_search SHA-256":      inventory.SourceSHA256,
		"block evidence SHA-256": inventory.BlockSHA256,
		"event commitment":       inventory.EventCommitment,
	} {
		if len(digest) != sha256.Size*2 {
			return fmt.Errorf("V1 inventory %s is invalid", label)
		}
		if _, err := hex.DecodeString(digest); err != nil || strings.ToLower(digest) != digest {
			return fmt.Errorf("V1 inventory %s is invalid", label)
		}
	}
	if len(inventory.Blocks) == 0 {
		return errors.New("V1 inventory has no block evidence")
	}
	cutoverFound := false
	previousHeight := uint64(0)
	for _, block := range inventory.Blocks {
		if block.Height == 0 || block.Height <= previousHeight || block.Timestamp == 0 {
			return errors.New("V1 inventory blocks are not canonically ordered")
		}
		if err := validateLowerHash("block hash", block.Hash); err != nil {
			return fmt.Errorf("V1 inventory block %d: %w", block.Height, err)
		}
		if block.Height == inventory.CutoverHeight {
			cutoverFound = block.Hash == inventory.CutoverBlockHash
		}
		previousHeight = block.Height
	}
	if !cutoverFound {
		return errors.New("V1 inventory cutover block is missing or inconsistent")
	}
	if !sort.SliceIsSorted(inventory.Repositories, func(i, j int) bool {
		if inventory.Repositories[i].Owner != inventory.Repositories[j].Owner {
			return inventory.Repositories[i].Owner < inventory.Repositories[j].Owner
		}
		if inventory.Repositories[i].Name != inventory.Repositories[j].Name {
			return inventory.Repositories[i].Name < inventory.Repositories[j].Name
		}
		return inventory.Repositories[i].Identity < inventory.Repositories[j].Identity
	}) {
		return errors.New("V1 inventory repositories are not canonical")
	}
	if !sort.StringsAreSorted(inventory.Owners) || !sort.StringsAreSorted(inventory.BadgeRecipients) ||
		!sort.StringsAreSorted(inventory.ReleaseVersions) {
		return errors.New("V1 inventory collections are not canonical")
	}
	if !sort.SliceIsSorted(inventory.ReportIDs, func(i, j int) bool { return inventory.ReportIDs[i] < inventory.ReportIDs[j] }) ||
		!sort.SliceIsSorted(inventory.Usernames, func(i, j int) bool { return inventory.Usernames[i].Name < inventory.Usernames[j].Name }) {
		return errors.New("V1 inventory collections are not canonical")
	}
	previousPosition := ""
	for _, entry := range inventory.ModerationTrail {
		if entry.Action == "" || entry.Actor == "" || entry.Timestamp == 0 || entry.Height == 0 {
			return errors.New("V1 inventory contains incomplete moderation history")
		}
		position := fmt.Sprintf("%020d/%010d/%010d", entry.Height, entry.TxIndex, entry.EventIndex)
		if previousPosition != "" && position <= previousPosition {
			return errors.New("V1 inventory moderation history is not canonically ordered")
		}
		previousPosition = position
	}
	identities, locators, usernames, usernameOwners := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, repository := range inventory.Repositories {
		if repository.Identity == "" || repository.Owner == "" || repository.Name == "" || repository.CreatedTxHash == "" || repository.CreatedHeight == 0 {
			return errors.New("V1 inventory contains an incomplete repository")
		}
		if _, duplicate := identities[repository.Identity]; duplicate {
			return fmt.Errorf("V1 inventory contains duplicate repository identity %s", repository.Identity)
		}
		identities[repository.Identity] = struct{}{}
		if !sort.StringsAreSorted(repository.Aliases) || len(repository.Aliases) == 0 {
			return fmt.Errorf("V1 inventory aliases for %s/%s are not canonical", repository.Owner, repository.Name)
		}
		for _, locator := range repository.Aliases {
			if _, duplicate := locators[locator]; duplicate {
				return fmt.Errorf("V1 inventory contains duplicate locator %s", locator)
			}
			locators[locator] = struct{}{}
		}
	}
	if err := rejectSortedDuplicates("owner", inventory.Owners); err != nil {
		return err
	}
	if err := rejectSortedDuplicates("badge recipient", inventory.BadgeRecipients); err != nil {
		return err
	}
	if err := rejectSortedDuplicates("release version", inventory.ReleaseVersions); err != nil {
		return err
	}
	for index, id := range inventory.ReportIDs {
		if id == 0 || (index > 0 && id == inventory.ReportIDs[index-1]) {
			return fmt.Errorf("V1 inventory contains invalid or duplicate report ID %d", id)
		}
	}
	for _, username := range inventory.Usernames {
		if username.Name == "" || username.Owner == "" {
			return errors.New("V1 inventory contains an incomplete username")
		}
		if _, duplicate := usernames[username.Name]; duplicate {
			return fmt.Errorf("V1 inventory contains duplicate username %s", username.Name)
		}
		if _, duplicate := usernameOwners[username.Owner]; duplicate {
			return fmt.Errorf("V1 inventory contains duplicate username owner %s", username.Owner)
		}
		usernames[username.Name], usernameOwners[username.Owner] = struct{}{}, struct{}{}
	}
	return nil
}

func validateLowerHash(label, value string) error {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return fmt.Errorf("%s must be a lowercase 32-byte hex hash", label)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("%s must be a lowercase 32-byte hex hash", label)
	}
	return nil
}

// VerifyBlockEvidence detects fixed-height reorgs or substitution after an
// inventory was generated. It also ensures the exact evidence document digest
// is unchanged.
func VerifyBlockEvidence(inventory *Inventory, evidence *BlockEvidence) error {
	if err := ValidateInventory(inventory); err != nil {
		return err
	}
	if evidence == nil || evidence.sha256 == "" {
		return errors.New("block evidence is required")
	}
	if evidence.sha256 != inventory.BlockSHA256 {
		return errors.New("block evidence SHA-256 does not match inventory")
	}
	for _, expected := range inventory.Blocks {
		actual, exists := evidence.blocks[expected.Height]
		if !exists {
			return fmt.Errorf("block evidence is missing inventory height %d", expected.Height)
		}
		if actual.chainID != inventory.ChainID || actual.hash != expected.Hash || actual.timestamp != expected.Timestamp {
			return fmt.Errorf("block evidence at height %d does not match inventory; possible fixed-height reorg", expected.Height)
		}
	}
	return nil
}

// VerifyInventoryEvidence replays both source evidence files and requires the
// result to equal the saved inventory byte-for-byte after canonical encoding.
func VerifyInventoryEvidence(inventory *Inventory, txSearchRaw []byte, evidence *BlockEvidence) error {
	if err := VerifyBlockEvidence(inventory, evidence); err != nil {
		return err
	}
	rebuilt, err := BuildInventory(txSearchRaw, inventory.ChainID, inventory.Contract, inventory.CutoverHeight, evidence)
	if err != nil {
		return fmt.Errorf("replay V1 inventory evidence: %w", err)
	}
	expected, err := MarshalInventory(inventory)
	if err != nil {
		return err
	}
	actual, err := MarshalInventory(rebuilt)
	if err != nil {
		return err
	}
	if !bytes.Equal(expected, actual) {
		return errors.New("replayed V1 inventory does not match saved inventory")
	}
	return nil
}

func rejectSortedDuplicates(label string, values []string) error {
	for index, value := range values {
		if value == "" || (index > 0 && value == values[index-1]) {
			return fmt.Errorf("V1 inventory contains invalid or duplicate %s %q", label, value)
		}
	}
	return nil
}
