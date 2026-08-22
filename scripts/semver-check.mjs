#!/usr/bin/env node

import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

// Strict SemVer 2.0.0, with the repository's required leading `v`.
const semanticVersion =
  /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/;

export function isReleaseVersion(value) {
  return typeof value === "string" && semanticVersion.test(value);
}

function main(args) {
  if (args.length !== 1) {
    console.error("usage: semver-check.mjs <vMAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]>");
    return 2;
  }
  if (!isReleaseVersion(args[0])) {
    console.error(`release tag is not strict SemVer 2.0.0 with a leading v: ${args[0]}`);
    return 1;
  }
  console.log(`release tag check: pass (${args[0]})`);
  return 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  process.exitCode = main(process.argv.slice(2));
}
