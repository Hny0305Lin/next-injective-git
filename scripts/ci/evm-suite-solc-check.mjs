#!/usr/bin/env node

import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const _candidate1 = resolve(scriptDir, "../..");
const _candidate2 = resolve(scriptDir, "..");
const root = existsSync(resolve(_candidate1, "CLAUDE.md")) ? _candidate1 : _candidate2;
const contractDir = resolve(root, "contracts", "evm-v2");
const srcDir = resolve(contractDir, "src");
const artifactDir = resolve(contractDir, "artifacts");
const testPath = resolve(contractDir, "test", "SuiteArchitecture.t.sol");
const solcPackagePath = resolve(contractDir, "node_modules", "solc", "package.json");
const writeGenerated = process.argv.includes("--write-abi") || process.argv.includes("--write-artifacts");
const require = createRequire(import.meta.url);

const productionContracts = new Map([
  ["SuiteDirectory", "src/SuiteDirectory.sol"],
  ["BootstrapCoordinator", "src/BootstrapCoordinator.sol"],
  ["RepositoryCore", "src/modules/RepositoryCore.sol"],
  ["RecoveryModule", "src/modules/RecoveryModule.sol"],
  ["ModerationModule", "src/modules/ModerationModule.sol"],
  ["EconomicModule", "src/modules/EconomicModule.sol"],
  ["UsernameModule", "src/modules/UsernameModule.sol"],
  ["BadgeModule", "src/modules/BadgeModule.sol"],
  ["ReleaseModule", "src/modules/ReleaseModule.sol"],
]);

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name);
    return entry.isDirectory() ? walk(path) : [path];
  });
}

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

const productionFiles = walk(srcDir).filter((path) => path.endsWith(".sol"));
const sources = {};
for (const path of [...productionFiles, testPath]) {
  const name = relative(contractDir, path).replaceAll("\\", "/");
  const content = readFileSync(path, "utf8");
  if (productionFiles.includes(path) && /\b(delegatecall|selfdestruct)\b/i.test(content)) {
    console.error(`FAIL: forbidden upgrade/destructive opcode primitive found in ${name}`);
    process.exit(1);
  }
  sources[name] = { content };
}

const input = {
  language: "Solidity",
  sources,
  settings: {
    optimizer: { enabled: true, runs: 1 },
    viaIR: true,
    outputSelection: {
      "*": {
        "*": [
          "abi",
          "evm.bytecode.object",
          "evm.deployedBytecode.object",
          "evm.deployedBytecode.immutableReferences",
        ],
      },
    },
  },
};

let output;
try {
  const solc = require(resolve(contractDir, "node_modules", "solc"));
  output = JSON.parse(solc.compile(JSON.stringify(input)));
} catch (error) {
  console.error(`FAIL: unable to run locked solc@0.8.24 (${error.message})`);
  process.exit(1);
}
for (const diagnostic of output.errors || []) {
  const stream = diagnostic.severity === "error" ? process.stderr : process.stdout;
  stream.write(`${diagnostic.formattedMessage || diagnostic.message}\n`);
}
if ((output.errors || []).some((diagnostic) => diagnostic.severity === "error")) {
  console.error("EVM SUITE SOLC CHECK: FAIL");
  process.exit(1);
}

const maximumInitcodeBytes = 49_152;
const maximumRuntimeBytes = 24_576;
const minimumCoreRuntimeHeadroomBytes = 1_024;
const bytecodeBytes = (artifact) => artifact.evm.bytecode.object.length / 2;
const runtimeBytes = (artifact) => artifact.evm.deployedBytecode.object.length / 2;

for (const [contractName, sourceName] of productionContracts) {
  const artifact = output.contracts?.[sourceName]?.[contractName];
  if (!artifact) {
    console.error(`FAIL: missing compiled artifact ${sourceName}:${contractName}`);
    process.exit(1);
  }
  const initcode = bytecodeBytes(artifact);
  const runtime = runtimeBytes(artifact);
  if (initcode > maximumInitcodeBytes) {
    console.error(`FAIL: ${contractName} initcode is ${initcode} bytes; maximum is ${maximumInitcodeBytes}`);
    process.exit(1);
  }
  if (runtime > maximumRuntimeBytes) {
    console.error(`FAIL: ${contractName} runtime is ${runtime} bytes; maximum is ${maximumRuntimeBytes}`);
    process.exit(1);
  }
  if (contractName === "RepositoryCore" && maximumRuntimeBytes - runtime < minimumCoreRuntimeHeadroomBytes) {
    console.error(
      `FAIL: RepositoryCore has only ${maximumRuntimeBytes - runtime} bytes of EIP-170 headroom; ` +
        `at least ${minimumCoreRuntimeHeadroomBytes} are required`,
    );
    process.exit(1);
  }

  const abiPath = resolve(contractDir, "abi", `${contractName}.json`);
  const rendered = `${JSON.stringify(artifact.abi, null, 2)}\n`;
  if (writeGenerated) {
    writeFileSync(abiPath, rendered, "utf8");
  } else {
    let committed;
    try {
      committed = readFileSync(abiPath, "utf8").replaceAll("\r\n", "\n");
    } catch (error) {
      console.error(`FAIL: missing checked-in ABI ${relative(root, abiPath)} (${error.message})`);
      process.exit(1);
    }
    if (committed !== rendered) {
      console.error(`FAIL: ${relative(root, abiPath)} is stale; run this script with --write-abi`);
      process.exit(1);
    }
  }

  const sourceSHA256 = createHash("sha256")
    .update(readFileSync(resolve(contractDir, sourceName)))
    .digest("hex");
  const creationBytecode = `0x${artifact.evm.bytecode.object}`;
  const runtimeTemplate = `0x${artifact.evm.deployedBytecode.object}`;
  const checkedArtifact = {
    schema: "igit.evm-suite.solc-artifact.v1",
    contract_name: contractName,
    source_name: sourceName,
    source_sha256: sourceSHA256,
    compiler: {
      version: solcPackage.version,
      settings: {
        optimizer: { enabled: true, runs: 1 },
        via_ir: true,
        evm_version: "default",
      },
    },
    abi: artifact.abi,
    creation_bytecode: creationBytecode,
    creation_bytecode_sha256: createHash("sha256").update(Buffer.from(artifact.evm.bytecode.object, "hex")).digest("hex"),
    runtime_template: runtimeTemplate,
    runtime_template_sha256: createHash("sha256").update(Buffer.from(artifact.evm.deployedBytecode.object, "hex")).digest("hex"),
    immutable_references: artifact.evm.deployedBytecode.immutableReferences || {},
  };
  const artifactPath = resolve(artifactDir, `${contractName}.json`);
  const renderedArtifact = `${JSON.stringify(checkedArtifact, null, 2)}\n`;
  if (writeGenerated) {
    mkdirSync(artifactDir, { recursive: true });
    writeFileSync(artifactPath, renderedArtifact, "utf8");
  } else {
    let committed;
    try {
      committed = readFileSync(artifactPath, "utf8").replaceAll("\r\n", "\n");
    } catch (error) {
      console.error(`FAIL: missing checked-in artifact ${relative(root, artifactPath)} (${error.message})`);
      process.exit(1);
    }
    if (committed !== renderedArtifact) {
      console.error(`FAIL: ${relative(root, artifactPath)} is stale; run this script with --write-artifacts`);
      process.exit(1);
    }
  }
  console.log(`${contractName}: initcode=${initcode} runtime=${runtime}`);
}

const suiteTest = output.contracts?.["test/SuiteArchitecture.t.sol"]?.SuiteArchitectureTest;
if (!suiteTest) {
  console.error("FAIL: SuiteArchitectureTest was not compiled");
  process.exit(1);
}
const suiteSecurityTest = output.contracts?.["test/SuiteArchitecture.t.sol"]?.SuiteArchitectureSecurityTest;
if (!suiteSecurityTest) {
  console.error("FAIL: SuiteArchitectureSecurityTest was not compiled");
  process.exit(1);
}
for (const [testName, testArtifact] of [
  ["SuiteArchitectureTest", suiteTest],
  ["SuiteArchitectureSecurityTest", suiteSecurityTest],
]) {
  if (bytecodeBytes(testArtifact) > maximumInitcodeBytes) {
    console.error(`FAIL: ${testName} initcode is ${bytecodeBytes(testArtifact)} bytes; use artifact deployment`);
    process.exit(1);
  }
  console.log(`${testName}: initcode=${bytecodeBytes(testArtifact)} runtime=${runtimeBytes(testArtifact)}`);
}

if (writeGenerated) console.log("Suite ABIs and solc artifacts updated");
console.log("EVM SUITE SOLC CHECK: PASS");
