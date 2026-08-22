#!/usr/bin/env node

// The alpha Foundry output layout no longer exists. Checked Suite ABIs and
// compiler artifacts are generated from one locked solc source of truth, so
// retain this entry point as a compatibility wrapper around that verifier.
import { spawnSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const currentCheck = resolve(scriptDir, "evm-suite-solc-check.mjs");
const result = spawnSync(process.execPath, [currentCheck], { stdio: "inherit" });

if (result.error) {
  console.error(`Suite ABI check: FAIL (${result.error.message})`);
  process.exit(1);
}
process.exit(result.status ?? 1);
