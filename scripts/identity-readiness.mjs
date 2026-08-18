#!/usr/bin/env node

// Static regression gate for the immutable Suite repository-identity path.
// It intentionally checks source and checked ABI evidence only; deployment
// evidence belongs to migration-cutover-readiness.sh.
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(process.argv[2] || fileURLToPath(new URL("..", import.meta.url)));
const failures = [];
const read = (relative) => readFileSync(join(root, relative), "utf8");
const check = (condition, message) => {
  if (!condition) failures.push(message);
};

let abi;
try {
  abi = JSON.parse(read("contracts/evm-v2/abi/RepositoryCore.json"));
} catch (error) {
  console.error(`IDENTITY READINESS: FAIL (ABI parse: ${error.message})`);
  process.exit(1);
}

const core = read("contracts/evm-v2/src/RepositoryCore.sol");
const webAbi = read("web/src/lib/abis.ts");
const webRegistry = read("web/src/lib/registry.ts");
const goRegistry = read("cli/internal/chain/evm_suite_registry.go");
const identityDoc = read("docs/evm-v2-repo-identity.md");

const hasFunction = (name, inputTypes) => abi.some((entry) =>
  entry.type === "function" && entry.name === name
  && JSON.stringify(entry.inputs.map((input) => input.type)) === JSON.stringify(inputTypes),
);

for (const [name, inputTypes] of [
  ["resolveRepository", ["address", "string"]],
  ["getRepository", ["bytes32"]],
  ["beginOwnershipTransfer", ["bytes32", "address"]],
  ["cancelOwnershipTransfer", ["bytes32"]],
  ["acceptOwnershipTransfer", ["bytes32"]],
  ["expireOwnershipTransfer", ["bytes32"]],
  ["pendingOwnershipTransfer", ["bytes32"]],
]) {
  check(hasFunction(name, inputTypes), `checked RepositoryCore ABI is missing ${name}`);
}

check(!abi.some((entry) => entry.type === "function" && entry.name === "rejectOwnershipTransfer"),
  "RepositoryCore ABI must not advertise a nonexistent rejectOwnershipTransfer selector");
check(core.includes("function cancelOwnershipTransfer(bytes32 repoId)"), "RepositoryCore cancellation entry point is missing");
check(!core.includes("function rejectOwnershipTransfer("), "RepositoryCore exposes an unsupported rejection entry point");
check(!webAbi.includes('"function rejectOwnershipTransfer(bytes32 repoId)"'),
  "Web ABI advertises the unsupported ownership rejection selector");
check(/rejectOwnershipTransferWithEvm[\s\S]*?"cancelOwnershipTransfer"/.test(webRegistry),
  "Web rejection must call cancelOwnershipTransfer");
check(/func \(s \*EVMSuiteRegistry\) RejectOwnershipTransfer[\s\S]*?return s\.CancelOwnershipTransfer/.test(goRegistry),
  "CLI rejection must call CancelOwnershipTransfer");
check(identityDoc.includes("mutually exclusive"),
  "repository identity documentation must record transfer/recovery mutual exclusion");

if (failures.length > 0) {
  for (const failure of failures) console.error(`FAIL: ${failure}`);
  console.error(`IDENTITY READINESS: FAIL (${failures.length} failure(s))`);
  process.exit(1);
}

console.log("IDENTITY READINESS: PASS (Suite ABI, transfer cancellation mapping, and identity rules)");
