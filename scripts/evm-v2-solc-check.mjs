#!/usr/bin/env node

// Compatibility entry point retained for local automation that used the
// foundation-era filename. The immutable Suite owns the authoritative check.
import { spawnSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const currentCheck = resolve(scriptDir, "evm-suite-solc-check.mjs");
const result = spawnSync(process.execPath, [currentCheck, ...process.argv.slice(2)], { stdio: "inherit" });

if (result.error) {
  console.error(`EVM SUITE SOLC CHECK: FAIL (${result.error.message})`);
  process.exit(1);
}
process.exit(result.status ?? 1);
