#!/usr/bin/env node

import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const [evidenceArg, expectedCommitArg] = process.argv.slice(2);

function fail(message) {
  throw new Error(message);
}

function readJSON(path) {
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    fail(`${path}: ${error instanceof Error ? error.message : String(error)}`);
  }
}

function string(value, field, pattern) {
  if (typeof value !== "string" || !value || (pattern && !pattern.test(value))) fail(`invalid ${field}`);
  return value.toLowerCase();
}

function equal(actual, expected, field) {
  if (actual !== expected) fail(`${field} = ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`);
}

function successfulReceipt(receipt, field, expectedHash, expectedAddress = "") {
  if (!receipt || typeof receipt !== "object") fail(`missing ${field}`);
  const status = String(receipt.status ?? "").toLowerCase();
  if (status !== "0x1" && status !== "1" && status !== "success") fail(`${field}.status is not successful`);
  equal(string(receipt.transactionHash ?? receipt.transaction_hash, `${field}.transaction hash`, /^0x[0-9a-f]{64}$/i), expectedHash, `${field}.transaction hash`);
  string(receipt.blockNumber ?? receipt.block_number, `${field}.block number`, /^0x(?:0|[1-9a-f][0-9a-f]*)$/i);
  string(receipt.blockHash ?? receipt.block_hash, `${field}.block hash`, /^0x[0-9a-f]{64}$/i);
  if (expectedAddress) {
    equal(string(receipt.contractAddress ?? receipt.contract_address, `${field}.contract address`, /^0x[0-9a-f]{40}$/i), expectedAddress, `${field}.contract address`);
  }
}

try {
  if (!evidenceArg || !/^[0-9a-f]{40}$/i.test(expectedCommitArg ?? "")) {
    fail("usage: validate-suite-cutover.mjs EVIDENCE_DIRECTORY EXPECTED_COMMIT");
  }
  const evidence = resolve(evidenceArg);
  const expectedCommit = expectedCommitArg.toLowerCase();
  const deployment = readJSON(resolve(evidence, "deployment.json"));
  const activation = readJSON(resolve(evidence, "suite-verification.json"));
  const blockscout = readJSON(resolve(evidence, "blockscout-verification.json"));
  const scope = readJSON(resolve(evidence, "cutover-scope.json"));

  equal(deployment.schema, "igit.evm-suite.deployment.v1", "deployment.schema");
  equal(deployment.status, "bootstrapping", "deployment.status");
  if (!["live-broadcast", "historical-recovery"].includes(deployment.evidence_mode)) fail("invalid deployment.evidence_mode");
  if (deployment.evidence_mode === "historical-recovery") {
    equal(deployment.recovery?.transactions_validated, 17, "deployment historical transaction count");
    string(deployment.recovery?.source, "deployment.recovery.source", /^https:\/\//i);
  }
  equal(string(deployment.source?.commit, "deployment.source.commit", /^[0-9a-f]{40}$/i), expectedCommit, "deployment source commit");
  equal(deployment.compiler?.version, "0.8.24", "deployment.compiler.version");
  if (!Number.isSafeInteger(deployment.chain?.chain_id) || deployment.chain.chain_id <= 0) fail("invalid deployment.chain.chain_id");
  if (!/^0x[0-9a-f]{64}$/i.test(deployment.snapshot_root ?? "")) fail("invalid deployment.snapshot_root");
  if (!Array.isArray(deployment.contracts) || deployment.contracts.length !== 9) fail("deployment must contain nine contracts");

  const expectedContracts = [
    "SuiteDirectory", "BootstrapCoordinator", "RepositoryCore", "RecoveryModule", "ModerationModule",
    "EconomicModule", "UsernameModule", "BadgeModule", "ReleaseModule",
  ];
  const expectedContractOrders = [1, 2, 4, 6, 8, 10, 12, 14, 16];
  const deployed = new Map();
  for (const [index, contract] of deployment.contracts.entries()) {
    equal(contract.contract_name, expectedContracts[index], `deployment.contracts[${index}].contract_name`);
    equal(contract.transaction_order, expectedContractOrders[index], `deployment.contracts[${index}].transaction_order`);
    const address = string(contract.address, `${contract.contract_name}.address`, /^0x[0-9a-f]{40}$/i);
    const transactionHash = string(contract.transaction_hash, `${contract.contract_name}.transaction_hash`, /^0x[0-9a-f]{64}$/i);
    successfulReceipt(contract.receipt, `${contract.contract_name}.receipt`, transactionHash, address);
    const codeHash = string(contract.runtime?.code_hash_keccak256, `${contract.contract_name}.runtime.code_hash_keccak256`, /^0x[0-9a-f]{64}$/i);
    equal(contract.runtime?.template_match_verified, true, `${contract.contract_name}.runtime.template_match_verified`);
    deployed.set(contract.contract_name, { address, codeHash });
  }
  const expectedConfigurations = [
    [3, "bind_bootstrap_coordinator", "SuiteDirectory"],
    [5, "register_repositorycore", "BootstrapCoordinator"],
    [7, "register_recoverymodule", "BootstrapCoordinator"],
    [9, "register_moderationmodule", "BootstrapCoordinator"],
    [11, "register_economicmodule", "BootstrapCoordinator"],
    [13, "register_usernamemodule", "BootstrapCoordinator"],
    [15, "register_badgemodule", "BootstrapCoordinator"],
    [17, "register_releasemodule", "BootstrapCoordinator"],
  ];
  if (!Array.isArray(deployment.configuration_transactions) || deployment.configuration_transactions.length !== expectedConfigurations.length) {
    fail("deployment must contain eight configuration transactions");
  }
  for (const [index, configuration] of deployment.configuration_transactions.entries()) {
    const [order, purpose, targetName] = expectedConfigurations[index];
    equal(configuration.transaction_order, order, `deployment.configuration_transactions[${index}].transaction_order`);
    equal(configuration.purpose, purpose, `deployment.configuration_transactions[${index}].purpose`);
    equal(string(configuration.target, `${purpose}.target`, /^0x[0-9a-f]{40}$/i), deployed.get(targetName).address, `${purpose}.target`);
    string(configuration.calldata, `${purpose}.calldata`, /^0x[0-9a-f]+$/i);
    string(configuration.calldata_sha256, `${purpose}.calldata_sha256`, /^[0-9a-f]{64}$/i);
    const transactionHash = string(configuration.transaction_hash, `${purpose}.transaction_hash`, /^0x[0-9a-f]{64}$/i);
    successfulReceipt(configuration.receipt, `${purpose}.receipt`, transactionHash);
  }
  equal(deployment.directory_binding_verification?.active, false, "deployment directory active state");
  equal(deployment.directory_binding_verification?.registered_module_count, 7, "deployment registered module count");

  equal(scope.schema, "igit.evm-suite.cutover-scope.v1", "scope.schema");
  equal(string(scope.source_commit, "scope.source_commit", /^[0-9a-f]{40}$/i), expectedCommit, "scope source commit");
  equal(scope.mode, "fresh-empty-suite", "scope.mode");
  equal(scope.v1_runtime_policy, "archive-preview-only", "scope.v1_runtime_policy");
  equal(scope.v1_migration_performed, false, "scope.v1_migration_performed");
  equal(scope.migration_evidence_required, false, "scope.migration_evidence_required");
  string(scope.activation_journal, "scope.activation_journal", /^[A-Za-z0-9._-]+$/);
  equal(scope.chain_id, deployment.chain.chain_id, "scope.chain_id");
  equal(string(scope.directory, "scope.directory", /^0x[0-9a-f]{40}$/i), deployed.get("SuiteDirectory").address, "scope.directory");
  equal(string(scope.snapshot_root, "scope.snapshot_root", /^0x[0-9a-f]{64}$/i), deployment.snapshot_root.toLowerCase(), "scope.snapshot_root");
  const emptyAttestation = readJSON(resolve(evidence, "empty-username-escrow-attestation.json"));
  equal(emptyAttestation.schema, "igit.evm-suite.empty-username-escrow-attestation.v1", "empty attestation schema");
  equal(string(emptyAttestation.source_commit, "empty attestation source_commit", /^[0-9a-f]{40}$/i), expectedCommit, "empty attestation source commit");
  equal(emptyAttestation.activation_mode, "fresh-empty-suite", "empty attestation activation mode");
  equal(emptyAttestation.v1_migration_performed, false, "empty attestation migration state");
  equal(emptyAttestation.imported_username_records, 0, "empty attestation imported usernames");
  equal(emptyAttestation.username_escrow_liability, "none", "empty attestation username liability");
  string(emptyAttestation.attestation_transaction_hash, "empty attestation transaction hash", /^0x[0-9a-f]{64}$/i);

  equal(activation.schema, "igit.evm-suite.activation-verification.v1", "activation.schema");
  equal(string(activation.source_commit, "activation.source_commit", /^[0-9a-f]{40}$/i), expectedCommit, "activation source commit");
  equal(activation.chain_id, deployment.chain.chain_id, "activation.chain_id");
  equal(activation.suite_version, 3, "activation.suite_version");
  equal(activation.state, 1, "activation.state");
  equal(activation.active, true, "activation.active");
  equal(activation.activation_mode, scope.mode, "activation.activation_mode");
  equal(activation.v1_runtime_policy, scope.v1_runtime_policy, "activation.v1_runtime_policy");
  equal(activation.v1_migration_performed, false, "activation.v1_migration_performed");
  equal(activation.coordinator_activated, true, "activation.coordinator_activated");
  equal(activation.registered_module_count, 7, "activation.registered_module_count");
  string(activation.block_number, "activation.block_number", /^0x(?:0|[1-9a-f][0-9a-f]*)$/i);
  string(activation.block_hash, "activation.block_hash", /^0x[0-9a-f]{64}$/i);
  equal(string(activation.snapshot_root, "activation.snapshot_root", /^0x[0-9a-f]{64}$/i), deployment.snapshot_root.toLowerCase(), "activation snapshot root");
  const directory = string(activation.directory, "activation.directory", /^0x[0-9a-f]{40}$/i);
  equal(directory, deployed.get("SuiteDirectory").address, "activation directory");

  const expectedModules = expectedContracts.slice(2);
  if (!Array.isArray(activation.modules) || activation.modules.length !== expectedModules.length) fail("activation must contain seven modules");
  for (const [index, module] of activation.modules.entries()) {
    const name = expectedModules[index];
    equal(module.contract_name, name, `activation.modules[${index}].contract_name`);
    string(module.module_id, `${name}.module_id`, /^0x[0-9a-f]{64}$/i);
    equal(module.bootstrap_started, true, `${name}.bootstrap_started`);
    equal(module.expected_count, 0, `${name}.expected_count`);
    equal(module.expected_batches, 0, `${name}.expected_batches`);
    equal(module.imported_count, 0, `${name}.imported_count`);
    equal(module.next_sequence, 0, `${name}.next_sequence`);
    const expectedRoot = string(module.expected_root, `${name}.expected_root`, /^0x[0-9a-f]{64}$/i);
    equal(string(module.rolling_root, `${name}.rolling_root`, /^0x[0-9a-f]{64}$/i), expectedRoot, `${name} empty rolling root`);
    equal(scope.expected_imported_records?.[name], 0, `${name} scoped imported records`);
    equal(string(module.address, `${name}.address`, /^0x[0-9a-f]{40}$/i), deployed.get(name).address, `${name} address`);
    const observed = string(module.observed_code_hash, `${name}.observed_code_hash`, /^0x[0-9a-f]{64}$/i);
    equal(observed, deployed.get(name).codeHash, `${name} deployed code hash`);
    equal(string(module.directory_code_hash, `${name}.directory_code_hash`, /^0x[0-9a-f]{64}$/i), observed, `${name} directory code hash`);
    equal(module.directory_verified, true, `${name}.directory_verified`);
    equal(string(module.module_directory, `${name}.module_directory`, /^0x[0-9a-f]{40}$/i), directory, `${name} module directory`);
    equal(module.bootstrap_finalized, true, `${name}.bootstrap_finalized`);
  }

  equal(blockscout.schema, "igit.evm-suite.blockscout-verification.v1", "blockscout.schema");
  equal(string(blockscout.source_commit, "blockscout.source_commit", /^[0-9a-f]{40}$/i), expectedCommit, "blockscout source commit");
  equal(blockscout.chain_id, deployment.chain.chain_id, "blockscout.chain_id");
  string(blockscout.explorer, "blockscout.explorer", /^https:\/\//i);
  if (!Array.isArray(blockscout.contracts) || blockscout.contracts.length !== expectedContracts.length) fail("Blockscout evidence must contain nine contracts");
  for (const [index, contract] of blockscout.contracts.entries()) {
    const name = expectedContracts[index];
    equal(contract.contract_name, name, `blockscout.contracts[${index}].contract_name`);
    equal(string(contract.address, `${name} Blockscout address`, /^0x[0-9a-f]{40}$/i), deployed.get(name).address, `${name} Blockscout address`);
    equal(contract.status, "verified", `${name} Blockscout status`);
    equal(string(contract.creation_transaction_hash, `${name} creation transaction hash`, /^0x[0-9a-f]{64}$/i), string(deployment.contracts[index].transaction_hash, `${name} deployment transaction hash`, /^0x[0-9a-f]{64}$/i), `${name} creation transaction hash`);
  }

  console.log("SUITE CUTOVER JSON: PASS (historical deployment, fresh-empty activation, code hashes, bindings, and Blockscout verified)");
} catch (error) {
  console.error(`SUITE CUTOVER JSON: FAIL (${error instanceof Error ? error.message : String(error)})`);
  process.exit(1);
}
