import canonicalize from 'canonicalize';

export const MAX_MANIFEST_BYTES = 64 * 1024;
export const MAX_PACK_BYTES = 512 * 1024 * 1024;
export const MAX_TOTAL_BYTES = 2 * 1024 * 1024 * 1024;
const encoder = new TextEncoder();
const invalid = (): never => { throw new Error('Invalid manifest JSON, schema, context, commitment or limits'); };
function unicode(s: string): boolean {
  for (let i = 0; i < s.length; i++) {
    const n = s.charCodeAt(i);
    if (n >= 0xdc00 && n <= 0xdfff) return false;
    if (n >= 0xd800 && n <= 0xdbff) { const low = s.charCodeAt(++i); if (!(low >= 0xdc00 && low <= 0xdfff)) return false; }
  }
  return true;
}
// Parse tokens before JSON.parse could discard duplicate keys. Scalar strings
// and numbers use ECMAScript semantics; canonicalization uses the JCS library.
export function strictJSON(bytes: Uint8Array, limit = MAX_MANIFEST_BYTES): unknown {
  if (!bytes.length || bytes.length > limit || (bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf)) invalid();
  let text: string;
  try { text = new TextDecoder('utf-8', { fatal: true }).decode(bytes); } catch { return invalid(); }
  let i = 0;
  const ws = () => { while (' \t\r\n'.includes(text[i] ?? '\0')) i++; };
  const str = (): string => {
    const start = i++;
    while (i < text.length) {
      if (text[i] === '\\') { i += 2; continue; }
      if (text[i++] === '"') { let s: string; try { s = JSON.parse(text.slice(start, i)); } catch { return invalid(); } if (!unicode(s)) invalid(); return s; }
    }
    return invalid();
  };
  const value = (depth: number): unknown => {
    if (depth > 16) invalid(); ws(); const c = text[i];
    if (c === '"') return str();
    if (c === '{') {
      i++; ws(); const o: Record<string, unknown> = Object.create(null); const seen = new Set<string>();
      if (text[i] !== '}') while (true) {
        ws(); if (text[i] !== '"') invalid(); const key = str(); if (seen.has(key)) invalid(); seen.add(key);
        ws(); if (text[i++] !== ':') invalid(); o[key] = value(depth + 1); ws(); if (text[i] !== ',') break; i++;
      }
      if (text[i++] !== '}') invalid(); return o;
    }
    if (c === '[') {
      i++; ws(); const a: unknown[] = [];
      if (text[i] !== ']') while (true) { a.push(value(depth + 1)); ws(); if (text[i] !== ',') break; i++; }
      if (text[i++] !== ']') invalid(); return a;
    }
    const token = /^(?:true|false|null|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)/.exec(text.slice(i));
    if (!token) return invalid(); i += token[0].length;
    const v: unknown = JSON.parse(token[0]); if (typeof v === 'number' && !Number.isFinite(v)) invalid(); return v;
  };
  const result = value(0); ws(); if (i !== text.length) invalid(); return result;
}
export function canonicalJSON(bytes: Uint8Array): Uint8Array {
  return encoder.encode(canonicalize(strictJSON(bytes)) ?? invalid());
}
export type Commit = { algorithm: 'sha1'; oid: string };
export type ManifestContext = { chainId: string; suiteDirectory: string; repoId: string; refName: string; commit: Commit };
export type PackLocation = { provider: 'aws-s3' | 'cloudflare-r2' | 'ipfs'; url: string; reader: string };
export type PackEntry = { sequence: number; sha256: string; size: string; format: 'git-pack'; packVersion: 2; thin: false; dependsOn: string[]; locations: PackLocation[] };
export type PackManifest = ManifestContext & { schema: 'igit.pack-manifest'; schemaVersion: 1 | 2; packs: PackEntry[] };
export type ManifestCommitment = { sha256: string; size: string; bootstrapLocator: string };
function obj(v: unknown, keys: string[]): Record<string, unknown> {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return invalid();
  const o = v as Record<string, unknown>;
  const actual = Object.keys(o).sort(); const expected = [...keys].sort();
  if (actual.length !== expected.length || !actual.every((key, i) => key === expected[i])) invalid(); return o;
}
function match(v: unknown, re: RegExp): v is string { return typeof v === 'string' && unicode(v) && re.exec(v)?.[0] === v; }
function size(v: unknown, max: bigint): bigint {
  if (!match(v, /^[1-9][0-9]*$/) || v.length > 78) return invalid();
  const n = BigInt(v); if (n > max) invalid(); return n;
}
export function validPublicURL(raw: string): boolean {
  if (typeof raw !== 'string' || raw.length > 2048) return false;
  // Keep the exact wire spelling; URL() alone normalizes traversal and ports.
  const m = /^https:\/\/([a-z0-9]+[a-z0-9.-]*)((?:\/[a-zA-Z0-9_-][a-zA-Z0-9_.-]*)*\/?)$/.exec(raw);
  if (!m || m[0] !== raw || raw.length > 2048) return false;
  const host = m[1]; if (host.length > 253 || !host.includes('.') || /^[0-9.]+$/.test(host) || /\.(localhost|local|internal|test|invalid)$/.test(host)) return false;
  if (host.split('.').some(p => !p || p.length > 63 || p.startsWith('-') || p.endsWith('-'))) return false;
  if (!/^[a-z][a-z0-9-]*[a-z0-9]$/.test(host.split('.').at(-1)!)) return false;
  try { const u = new URL(raw); return u.hostname === host && u.pathname === (m[2] || '/'); } catch { return false; }
}
function context(v: Record<string, unknown>): void {
  size(v.chainId, (1n << 256n) - 1n);
  if (!match(v.suiteDirectory, /^0x[0-9a-f]{40}$/) || !match(v.repoId, /^0x[0-9a-f]{64}$/)) invalid();
  if (!match(v.refName, /^refs\/(heads|tags)\/.+$/)) invalid(); const ref = v.refName as string;
  if (encoder.encode(ref).length > 255 || /[\x00-\x20\x7f~^:?*\[\\]/.test(ref) || ref.includes('..') || ref.includes('@{') || ref.endsWith('.') || ref.split('/').some(p => !p || p.startsWith('.') || p.endsWith('.lock'))) invalid();
  const c = obj(v.commit, ['algorithm', 'oid']); if (c.algorithm !== 'sha1' || !match(c.oid, /^[0-9a-f]{40}$/) || c.oid === '0'.repeat(40)) invalid();
}
export function validateManifest(input: unknown): PackManifest {
  const m = obj(input, ['schema','schemaVersion','chainId','suiteDirectory','repoId','refName','commit','packs']);
  context(m); if (m.schema !== 'igit.pack-manifest' || (m.schemaVersion !== 1 && m.schemaVersion !== 2) || !Array.isArray(m.packs) || !m.packs.length || m.packs.length > 16) invalid();
  let total = 0n; const digests = new Set<string>();
  Array.from(m.packs as unknown[]).forEach((v, i) => {
    const p = obj(v, ['sequence','sha256','size','format','packVersion','thin','dependsOn','locations']);
    const n = size(p.size, BigInt(MAX_PACK_BYTES)); total += n;
    if (n < 32n || total > BigInt(MAX_TOTAL_BYTES) || p.sequence !== i || !Number.isSafeInteger(p.sequence) || !match(p.sha256, /^[0-9a-f]{64}$/) || digests.has(p.sha256 as string) || p.format !== 'git-pack' || p.packVersion !== 2 || p.thin !== false || !Array.isArray(p.dependsOn) || !Array.isArray(p.locations) || !p.locations.length || p.locations.length > 4) invalid();
    // Schema 1 keeps self-contained packs only; schema 2 (incremental chains)
    // allows an explicit backward-only dependency closure per entry.
    if (m.schemaVersion === 1 && (p.dependsOn as unknown[]).length !== 0) invalid();
    if (m.schemaVersion === 2) { const deps = new Set<string>(); for (const d of p.dependsOn as unknown[]) { if (typeof d !== 'string' || !match(d, /^[0-9a-f]{64}$/) || !digests.has(d) || deps.has(d)) invalid(); deps.add(d as string); } }
    digests.add(p.sha256 as string); const locations = new Set<string>();
    Array.from(p.locations as unknown[]).forEach(v => {
      const l = obj(v, ['provider','url','reader']);
      if (!['aws-s3','cloudflare-r2','ipfs'].includes(l.provider as string) || typeof l.url !== 'string' || typeof l.reader !== 'string' || (l.url === '') === (l.reader === '')) invalid();
      if (l.reader !== '') { if (l.provider === 'ipfs' || !match(l.reader, /^[a-zA-Z0-9_-]{1,64}$/)) invalid(); }
      else if (l.provider === 'ipfs') { if (!match(l.url, /^ipfs:\/\/(b[a-z2-7]{20,120}|Qm[1-9A-HJ-NP-Za-km-z]{44})$/)) invalid(); }
      else if (!validPublicURL(l.url as string)) invalid();
      const key = canonicalize(l)!; if (locations.has(key)) invalid(); locations.add(key);
    });
  });
  return input as PackManifest;
}
export function encodeManifest(m: PackManifest): Uint8Array {
  validateManifest(m); const bytes = encoder.encode(canonicalize(m)!); if (bytes.length > MAX_MANIFEST_BYTES) invalid(); return bytes;
}
export async function digest(bytes: Uint8Array): Promise<string> {
  const hash = await crypto.subtle.digest('SHA-256', new Uint8Array(bytes));
  return [...new Uint8Array(hash)].map(b => b.toString(16).padStart(2, '0')).join('');
}
export async function parseManifest(bytes: Uint8Array, expected: ManifestContext, c: ManifestCommitment): Promise<PackManifest> {
  obj(c, ['sha256','size','bootstrapLocator']);
  if (size(c.size, BigInt(MAX_MANIFEST_BYTES)) !== BigInt(bytes.length) || !match(c.sha256, /^[0-9a-f]{64}$/) || !validPublicURL(c.bootstrapLocator) || await digest(bytes) !== c.sha256) invalid();
  const m = validateManifest(strictJSON(bytes)); const encoded = encodeManifest(m);
  if (encoded.length !== bytes.length || !encoded.every((b,i) => bytes[i] === b)) invalid();
  const ctx = { chainId:m.chainId,suiteDirectory:m.suiteDirectory,repoId:m.repoId,refName:m.refName,commit:m.commit };
  obj(expected, ['chainId','suiteDirectory','repoId','refName','commit']); context(expected);
  if (canonicalize(ctx) !== canonicalize(expected)) invalid(); return m;
}

export type ManifestBaseContext = { chainId: string; suiteDirectory: string; repoId: string; refName: string };

/**
 * Cold-reader entry: identical to parseManifest except the commit OID is not
 * known in advance; every other context field and the whole commitment check
 * stay enforced, and the commit is learned from the verified body.
 */
export async function parseRefManifest(bytes: Uint8Array, base: ManifestBaseContext, c: ManifestCommitment): Promise<PackManifest> {
  obj(c, ['sha256','size','bootstrapLocator']);
  if (size(c.size, BigInt(MAX_MANIFEST_BYTES)) !== BigInt(bytes.length) || !match(c.sha256, /^[0-9a-f]{64}$/) || !validPublicURL(c.bootstrapLocator) || await digest(bytes) !== c.sha256) invalid();
  const m = validateManifest(strictJSON(bytes)); const encoded = encodeManifest(m);
  if (encoded.length !== bytes.length || !encoded.every((b,i) => bytes[i] === b)) invalid();
  if (m.chainId !== base.chainId || m.suiteDirectory !== base.suiteDirectory || m.repoId !== base.repoId || m.refName !== base.refName) invalid();
  return m;
}