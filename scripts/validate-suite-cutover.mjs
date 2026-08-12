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

function successfulReceipt(receipt, field) {
  if (!receipt || typeof receipt !== "object") fail(`missing ${field}`);
  const status = String(receipt.status ?? "").toLowerCase();
  if (status !== "0x1" && status !== "1" && status !== "success") fail(`${field}.status is not successful`);
  string(receipt.blockHash ?? receipt.block_hash, `${field}.block hash`, /^0x[0-9a-f]{64}$/i);
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

  equal(deployment.schema, "igit.evm-suite.deployment.v1", "deployment.schema");
  equal(deployment.status, "bootstrapping", "deployment.status");
  equal(string(deployment.source?.commit, "deployment.source.commit", /^[0-9a-f]{40}$/i), expectedCommit, "deployment source commit");
  equal(deployment.compiler?.version, "0.8.24", "deployment.compiler.version");
  if (!Number.isSafeInteger(deployment.chain?.chain_id) || deployment.chain.chain_id <= 0) fail("invalid deployment.chain.chain_id");
  if (!/^0x[0-9a-f]{64}$/i.test(deployment.snapshot_root ?? "")) fail("invalid deployment.snapshot_root");
  if (!Array.isArray(deployment.contracts) || deployment.contracts.length !== 9) fail("deployment must contain nine contracts");

  const expectedContracts = [
    "SuiteDirectory", "BootstrapCoordinator", "RepositoryCore", "RecoveryModule", "ModerationModule",
    "EconomicModule", "UsernameModule", "BadgeModule", "ReleaseModule",
  ];
  const deployed = new Map();
  for (const [index, contract] of deployment.contracts.entries()) {
    equal(contract.contract_name, expectedContracts[index], `deployment.contracts[${index}].contract_name`);
    equal(contract.transaction_order, index, `deployment.contracts[${index}].transaction_order`);
    const address = string(contract.address, `${contract.contract_name}.address`, /^0x[0-9a-f]{40}$/i);
    string(contract.transaction_hash, `${contract.contract_name}.transaction_hash`, /^0x[0-9a-f]{64}$/i);
    successfulReceipt(contract.receipt, `${contract.contract_name}.receipt`);
    const codeHash = string(contract.runtime?.code_hash_keccak256, `${contract.contract_name}.runtime.code_hash_keccak256`, /^0x[0-9a-f]{64}$/i);
    equal(contract.runtime?.template_match_verified, true, `${contract.contract_name}.runtime.template_match_verified`);
    deployed.set(contract.contract_name, { address, codeHash });
  }
  equal(deployment.directory_binding_verification?.active, false, "deployment directory active state");
  equal(deployment.directory_binding_verification?.registered_module_count, 7, "deployment registered module count");

  equal(activation.schema, "igit.evm-suite.activation-verification.v1", "activation.schema");
  equal(string(activation.source_commit, "activation.source_commit", /^[0-9a-f]{40}$/i), expectedCommit, "activation source commit");
  equal(activation.chain_id, deployment.chain.chain_id, "activation.chain_id");
  equal(activation.suite_version, 3, "activation.suite_version");
  equal(activation.state, 1, "activation.state");
  equal(activation.active, true, "activation.active");
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
  }

  console.log("SUITE CUTOVER JSON: PASS (deployment, activation, code hashes, bindings, and Blockscout verified)");
} catch (error) {
  console.error(`SUITE CUTOVER JSON: FAIL (${error instanceof Error ? error.message : String(error)})`);
  process.exit(1);
}
