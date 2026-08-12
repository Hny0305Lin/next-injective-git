package chain

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// evmNonceManager serializes the initial pending-nonce query and then hands
// out locally incremented nonces to concurrent operations on one registry. A
// failed operation invalidates the cache so the next operation re-syncs with
// the node instead of trusting a potentially stale local value.
type evmNonceManager struct {
	mu         sync.Mutex
	valid      bool
	endpoint   string
	chainID    uint64
	from       string
	next       uint64
	generation uint64
}

func newEVMNonceManager() *evmNonceManager {
	return &evmNonceManager{}
}

type evmNonceReservation struct {
	manager    *evmNonceManager
	endpoint   string
	chainID    uint64
	from       string
	generation uint64
	nonce      uint64
}

// reserve obtains eth_getTransactionCount only when this registry has no
// valid cache for the current endpoint/chain/account. Holding the manager lock
// over that first RPC prevents concurrent callers from both reserving the same
// pending nonce before either local increment is visible.
func (m *evmNonceManager) reserve(ctx context.Context, rpc *EVMRPC, chainID uint64, from string) (uint64, *evmNonceReservation, error) {
	if m == nil {
		return 0, nil, fmt.Errorf("EVM nonce manager is not configured")
	}
	if rpc == nil {
		return 0, nil, fmt.Errorf("EVM RPC transport is not configured")
	}
	normalizedFrom, err := normalizeEVMAddress(from)
	if err != nil {
		return 0, nil, err
	}
	normalizedFrom = strings.ToLower(normalizedFrom)

	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.valid || m.endpoint != rpc.endpoint || m.chainID != chainID || m.from != normalizedFrom {
		pending, err := rpc.TransactionCount(ctx, normalizedFrom, "pending")
		if err != nil {
			m.valid = false
			return 0, nil, err
		}
		if pending == ^uint64(0) {
			return 0, nil, fmt.Errorf("EVM account nonce overflow")
		}
		m.endpoint = rpc.endpoint
		m.chainID = chainID
		m.from = normalizedFrom
		m.next = pending
		m.generation++
		m.valid = true
	}
	nonce := m.next
	if m.next == ^uint64(0) {
		m.valid = false
		return 0, nil, fmt.Errorf("EVM account nonce overflow")
	}
	m.next++
	return nonce, &evmNonceReservation{
		manager:    m,
		endpoint:   m.endpoint,
		chainID:    m.chainID,
		from:       m.from,
		generation: m.generation,
		nonce:      nonce,
	}, nil
}

// Invalidate discards the local cache. It is intentionally safe to call after
// an uncertain send: a subsequent operation will ask the node for pending
// state and can reuse the nonce if the transaction never entered the mempool.
func (r *evmNonceReservation) Invalidate() {
	if r == nil || r.manager == nil {
		return
	}
	r.manager.invalidate(r.endpoint, r.chainID, r.from, r.generation)
}

// Release is the pre-broadcast spelling used by callers. It invalidates the
// cache rather than guessing whether another concurrent reservation exists.
func (r *evmNonceReservation) Release() { r.Invalidate() }

// Commit closes a successful reservation without invalidating the incremented
// cache, allowing the next transaction to use the following nonce locally.
func (r *evmNonceReservation) Commit() {
	// The manager's increment remains valid after a successful receipt.
}

func (m *evmNonceManager) invalidate(endpoint string, chainID uint64, from string, generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.valid && m.endpoint == endpoint && m.chainID == chainID && m.from == from && m.generation == generation {
		m.valid = false
		m.generation++
	}
}
