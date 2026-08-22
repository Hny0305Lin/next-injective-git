#!/usr/bin/env node
// Compatibility shim — moved to scripts/ci/semver-check.test.mjs (remove after one release cycle)
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const target = path.join(__dirname, "ci/semver-check.test.mjs");
const result = spawnSync(process.execPath, [target, ...process.argv.slice(2)], { stdio: "inherit" });
process.exit(result.status ?? 0);
