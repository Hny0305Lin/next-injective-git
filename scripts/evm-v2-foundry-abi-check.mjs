#!/usr/bin/env node

import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const contractDir = resolve(process.argv[2] || "contracts/evm-v2");
const pairs = [
  ["out/RepoRegistryV2.sol/RepoRegistryV2.json", "abi/RepoRegistryV2.json"],
  ["out/RepoRegistryV2ImportController.sol/RepoRegistryV2ImportController.json", "abi/RepoRegistryV2ImportController.json"],
  ["out/RepoRegistryV2BadgeModule.sol/RepoRegistryV2BadgeModule.json", "abi/RepoRegistryV2BadgeModule.json"],
  ["out/RepoRegistryV2EconomicModule.sol/RepoRegistryV2EconomicModule.json", "abi/RepoRegistryV2EconomicModule.json"],
  ["out/RepoRegistryV2ModerationModule.sol/RepoRegistryV2ModerationModule.json", "abi/RepoRegistryV2ModerationModule.json"],
];

function canonicalValue(value) {
  if (Array.isArray(value)) return value.map(canonicalValue);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.keys(value).sort().map((key) => [key, canonicalValue(value[key])]),
    );
  }
  return value;
}

function canonicalAbi(abi) {
  return abi
    .map((entry) => canonicalValue(entry))
    .sort((left, right) => JSON.stringify(left).localeCompare(JSON.stringify(right)));
}

for (const [artifactPath, committedPath] of pairs) {
  let artifact;
  let committed;
  try {
    artifact = JSON.parse(readFileSync(resolve(contractDir, artifactPath), "utf8"));
    committed = JSON.parse(readFileSync(resolve(contractDir, committedPath), "utf8"));
  } catch (error) {
    console.error(`FAIL: unable to parse EVM V2 ABI evidence: ${error.message}`);
    process.exit(1);
  }
  if (!Array.isArray(artifact.abi) || !Array.isArray(committed)) {
    console.error(`FAIL: malformed EVM V2 ABI evidence: contracts/evm-v2/${committedPath}`);
    process.exit(1);
  }
  if (JSON.stringify(canonicalAbi(artifact.abi)) !== JSON.stringify(canonicalAbi(committed))) {
    console.error(`FAIL: contracts/evm-v2/${committedPath} is stale`);
    process.exit(1);
  }
}

console.log("Foundry artifact ABI comparison: pass");
