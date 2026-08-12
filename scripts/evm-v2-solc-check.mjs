#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const root = resolve(scriptDir, "..");
const contractDir = resolve(root, "contracts", "evm-v2");
const solcPackagePath = resolve(contractDir, "node_modules", "solc", "package.json");
const contractPath = resolve(root, "contracts", "evm-v2", "src", "RepoRegistryV2.sol");
const testPath = resolve(root, "contracts", "evm-v2", "test", "RepoRegistryV2.t.sol");
const controllerPath = resolve(root, "contracts", "evm-v2", "src", "RepoRegistryV2ImportController.sol");
const controllerTestPath = resolve(root, "contracts", "evm-v2", "test", "RepoRegistryV2ImportController.t.sol");
const badgePath = resolve(root, "contracts", "evm-v2", "src", "RepoRegistryV2BadgeModule.sol");
const badgeTestPath = resolve(root, "contracts", "evm-v2", "test", "RepoRegistryV2BadgeModule.t.sol");
const economicPath = resolve(root, "contracts", "evm-v2", "src", "RepoRegistryV2EconomicModule.sol");
const economicTestPath = resolve(root, "contracts", "evm-v2", "test", "RepoRegistryV2EconomicModule.t.sol");
const moderationPath = resolve(root, "contracts", "evm-v2", "src", "RepoRegistryV2ModerationModule.sol");
const moderationTestPath = resolve(root, "contracts", "evm-v2", "test", "RepoRegistryV2ModerationModule.t.sol");
const invariantTestPath = resolve(root, "contracts", "evm-v2", "test", "RepoRegistryV2Invariant.t.sol");
const gasTestPath = resolve(root, "contracts", "evm-v2", "test", "RepoRegistryV2Gas.t.sol");
const abiPath = resolve(root, "contracts", "evm-v2", "abi", "RepoRegistryV2.json");
const controllerAbiPath = resolve(root, "contracts", "evm-v2", "abi", "RepoRegistryV2ImportController.json");
const badgeAbiPath = resolve(root, "contracts", "evm-v2", "abi", "RepoRegistryV2BadgeModule.json");
const economicAbiPath = resolve(root, "contracts", "evm-v2", "abi", "RepoRegistryV2EconomicModule.json");
const moderationAbiPath = resolve(root, "contracts", "evm-v2", "abi", "RepoRegistryV2ModerationModule.json");
const writeAbi = process.argv.includes("--write-abi");
const solcScript = resolve(contractDir, "node_modules", "solc", "solc.js");
let solcPackage;
try {
  solcPackage = JSON.parse(readFileSync(solcPackagePath, "utf8"));
} catch (error) {
  console.error(`FAIL: locked solc package is unavailable (${error.message}); run npm ci --prefix contracts/evm-v2`);
  process.exit(1);
}
if (solcPackage.version !== "0.8.24") {
  console.error(`FAIL: locked solc version is ${solcPackage.version}, want exactly 0.8.24`);
  process.exit(1);
}

const input = {
  language: "Solidity",
  sources: {
    "src/RepoRegistryV2.sol": { content: readFileSync(contractPath, "utf8") },
    "test/RepoRegistryV2.t.sol": { content: readFileSync(testPath, "utf8") },
    "src/RepoRegistryV2ImportController.sol": { content: readFileSync(controllerPath, "utf8") },
    "test/RepoRegistryV2ImportController.t.sol": { content: readFileSync(controllerTestPath, "utf8") },
    "src/RepoRegistryV2BadgeModule.sol": { content: readFileSync(badgePath, "utf8") },
    "test/RepoRegistryV2BadgeModule.t.sol": { content: readFileSync(badgeTestPath, "utf8") },
    "src/RepoRegistryV2EconomicModule.sol": { content: readFileSync(economicPath, "utf8") },
    "test/RepoRegistryV2EconomicModule.t.sol": { content: readFileSync(economicTestPath, "utf8") },
    "src/RepoRegistryV2ModerationModule.sol": { content: readFileSync(moderationPath, "utf8") },
    "test/RepoRegistryV2ModerationModule.t.sol": { content: readFileSync(moderationTestPath, "utf8") },
    "test/RepoRegistryV2Invariant.t.sol": { content: readFileSync(invariantTestPath, "utf8") },
    "test/RepoRegistryV2Gas.t.sol": { content: readFileSync(gasTestPath, "utf8") },
  },
  settings: {
    optimizer: { enabled: true, runs: 1 },
    viaIR: true,
    outputSelection: {
      "*": {
        "*": ["abi", "evm.bytecode.object", "evm.deployedBytecode.object"],
      },
    },
  },
};

const result = spawnSync(process.execPath, [solcScript, "--standard-json"], {
  cwd: contractDir,
  input: JSON.stringify(input),
  encoding: "utf8",
  maxBuffer: 64 * 1024 * 1024,
});

if (result.error) {
  console.error(
    `FAIL: unable to run locked solc@0.8.24 (${result.error.message}); ` +
      "run npm ci --prefix contracts/evm-v2",
  );
  process.exit(1);
}
if (result.status !== 0 && !result.stdout) {
  process.stderr.write(result.stderr || "solc exited without output\n");
  process.exit(result.status || 1);
}

// solc-js may print an informational SMT line before its JSON payload.
const jsonStart = result.stdout.indexOf("{");
if (jsonStart < 0) {
  process.stderr.write(result.stderr || result.stdout || "solc returned no JSON\n");
  process.exit(1);
}

let output;
try {
  output = JSON.parse(result.stdout.slice(jsonStart));
} catch (error) {
  console.error(`FAIL: invalid solc JSON output: ${error.message}`);
  process.exit(1);
}

const diagnostics = output.errors || [];
for (const diagnostic of diagnostics) {
  const stream = diagnostic.severity === "error" ? process.stderr : process.stdout;
  stream.write(`${diagnostic.formattedMessage || diagnostic.message}\n`);
}

const errors = diagnostics.filter((diagnostic) => diagnostic.severity === "error");
if (errors.length > 0) {
  console.error(`SOLC CHECK: FAIL (${errors.length} error(s))`);
  process.exit(1);
}

const registry = output.contracts?.["src/RepoRegistryV2.sol"]?.RepoRegistryV2;
const coreTest = output.contracts?.["test/RepoRegistryV2.t.sol"]?.RepoRegistryV2Test;
const metadataTest = output.contracts?.["test/RepoRegistryV2.t.sol"]?.RepoRegistryV2MetadataTest;
const identityTest = output.contracts?.["test/RepoRegistryV2.t.sol"]?.RepoRegistryV2IdentityTest;
const paginationTest = output.contracts?.["test/RepoRegistryV2.t.sol"]?.RepoRegistryV2PaginationTest;
const importTest = output.contracts?.["test/RepoRegistryV2.t.sol"]?.RepoRegistryV2ImportTest;
const controller = output.contracts?.["src/RepoRegistryV2ImportController.sol"]?.RepoRegistryV2ImportController;
const controllerCoreTest = output.contracts?.["test/RepoRegistryV2ImportController.t.sol"]?.RepoRegistryV2ImportControllerCoreTest;
const controllerRecoveryTest = output.contracts?.["test/RepoRegistryV2ImportController.t.sol"]?.RepoRegistryV2ImportControllerRecoveryTest;
const badge = output.contracts?.["src/RepoRegistryV2BadgeModule.sol"]?.RepoRegistryV2BadgeModule;
const badgeTest = output.contracts?.["test/RepoRegistryV2BadgeModule.t.sol"]?.RepoRegistryV2BadgeModuleTest;
const economic = output.contracts?.["src/RepoRegistryV2EconomicModule.sol"]?.RepoRegistryV2EconomicModule;
const economicTest = output.contracts?.["test/RepoRegistryV2EconomicModule.t.sol"]?.RepoRegistryV2EconomicModuleTest;
const moderation = output.contracts?.["src/RepoRegistryV2ModerationModule.sol"]?.RepoRegistryV2ModerationModule;
const moderationTrailTest = output.contracts?.["test/RepoRegistryV2ModerationModule.t.sol"]?.RepoRegistryV2ModerationModuleTrailTest;
const moderationPolicyTest = output.contracts?.["test/RepoRegistryV2ModerationModule.t.sol"]?.RepoRegistryV2ModerationModulePolicyTest;
const invariantHandler = output.contracts?.["test/RepoRegistryV2Invariant.t.sol"]?.RepoRegistryV2Handler;
const invariantTest = output.contracts?.["test/RepoRegistryV2Invariant.t.sol"]?.RepoRegistryV2InvariantTest;
const gasTest = output.contracts?.["test/RepoRegistryV2Gas.t.sol"]?.RepoRegistryV2GasTest;
if (!registry || !coreTest || !metadataTest || !identityTest || !paginationTest || !importTest || !controller || !controllerCoreTest || !controllerRecoveryTest || !badge || !badgeTest || !economic || !economicTest || !moderation || !moderationTrailTest || !moderationPolicyTest || !invariantHandler || !invariantTest || !gasTest) {
  console.error("SOLC CHECK: FAIL (expected contract artifacts are missing)");
  process.exit(1);
}

const bytecodeBytes = (artifact) => artifact.evm.bytecode.object.length / 2;
const runtimeBytecodeBytes = (artifact) => artifact.evm.deployedBytecode.object.length / 2;
const maximumInitcodeBytes = 49_152;
const maximumRuntimeBytes = 24_576;
const minimumRegistryRuntimeHeadroomBytes = 128;
const registryRuntimeBytes = runtimeBytecodeBytes(registry);
const registryRuntimeHeadroomBytes = maximumRuntimeBytes - registryRuntimeBytes;
if (registryRuntimeBytes > maximumRuntimeBytes) {
  console.error(
    `FAIL: RepoRegistryV2 runtime is ${registryRuntimeBytes.toLocaleString("en-US")} bytes; ` +
      `EIP-170 maximum is ${maximumRuntimeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (registryRuntimeHeadroomBytes < minimumRegistryRuntimeHeadroomBytes) {
  console.error(
    `FAIL: RepoRegistryV2 runtime has only ${registryRuntimeHeadroomBytes.toLocaleString("en-US")} bytes ` +
      `of EIP-170 headroom; at least ${minimumRegistryRuntimeHeadroomBytes.toLocaleString("en-US")} bytes ` +
      "must remain. Move additional capabilities into a reviewed module.",
  );
  process.exit(1);
}
if (bytecodeBytes(registry) > maximumInitcodeBytes) {
  console.error(
    `FAIL: RepoRegistryV2 initcode is ${bytecodeBytes(registry).toLocaleString("en-US")} bytes; ` +
      `EIP-3860 maximum is ${maximumInitcodeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (runtimeBytecodeBytes(controller) > maximumRuntimeBytes) {
  console.error(
    `FAIL: RepoRegistryV2ImportController runtime is ${runtimeBytecodeBytes(controller).toLocaleString("en-US")} bytes; ` +
      `EIP-170 maximum is ${maximumRuntimeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (bytecodeBytes(controller) > maximumInitcodeBytes) {
  console.error(
    `FAIL: RepoRegistryV2ImportController initcode is ${bytecodeBytes(controller).toLocaleString("en-US")} bytes; ` +
      `maximum is ${maximumInitcodeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (runtimeBytecodeBytes(badge) > maximumRuntimeBytes) {
  console.error(
    `FAIL: RepoRegistryV2BadgeModule runtime is ${runtimeBytecodeBytes(badge).toLocaleString("en-US")} bytes; ` +
      `EIP-170 maximum is ${maximumRuntimeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (bytecodeBytes(badge) > maximumInitcodeBytes) {
  console.error(
    `FAIL: RepoRegistryV2BadgeModule initcode is ${bytecodeBytes(badge).toLocaleString("en-US")} bytes; ` +
      `maximum is ${maximumInitcodeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (runtimeBytecodeBytes(economic) > maximumRuntimeBytes) {
  console.error(
    `FAIL: RepoRegistryV2EconomicModule runtime is ${runtimeBytecodeBytes(economic).toLocaleString("en-US")} bytes; ` +
      `EIP-170 maximum is ${maximumRuntimeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (bytecodeBytes(economic) > maximumInitcodeBytes) {
  console.error(
    `FAIL: RepoRegistryV2EconomicModule initcode is ${bytecodeBytes(economic).toLocaleString("en-US")} bytes; ` +
      `maximum is ${maximumInitcodeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (runtimeBytecodeBytes(moderation) > maximumRuntimeBytes) {
  console.error(
    `FAIL: RepoRegistryV2ModerationModule runtime is ${runtimeBytecodeBytes(moderation).toLocaleString("en-US")} bytes; ` +
      `EIP-170 maximum is ${maximumRuntimeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
if (bytecodeBytes(moderation) > maximumInitcodeBytes) {
  console.error(
    `FAIL: RepoRegistryV2ModerationModule initcode is ${bytecodeBytes(moderation).toLocaleString("en-US")} bytes; ` +
      `EIP-3860 maximum is ${maximumInitcodeBytes.toLocaleString("en-US")} bytes`,
  );
  process.exit(1);
}
const oversizedTests = [
  ["RepoRegistryV2Test", coreTest],
  ["RepoRegistryV2MetadataTest", metadataTest],
  ["RepoRegistryV2IdentityTest", identityTest],
  ["RepoRegistryV2PaginationTest", paginationTest],
  ["RepoRegistryV2ImportTest", importTest],
  ["RepoRegistryV2ImportControllerCoreTest", controllerCoreTest],
  ["RepoRegistryV2ImportControllerRecoveryTest", controllerRecoveryTest],
  ["RepoRegistryV2BadgeModuleTest", badgeTest],
  ["RepoRegistryV2EconomicModuleTest", economicTest],
  ["RepoRegistryV2ModerationModuleTrailTest", moderationTrailTest],
  ["RepoRegistryV2ModerationModulePolicyTest", moderationPolicyTest],
  ["RepoRegistryV2Handler", invariantHandler],
  ["RepoRegistryV2InvariantTest", invariantTest],
  ["RepoRegistryV2GasTest", gasTest],
].filter(([, artifact]) => bytecodeBytes(artifact) > maximumInitcodeBytes);
if (oversizedTests.length > 0) {
  for (const [name, artifact] of oversizedTests) {
    console.error(
      `FAIL: ${name} initcode is ${bytecodeBytes(artifact).toLocaleString("en-US")} bytes; ` +
        `maximum is ${maximumInitcodeBytes.toLocaleString("en-US")} bytes`,
    );
  }
  process.exit(1);
}

const renderedAbi = `${JSON.stringify(registry.abi, null, 2)}\n`;
const renderedControllerAbi = `${JSON.stringify(controller.abi, null, 2)}\n`;
const renderedBadgeAbi = `${JSON.stringify(badge.abi, null, 2)}\n`;
const renderedEconomicAbi = `${JSON.stringify(economic.abi, null, 2)}\n`;
const renderedModerationAbi = `${JSON.stringify(moderation.abi, null, 2)}\n`;
if (writeAbi) {
  writeFileSync(abiPath, renderedAbi, "utf8");
  writeFileSync(controllerAbiPath, renderedControllerAbi, "utf8");
  writeFileSync(badgeAbiPath, renderedBadgeAbi, "utf8");
  writeFileSync(economicAbiPath, renderedEconomicAbi, "utf8");
  writeFileSync(moderationAbiPath, renderedModerationAbi, "utf8");
  console.log("ABI updated: contracts/evm-v2/abi/RepoRegistryV2.json");
  console.log("ABI updated: contracts/evm-v2/abi/RepoRegistryV2ImportController.json");
  console.log("ABI updated: contracts/evm-v2/abi/RepoRegistryV2BadgeModule.json");
  console.log("ABI updated: contracts/evm-v2/abi/RepoRegistryV2EconomicModule.json");
  console.log("ABI updated: contracts/evm-v2/abi/RepoRegistryV2ModerationModule.json");
} else if (readFileSync(abiPath, "utf8").replaceAll("\r\n", "\n") !== renderedAbi) {
  console.error(
    "SOLC CHECK: FAIL (contracts/evm-v2/abi/RepoRegistryV2.json is stale; " +
      "run node scripts/evm-v2-solc-check.mjs --write-abi)",
  );
  process.exit(1);
} else if (readFileSync(controllerAbiPath, "utf8").replaceAll("\r\n", "\n") !== renderedControllerAbi) {
  console.error("SOLC CHECK: FAIL (controller ABI is stale; run node scripts/evm-v2-solc-check.mjs --write-abi)");
  process.exit(1);
} else if (readFileSync(badgeAbiPath, "utf8").replaceAll("\r\n", "\n") !== renderedBadgeAbi) {
  console.error("SOLC CHECK: FAIL (badge ABI is stale; run node scripts/evm-v2-solc-check.mjs --write-abi)");
  process.exit(1);
} else if (readFileSync(economicAbiPath, "utf8").replaceAll("\r\n", "\n") !== renderedEconomicAbi) {
  console.error("SOLC CHECK: FAIL (economic ABI is stale; run node scripts/evm-v2-solc-check.mjs --write-abi)");
  process.exit(1);
} else if (readFileSync(moderationAbiPath, "utf8").replaceAll("\r\n", "\n") !== renderedModerationAbi) {
  console.error("SOLC CHECK: FAIL (moderation ABI is stale; run node scripts/evm-v2-solc-check.mjs --write-abi)");
  process.exit(1);
}

console.log(`ABI entries: ${registry.abi.length}`);
console.log(`main initcode: ${bytecodeBytes(registry).toLocaleString("en-US")} bytes`);
console.log(`main runtime: ${registryRuntimeBytes.toLocaleString("en-US")} bytes`);
console.log(`main runtime headroom: ${registryRuntimeHeadroomBytes.toLocaleString("en-US")} bytes`);
console.log(`controller initcode: ${bytecodeBytes(controller).toLocaleString("en-US")} bytes`);
console.log(`controller runtime: ${runtimeBytecodeBytes(controller).toLocaleString("en-US")} bytes`);
console.log(`badge module initcode: ${bytecodeBytes(badge).toLocaleString("en-US")} bytes`);
console.log(`badge module runtime: ${runtimeBytecodeBytes(badge).toLocaleString("en-US")} bytes`);
console.log(`economic module initcode: ${bytecodeBytes(economic).toLocaleString("en-US")} bytes`);
console.log(`economic module runtime: ${runtimeBytecodeBytes(economic).toLocaleString("en-US")} bytes`);
console.log(`moderation module initcode: ${bytecodeBytes(moderation).toLocaleString("en-US")} bytes`);
console.log(`moderation module runtime: ${runtimeBytecodeBytes(moderation).toLocaleString("en-US")} bytes`);
console.log(`core test bytecode: ${bytecodeBytes(coreTest).toLocaleString("en-US")} bytes`);
console.log(`metadata test bytecode: ${bytecodeBytes(metadataTest).toLocaleString("en-US")} bytes`);
console.log(`identity test bytecode: ${bytecodeBytes(identityTest).toLocaleString("en-US")} bytes`);
console.log(`pagination test bytecode: ${bytecodeBytes(paginationTest).toLocaleString("en-US")} bytes`);
console.log(`import test bytecode: ${bytecodeBytes(importTest).toLocaleString("en-US")} bytes`);
console.log(`controller core test bytecode: ${bytecodeBytes(controllerCoreTest).toLocaleString("en-US")} bytes`);
console.log(`controller recovery test bytecode: ${bytecodeBytes(controllerRecoveryTest).toLocaleString("en-US")} bytes`);
console.log(`badge module test bytecode: ${bytecodeBytes(badgeTest).toLocaleString("en-US")} bytes`);
console.log(`economic module test bytecode: ${bytecodeBytes(economicTest).toLocaleString("en-US")} bytes`);
console.log(`moderation trail test bytecode: ${bytecodeBytes(moderationTrailTest).toLocaleString("en-US")} bytes`);
console.log(`moderation policy test bytecode: ${bytecodeBytes(moderationPolicyTest).toLocaleString("en-US")} bytes`);
console.log(`stateful invariant handler bytecode: ${bytecodeBytes(invariantHandler).toLocaleString("en-US")} bytes`);
console.log(`stateful invariant test bytecode: ${bytecodeBytes(invariantTest).toLocaleString("en-US")} bytes`);
console.log(`gas ceiling test bytecode: ${bytecodeBytes(gasTest).toLocaleString("en-US")} bytes`);
console.log("Solidity errors: 0");
console.log("SOLC CHECK: PASS");
