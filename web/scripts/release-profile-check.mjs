#!/usr/bin/env node

import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import ts from "typescript";

const REQUIRED_DECLARATIONS = [
  "NETWORK_PROFILES",
  "DEFAULT_PROFILE",
  "DEFAULT_CONTRACT_VERSION",
];

function fail(message) {
  throw new Error(message);
}

function unwrapExpression(expression) {
  let current = expression;
  while (
    ts.isParenthesizedExpression(current) ||
    ts.isAsExpression(current) ||
    ts.isTypeAssertionExpression(current) ||
    ts.isSatisfiesExpression(current) ||
    ts.isNonNullExpression(current)
  ) {
    current = current.expression;
  }
  return current;
}

function staticPropertyName(name, context) {
  if (ts.isComputedPropertyName(name)) {
    fail(`${context} must not use a computed property name`);
  }
  if (ts.isIdentifier(name) || ts.isStringLiteral(name) || ts.isNumericLiteral(name)) {
    return name.text;
  }
  fail(`${context} has an unsupported property name`);
}

function stringLiteralValue(expression, context) {
  const value = unwrapExpression(expression);
  if (!ts.isStringLiteral(value)) {
    fail(`${context} must be a string literal`);
  }
  return value.text;
}

function objectLiteralValue(expression, context) {
  const value = unwrapExpression(expression);
  if (!ts.isObjectLiteralExpression(value)) {
    fail(`${context} must be an object literal`);
  }
  return value;
}

function topLevelConst(sourceFile, name) {
  const matches = [];
  for (const statement of sourceFile.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (ts.isIdentifier(declaration.name) && declaration.name.text === name) {
        matches.push({ declaration, declarationList: statement.declarationList });
      }
    }
  }

  if (matches.length !== 1) {
    fail(`${name} must have exactly one top-level declaration`);
  }
  const match = matches[0];
  if ((match.declarationList.flags & ts.NodeFlags.Const) === 0) {
    fail(`${name} must be declared with const`);
  }
  if (!match.declaration.initializer) {
    fail(`${name} must have an initializer`);
  }
  return match.declaration.initializer;
}

function parseSource(source, fileName) {
  const sourceFile = ts.createSourceFile(
    fileName,
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );
  const diagnostics = sourceFile.parseDiagnostics ?? [];
  if (diagnostics.length > 0) {
    const message = ts.flattenDiagnosticMessageText(diagnostics[0].messageText, "\n");
    fail(`${fileName} has a TypeScript parse error: ${message}`);
  }
  return sourceFile;
}

export function validateReleaseProfileSource(source, fileName = "chain.ts") {
  const sourceFile = parseSource(source, fileName);
  const declarations = new Map(
    REQUIRED_DECLARATIONS.map((name) => [name, topLevelConst(sourceFile, name)]),
  );

  const defaultProfile = stringLiteralValue(
    declarations.get("DEFAULT_PROFILE"),
    "DEFAULT_PROFILE",
  );
  const defaultContractVersion = stringLiteralValue(
    declarations.get("DEFAULT_CONTRACT_VERSION"),
    "DEFAULT_CONTRACT_VERSION",
  );
  if (defaultContractVersion !== "auto") {
    fail('DEFAULT_CONTRACT_VERSION must be the string literal "auto"');
  }

  const profileMap = objectLiteralValue(
    declarations.get("NETWORK_PROFILES"),
    "NETWORK_PROFILES",
  );
  const profileIds = [];
  const seenProfiles = new Set();

  for (const property of profileMap.properties) {
    if (!ts.isPropertyAssignment(property)) {
      fail("NETWORK_PROFILES may contain only static property assignments");
    }
    const profileId = staticPropertyName(property.name, "NETWORK_PROFILES");
    if (seenProfiles.has(profileId)) {
      fail(`NETWORK_PROFILES contains duplicate profile ${JSON.stringify(profileId)}`);
    }
    seenProfiles.add(profileId);
    profileIds.push(profileId);

    const profile = objectLiteralValue(
      property.initializer,
      `NETWORK_PROFILES[${JSON.stringify(profileId)}]`,
    );
    const evmContracts = [];
    const evmBadgeModules = [];
    const evmEconomicModules = [];
    for (const field of profile.properties) {
      if (!ts.isPropertyAssignment(field)) {
        fail(
          `NETWORK_PROFILES[${JSON.stringify(profileId)}] may contain only static property assignments`,
        );
      }
      const fieldName = staticPropertyName(
        field.name,
        `NETWORK_PROFILES[${JSON.stringify(profileId)}]`,
      );
      if (fieldName === "evmContract") evmContracts.push(field.initializer);
      if (fieldName === "evmBadgeModule") evmBadgeModules.push(field.initializer);
      if (fieldName === "evmEconomicModule") evmEconomicModules.push(field.initializer);
    }
    if (evmContracts.length !== 1) {
      fail(
        `NETWORK_PROFILES[${JSON.stringify(profileId)}] must define evmContract exactly once`,
      );
    }
    const evmContract = stringLiteralValue(
      evmContracts[0],
      `NETWORK_PROFILES[${JSON.stringify(profileId)}].evmContract`,
    );
    if (evmContract !== "") {
      fail(
        `NETWORK_PROFILES[${JSON.stringify(profileId)}].evmContract must be the empty string literal`,
      );
    }
    if (evmBadgeModules.length !== 1) {
      fail(
        `NETWORK_PROFILES[${JSON.stringify(profileId)}] must define evmBadgeModule exactly once`,
      );
    }
    const evmBadgeModule = stringLiteralValue(
      evmBadgeModules[0],
      `NETWORK_PROFILES[${JSON.stringify(profileId)}].evmBadgeModule`,
    );
    if (evmBadgeModule !== "") {
      fail(
        `NETWORK_PROFILES[${JSON.stringify(profileId)}].evmBadgeModule must be the empty string literal`,
      );
    }
    if (evmEconomicModules.length !== 1) {
      fail(
        `NETWORK_PROFILES[${JSON.stringify(profileId)}] must define evmEconomicModule exactly once`,
      );
    }
    const evmEconomicModule = stringLiteralValue(
      evmEconomicModules[0],
      `NETWORK_PROFILES[${JSON.stringify(profileId)}].evmEconomicModule`,
    );
    if (evmEconomicModule !== "") {
      fail(
        `NETWORK_PROFILES[${JSON.stringify(profileId)}].evmEconomicModule must be the empty string literal`,
      );
    }
  }

  if (!seenProfiles.has(defaultProfile)) {
    fail(
      `DEFAULT_PROFILE ${JSON.stringify(defaultProfile)} is not a key in NETWORK_PROFILES`,
    );
  }

  return { defaultProfile, defaultContractVersion, profileIds };
}

export async function validateReleaseProfileFile(path) {
  const source = await readFile(path, "utf8");
  return validateReleaseProfileSource(source, path);
}

async function main() {
  const sourcePath = process.argv[2]
    ? resolve(process.cwd(), process.argv[2])
    : fileURLToPath(new URL("../src/lib/chain.ts", import.meta.url));
  try {
    const result = await validateReleaseProfileFile(sourcePath);
    console.log(
      `WEB RELEASE PROFILE: PASS (default=${result.defaultProfile}, ` +
      `version=${result.defaultContractVersion}, profiles=${result.profileIds.length})`,
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`WEB RELEASE PROFILE: FAIL (${message})`);
    process.exitCode = 1;
  }
}

const invokedPath = process.argv[1] ? pathToFileURL(resolve(process.argv[1])).href : "";
if (invokedPath === import.meta.url) {
  await main();
}
