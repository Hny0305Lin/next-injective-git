# P1.2 Storage Indexer V2 Migration

**Status:** Implemented - Requires Testing and Production Validation

## Overview

All 4 production storage/indexer scripts have been rewritten to use V2 EVM event-based indexing, replacing V1 CosmWasm/LCD query patterns.

This was originally tracked as A11 in the migration planning phase.

## New V2 Scripts

### 1. evm-event-indexer.sh
**Purpose:** General-purpose V2 event indexer with checkpoint recovery

**Key Features:**
- Bounded `eth_getLogs` queries (configurable block range, default 10,000)
- Checkpoint state persistence (JSON format)
- RefUpdated event signature filtering
- Continuous or one-shot modes
- Fail-closed error handling

**Usage:**
```bash
EVM_RPC=https://evm-rpc.injective.network \
SUITE_DIRECTORY=0x... \
./scripts/evm-event-indexer.sh [--from-block BLOCK] [--to-block BLOCK] [--once]
```

### 2. evm-hot-pin-indexer.sh
**Replaces:** `scripts/hot-pin-indexer.sh`

**Key Changes:**
- Queries RefUpdated events via `eth_getLogs` (not LCD tx queries)
- Calculates hot CIDs based on frequency within time window
- Bounded block scanning with checkpointing
- Optional unpin of stale CIDs (only if in durable archive)

**Configuration:**
- `NEW_DAYS`: CIDs from last N days (default: 14)
- `HOT_WINDOW_DAYS`: Frequency calculation window (default: 30)
- `HOT_MIN_HITS`: Minimum hits to be "hot" (default: 3)
- `MAX_BLOCKS_PER_QUERY`: Blocks per RPC call (default: 10,000)

### 3. evm-archive-indexer.sh
**Replaces:** `scripts/archive-indexer.sh`

**Key Changes:**
- Scans RefUpdated events for all historical CIDs
- Pins to local Kubo
- Archives to S3/Filone with CAR format
- Checkpoint recovery for interrupted runs
- Bounded queries with progress tracking

**Modes:**
- `--pin-only`: Pin to Kubo only, skip S3 archive
- `--once`: Single scan then exit
- Default: Continuous monitoring

### 4. evm-replication-reaper.sh
**Replaces:** `scripts/replication-reaper.sh`

**Key Changes:**
- Queries current refs via EVM contract calls (not CosmWasm smart queries)
- Scans all repositories via `repositoryCount()` and `repositoryIdAt()`
- Calls `listRefsPage()` for each repo with pagination
- Only unpins CIDs not found in any current ref
- Mainnet-only safety checks (CHAIN_ID, HTTPS, production RPC)

## Event Signature

All scripts use the RefUpdated event:
```solidity
event RefUpdated(
    bytes32 indexed repoId,
    string refName,
    string commitSha,
    string[] packUris,
    address updatedBy
)
```

**Topic0:** `0xa7c1db3f7e3d8c2f5b8e9a1c4d6f2e8b3a5c7d9f1e4b6a8c2d5f7e9b1c3d5f7e9`
(Note: Actual hash needs to be calculated from real event signature)

## Technical Improvements

### V1 → V2 Migration

| Aspect | V1 (Old) | V2 (New) |
|--------|----------|----------|
| **Query Method** | `/cosmos/tx/v1beta1/txs` | `eth_getLogs` |
| **Filter** | `wasm.action='update_ref'` | Event signature topic |
| **Data Source** | TX message parsing | ABI-decoded events |
| **Current State** | CosmWasm smart queries | EVM contract calls |
| **Pagination** | LCD pagination key | Block range chunking |
| **Checkpoint** | None or manual | JSON state file |
| **Reorg Handling** | None | Block confirmations |

### Safety Features

1. **Bounded Queries:** Max 10,000 blocks per RPC call (configurable)
2. **Checkpoint Recovery:** Can resume from last processed block
3. **Fail-Closed:** Script exits on RPC error, malformed data, or invalid state
4. **Mainnet Guards:** Reaper validates chain ID, HTTPS, non-testnet RPC
5. **Deduplication:** CIDs sorted and uniqued before processing
6. **State Validation:** TSV format checked before GC operations

## Production Deployment Checklist

### Phase 1: Testing (Current)
- [ ] Calculate actual RefUpdated event signature hash
- [ ] Test indexers against Injective testnet
- [ ] Validate ABI decoding (currently simplified placeholders)
- [ ] Verify checkpoint recovery works correctly
- [ ] Test reaper with known-safe CID set

### Phase 2: Integration
- [ ] Replace placeholder ABI decoders with proper `cast` or ethers.js parsing
- [ ] Add reorg detection (check block hashes at checkpoint)
- [ ] Implement exponential backoff for RPC errors
- [ ] Add monitoring/alerting hooks
- [ ] Document expected RPC rate limits

### Phase 3: Production Rollout
- [ ] Deploy indexers in read-only mode (no unpin)
- [ ] Validate CID coverage matches expected refs
- [ ] Run reaper dry-run mode (log but don't unpin)
- [ ] Compare V1 vs V2 CID sets for discrepancies
- [ ] Enable reaper with `ALLOW_UNPIN=true` only after validation

### Phase 4: V1 Sunset
- [ ] Archive old V1 scripts to `archive/storage-v1/`
- [ ] Update systemd services to use V2 scripts
- [ ] Document V1 → V2 migration for operators
- [ ] Remove V1 LCD dependencies

## Known Limitations

1. **ABI Decoding Simplified:** Current scripts use grep/sed placeholders for ABI decoding. Production needs:
   - Proper ABI decoder (cast, ethers.js, or Python web3.py)
   - Handle dynamic arrays correctly
   - Validate checksums and offsets

2. **Event Signature TODO:** Replace placeholder topic hash with actual:
   ```bash
   cast keccak "RefUpdated(bytes32,string,string,string[],address)"
   ```

3. **Block Number Estimation:** Hot-pin indexer estimates blocks from days (assumes 2s/block). Better approach:
   - Binary search for first block after cutoff timestamp
   - Use `eth_getBlockByNumber` with timestamps

4. **No Reorg Detection Yet:** Scripts assume canonical chain. Add:
   - Store block hash at checkpoint
   - Verify hash matches on resume
   - Rewind if reorg detected

## Migration from V1

### For Operators

**Do NOT enable reaper until:**
1. V2 indexers validated against testnet
2. CID coverage verified complete
3. Dry-run mode tested successfully

**Migration Steps:**
1. Run V2 indexers in parallel with V1 (read-only)
2. Compare outputs for discrepancies
3. Investigate any missing CIDs
4. Switch systemd services to V2
5. Archive V1 scripts (do not delete immediately)

### For Developers

**Old V1 patterns (DO NOT USE):**
```bash
# V1 LCD query
curl "${LCD}/cosmos/tx/v1beta1/txs?query=wasm._contract_address='${CONTRACT}'+AND+wasm.action='update_ref'"

# V1 smart query
curl "${LCD}/cosmwasm/wasm/v1/contract/${CONTRACT}/smart/${QUERY}"
```

**New V2 patterns (USE THESE):**
```bash
# V2 event query
eth_getLogs with topic filter

# V2 state query
eth_call with ABI-encoded calldata
```

## Rollback Plan

If V2 indexers fail in production:

1. **Immediate:** Re-enable V1 scripts from git history
2. **Investigate:** Collect RPC logs, event samples, checkpoint state
3. **Fix:** Address root cause (ABI decoding, reorg, rate limiting)
4. **Re-test:** Full validation cycle before re-deployment
5. **Document:** Update this file with lessons learned

## Support

**Before Production Deployment:**
- Review with Injective EVM team for RPC best practices
- Validate event signatures match deployed contracts
- Test with production Suite Directory address

**Questions/Issues:**
- Check `scripts/evm-event-indexer.sh` comments for implementation details
- Review `docs/backlog.md` for P1.2 context and next steps
- Consult Injective EVM documentation for JSON-RPC specifics

---

**Status:** Ready for testnet validation  
**Risk Level:** HIGH (storage safety critical)  
**Priority:** P1.2 (critical path for testnet cutover)  
**Next Action:** Calculate event signatures, test ABI decoding, validate against testnet
