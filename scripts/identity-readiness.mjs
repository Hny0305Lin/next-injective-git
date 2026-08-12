#!/usr/bin/env node

import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// URL pathnames retain a leading slash before a Windows drive letter (for
// example `/D:/repo`). Convert the module URL through Node's platform-aware
// helper before resolving the default repository root.
const defaultRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const root = resolve(process.argv[2] || defaultRoot);
const failures = [];
const check = (condition, message) => {
  if (!condition) failures.push(message);
};
const read = (relative) => readFileSync(join(root, relative), "utf8");

let abi;
try {
  abi = JSON.parse(read("contracts/evm-v2/abi/RepoRegistryV2.json"));
} catch (error) {
  console.error(`IDENTITY READINESS: FAIL (ABI parse: ${error.message})`);
  process.exit(1);
}

const evm = read("contracts/evm-v2/src/RepoRegistryV2.sol");
const evmTests = read("contracts/evm-v2/test/RepoRegistryV2.t.sol");
const goBackend = read("cli/internal/chain/backend.go");
const goABI = read("cli/internal/chain/evm_abi.go");
const goRPC = read("cli/internal/chain/evm_rpc.go");
const goRegistry = read("cli/internal/chain/evm_registry.go");
const remote = read("cli/internal/remote/helper.go");
const remoteTests = read("cli/internal/remote/push_flow_test.go");
const webChain = read("web/src/lib/chain.ts");
const webPage = read("web/src/pages/Repo/index.tsx");
const webTests = read("web/test/chain-evm-v2.test.mjs");
const adr = read("docs/evm-v2-repo-identity.md");

for (const token of [
  "identityChainId",
  "_activeLocators",
  "_aliases",
  "_locatorReservations",
  "_pendingTransfers",
  "function resolveRepo(",
  "function getRepoById(",
  "function listReposPage(",
  "function beginOwnershipTransfer(",
  "function cancelOwnershipTransfer(",
  "function rejectOwnershipTransfer(",
  "function expireOwnershipTransfer(",
  "function acceptOwnership(",
  "error RepoMoved(",
  "event OwnershipTransferStarted(",
  "event OwnershipTransferCancelled(",
  "event OwnershipTransferred(",
  "_nativeRepoId",
  '"igit:v2:repo"',
]) {
  check(evm.includes(token), `Solidity identity token missing: ${token}`);
}

for (const name of [
  "testStableRepoIdUsesDomainSeparatedCreationIdentity",
  "testOwnershipTransferReservesTargetAndEmitsStarted",
  "testOwnershipTransferAcceptIsStableAndOldLocatorIsReadOnly",
  "testTransferCancelRejectAndExpiryReleaseReservations",
  "testSameRepoHistoricalAliasCanBecomeCanonicalAgain",
  "testHistoricalAliasCannotBeReusedByAnotherRepo",
  "testListReposPageDrainsStableIdsAndTracksOwnershipTransfer",
]) {
  check(evmTests.includes(name), `Foundry identity regression missing: ${name}`);
}

const hasABI = (type, name, inputTypes = undefined) => abi.some((entry) => {
  if (entry.type !== type || entry.name !== name) return false;
  if (!inputTypes) return true;
  return JSON.stringify(entry.inputs.map((input) => input.type)) === JSON.stringify(inputTypes);
});

for (const name of [
  "resolveRepo",
  "getRepoById",
  "beginOwnershipTransfer",
  "cancelOwnershipTransfer",
  "rejectOwnershipTransfer",
  "expireOwnershipTransfer",
  "acceptOwnership",
  "pendingOwnershipTransfer",
]) {
  check(hasABI("function", name), `checked-in ABI missing identity function: ${name}`);
}
for (const [name, inputTypes] of [
  ["listReposPage", ["address", "uint256", "uint256"]],
  ["listRefsPageById", ["bytes32", "uint256", "uint256"]],
  ["listCollaboratorsPageById", ["bytes32", "uint256", "uint256"]],
]) {
  check(hasABI("function", name, inputTypes), `checked-in ABI missing bounded page function: ${name}`);
}
check(
  hasABI("error", "RepoMoved", ["bytes32", "address", "string"]),
  "checked-in ABI missing RepoMoved(bytes32,address,string)",
);
check(
  hasABI("error", "LocatorNotFound", ["address", "string"]),
  "checked-in ABI missing LocatorNotFound(address,string)",
);
for (const name of ["OwnershipTransferStarted", "OwnershipTransferCancelled", "OwnershipTransferred"]) {
  check(hasABI("event", name), `checked-in ABI missing identity event: ${name}`);
}

for (const token of [
  "ResolveRepo(owner, repo string)",
  "type ResolvedRepo struct",
  "isEVMLocatorNotFound",
  "ListRepos(owner string)",
  "decodeEVMRepoPage",
  "listRefsPageByID",
  "listCollaboratorsPageByID",
  "snapshotBlockTag",
  "CallContractAt",
  "BlockNumber",
  "cursor did not advance",
]) {
  check(goBackend.includes(token) || goRegistry.includes(token) || goABI.includes(token) || goRPC.includes(token), `Go identity token missing: ${token}`);
}
check(goABI.includes("type RepoMovedError struct"), "Go typed RepoMovedError is missing");
check(goRegistry.includes("func movedError") || remote.includes("movedError"), "remote canonical write guard is missing");
check(remoteTests.includes("TestMovedRepositoryStopsBeforePreflightPackIPFSAndWrite"), "remote moved preflight regression is missing");

for (const token of [
  "resolveRepo:",
  '"2c1ce86c"',
  "listReposPage:",
  '"502a5e93"',
  "decodeV2RepoPage",
  "listRefsPage:",
  '"084871ef"',
  "listCollaboratorsPage:",
  '"1758c17d"',
  "encodeV2RepoIDPageCall",
  "decodeV2RefPage",
  "decodeV2CollaboratorPage",
  "evmSnapshotBlockTag",
  "blockTag",
  "class EVMLocatorNotFoundError",
  "class EVMRepoMovedError",
  "export async function resolveRepo",
  "return error instanceof EVMLocatorNotFoundError",
]) {
  check(webChain.includes(token), `Web identity token missing: ${token}`);
}
for (const token of ["repo-moved-notice", "resolvedRepo?.isCanonical === true", "getRepoStore(resolvedRepo?.repoId"]) {
  check(webPage.includes(token), `Web moved-route token missing: ${token}`);
}
check(webTests.includes("decode RepoMoved and expose the canonical URL"), "Web RepoMoved regression is missing");
check(
  webTests.includes("EVM owner repository listing drains bounded pages without V1 fallback"),
  "Web bounded owner-repository pagination regression is missing",
);
check(
  webTests.includes("EVM owner repository pagination rejects a stalled cursor without V1 fallback"),
  "Web owner-repository stalled-cursor regression is missing",
);
check(
  webTests.includes("EVM stable repo ID reads drain two ref and collaborator pages without V1 fallback"),
  "Web stable-ID ref/collaborator pagination regression is missing",
);
check(webTests.includes("eth_blockNumber") && webTests.includes("snapshotBlockTag"), "Web pagination block snapshot regression is missing");

for (const token of [
  "OWNERSHIP_TRANSFER_ACCEPTANCE_WINDOW",
  "rejectOwnershipTransfer",
  "expireOwnershipTransfer",
  "same repoId",
  "historical alias",
]) {
  check(adr.includes(token), `identity ADR missing security decision/evidence: ${token}`);
}

if (failures.length > 0) {
  for (const failure of failures) console.error(`FAIL: ${failure}`);
  console.error(`IDENTITY READINESS: FAIL (${failures.length} failure(s))`);
  process.exit(1);
}

console.log("IDENTITY READINESS: PASS (stable repoId, locator/alias, transfer, client, and docs evidence present)");
