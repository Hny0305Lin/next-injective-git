# Next Injective Git — Unified developer entrypoint
# Thin wrappers; CI remains authoritative (see .github/workflows/ci.yml).
# Usage: make <target>  or  make help

.PHONY: help vet test test-race check suite-check abi gen-abi web web-test web-typecheck contracts-check clean

help: ## Show available targets
	@grep -E '^[a-zA-Z0-9_.-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}{printf "  %-20s %s\n", $$1, $$2}'

# --- Go ---

vet: ## go vet (cli)
	cd cli && go vet ./...

test: ## go test (cli) — short, no -race
	cd cli && go test -count=1 ./...

test-race: ## go test with -race
	bash scripts/ci/race-check.sh --required 2>/dev/null || bash scripts/race-check.sh --required

# --- Solidity ---

contracts-check: ## immutable Suite solc + artifact parity gate
	bash scripts/ci/evm-v2-check.sh --required 2>/dev/null || bash scripts/evm-v2-check.sh --required

gen-abi: ## regenerate Go ABIs from canonical Solidity artifacts
	npm run --prefix contracts/evm-v2 abi 2>/dev/null || npm run --prefix contracts/evm-v2 check 2>/dev/null || true
	@mkdir -p cli/internal/evm/abi 2>/dev/null || mkdir -p cli/internal/chain/abi
	@if [ -d cli/internal/evm/abi ]; then \
		cp contracts/evm-v2/abi/*.json cli/internal/evm/abi/ 2>/dev/null || cp contracts/evm-v2/abi/*.json cli/internal/chain/abi/; \
		echo "ABIs synced to cli/internal/evm/abi (or chain/abi fallback)"; \
	else \
		cp contracts/evm-v2/abi/*.json cli/internal/chain/abi/; \
		echo "ABIs synced to cli/internal/chain/abi"; \
	fi

abi: gen-abi ## alias for gen-abi

# --- Web ---

web-test: ## web API tests
	cd web && npm run test:api

web-typecheck: ## web typecheck
	cd web && npm run typecheck

web-build: ## web production build
	cd web && npm run build

web: web-typecheck web-build ## web typecheck + build

# --- Gates ---

check: ## full source gate (suite-readiness --required)
	bash scripts/ci/suite-readiness.sh --required 2>/dev/null || bash scripts/suite-readiness.sh --required

suite-check: check ## alias for check

# --- Composite ---

all: vet test contracts-check web-typecheck ## vet + test + contracts + web typecheck

clean: ## remove build artifacts (cli/dist, web/dist, contracts/out)
	rm -rf cli/dist cli/bin web/dist contracts/evm-v2/out contracts/evm-v2/cache

# Default
.DEFAULT_GOAL := help
