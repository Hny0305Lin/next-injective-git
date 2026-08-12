#!/usr/bin/env node

import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const REQUIRED_DECLARATIONS = ["NETWORK_PROFILES", "DEFAULT_PROFILE"];

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
  ) current = current.expression;
  return current;
}

function staticPropertyName(name, context) {
  if (ts.isComputedPropertyName(name)) fail(`${context} must not use a computed property name`);
  if (ts.isIdentifier(name) || ts.isStringLiteral(name) || ts.isNumericLiteral(name)) return name.text;
  fail(`${context} has an unsupported property name`);
}

function stringLiteralValue(expression, context) {
  const value = unwrapExpression(expression);
  if (!ts.isStringLiteral(value)) fail(`${context} must be a string literal`);
  return value.text;
}

function objectLiteralValue(expression, context) {
  const value = unwrapExpression(expression);
  if (!ts.isObjectLiteralExpression(value)) fail(`${context} must be an object literal`);
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
  if (matches.length !== 1) fail(`${name} must have exactly one top-level declaration`);
  const match = matches[0];
  if ((match.declarationList.flags & ts.NodeFlags.Const) === 0) fail(`${name} must be declared with const`);
  if (!match.declaration.initializer) fail(`${name} must have an initializer`);
  return match.declaration.initializer;
}

function parseSource(source, fileName) {
  const sourceFile = ts.createSourceFile(fileName, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  const diagnostics = sourceFile.parseDiagnostics ?? [];
  if (diagnostics.length > 0) {
    fail(`${fileName} has a TypeScript parse error: ${ts.flattenDiagnosticMessageText(diagnostics[0].messageText, "\n")}`);
  }
  return sourceFile;
}

export function validateReleaseProfileSource(source, fileName = "profile.ts") {
  const sourceFile = parseSource(source, fileName);
  const declarations = new Map(REQUIRED_DECLARATIONS.map((name) => [name, topLevelConst(sourceFile, name)]));
  const defaultProfile = stringLiteralValue(declarations.get("DEFAULT_PROFILE"), "DEFAULT_PROFILE");
  const profileMap = objectLiteralValue(declarations.get("NETWORK_PROFILES"), "NETWORK_PROFILES");
  const profileIds = [];
  const seen = new Set();

  for (const property of profileMap.properties) {
    if (!ts.isPropertyAssignment(property)) {
      fail("NETWORK_PROFILES may contain only static property assignments");
    }
    const profileId = staticPropertyName(property.name, "NETWORK_PROFILES");
    if (seen.has(profileId)) fail(`NETWORK_PROFILES contains duplicate profile ${JSON.stringify(profileId)}`);
    seen.add(profileId);
    profileIds.push(profileId);
    const profile = objectLiteralValue(property.initializer, `NETWORK_PROFILES[${JSON.stringify(profileId)}]`);
    const directoryFields = [];
    for (const field of profile.properties) {
      if (!ts.isPropertyAssignment(field)) {
        fail(`NETWORK_PROFILES[${JSON.stringify(profileId)}] may contain only static property assignments`);
      }
      if (staticPropertyName(field.name, `NETWORK_PROFILES[${JSON.stringify(profileId)}]`) === "suiteDirectory") {
        directoryFields.push(field.initializer);
      }
    }
    if (directoryFields.length !== 1) {
      fail(`NETWORK_PROFILES[${JSON.stringify(profileId)}] must define suiteDirectory exactly once`);
    }
    const directory = stringLiteralValue(
      directoryFields[0],
      `NETWORK_PROFILES[${JSON.stringify(profileId)}].suiteDirectory`,
    );
    if (directory !== "") {
      fail(`NETWORK_PROFILES[${JSON.stringify(profileId)}].suiteDirectory must remain empty before cutover evidence approval`);
    }
  }
  if (!seen.has(defaultProfile)) {
    fail(`DEFAULT_PROFILE ${JSON.stringify(defaultProfile)} is not a key in NETWORK_PROFILES`);
  }
  return { defaultProfile, profileIds };
}

export async function validateReleaseProfileFile(path) {
  return validateReleaseProfileSource(await readFile(path, "utf8"), path);
}

async function main() {
  const sourcePath = process.argv[2]
    ? resolve(process.cwd(), process.argv[2])
    : fileURLToPath(new URL("../src/lib/profile.ts", import.meta.url));
  try {
    const result = await validateReleaseProfileFile(sourcePath);
    console.log(`WEB RELEASE PROFILE: PASS (default=${result.defaultProfile}, profiles=${result.profileIds.length})`);
  } catch (error) {
    console.error(`WEB RELEASE PROFILE: FAIL: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main();
