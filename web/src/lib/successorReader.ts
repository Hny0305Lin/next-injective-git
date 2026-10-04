// Verified BYOS reader for the storage-neutral successor suite. Downloads are
// length-bounded and checked against the on-chain commitment before any byte
// reaches isomorphic-git. The Web never holds cloud credentials: if a bucket
// is not anonymously readable (or lacks CORS), callers get an actionable hint
// instead of a prompt for secrets.
import {
  MAX_MANIFEST_BYTES,
  digest,
  parseRefManifest,
  type ManifestBaseContext,
  type ManifestCommitment,
  type PackEntry,
  type PackManifest,
} from "./packmanifest";

/** Browser memory budgets: stricter than the CLI receiver limits on purpose. */
export const MAX_WEB_PACK_BYTES = 32 * 1024 * 1024;
export const MAX_WEB_TOTAL_BYTES = 256 * 1024 * 1024;

export class ReaderLimitError extends Error {}
export class ReaderIntegrityError extends Error {}
export class ReaderAccessError extends Error {}


async function boundedFetch(url: string, limit: number, what: string): Promise<Uint8Array> {
  let response: Response;
  try {
    response = await fetch(url, { credentials: "omit", redirect: "error" });
  } catch (error) {
    throw new ReaderAccessError(
      `${what} fetch failed for ${url}: the bucket may not allow anonymous public reads or lacks CORS; ` +
        `configure public read (publicReadBase + CORS) or use the independent CLI reader instead. ` +
        `The web never accepts cloud secrets. (${error instanceof Error ? error.message : String(error)})`,
    );
  }
  if (!response.ok) {
    throw new ReaderAccessError(`${what} fetch returned HTTP ${response.status} for ${url}; public read is not configured or the object is missing`);
  }
  const declared = Number(response.headers.get("content-length") ?? "");
  if (Number.isFinite(declared) && declared > limit) {
    throw new ReaderLimitError(`${what} declares ${declared} bytes, over the ${limit}-byte web budget`);
  }
  const buffer = await response.arrayBuffer();
  if (buffer.byteLength > limit) {
    throw new ReaderLimitError(`${what} is ${buffer.byteLength} bytes, over the ${limit}-byte web budget`);
  }
  return new Uint8Array(buffer);
}

/** Fetch and verify a manifest against the on-chain commitment. */
export async function fetchVerifiedManifest(
  base: ManifestBaseContext,
  commitment: ManifestCommitment,
): Promise<PackManifest> {
  const bytes = await boundedFetch(commitment.bootstrapLocator, MAX_MANIFEST_BYTES, "manifest");
  if (await digest(bytes) !== commitment.sha256) {
    throw new ReaderIntegrityError("manifest bytes do not match the on-chain commitment digest");
  }
  return parseRefManifest(bytes, base, commitment);
}

export interface PackBudget {
  total: number;
}

/** Download one pack entry, verify its digest/size and enforce memory budgets. */
export async function fetchVerifiedPack(entry: PackEntry, budget: PackBudget): Promise<Uint8Array> {
  const declared = Number(entry.size);
  if (!Number.isSafeInteger(declared) || declared <= 0) {
    throw new ReaderIntegrityError("pack entry has an invalid size");
  }
  if (declared > MAX_WEB_PACK_BYTES) {
    throw new ReaderLimitError(`pack ${entry.sha256.slice(0, 12)} exceeds the per-pack web budget`);
  }
  if (budget.total + declared > MAX_WEB_TOTAL_BYTES) {
    throw new ReaderLimitError("repository packs exceed the total web download budget");
  }
  let lastError: unknown = null;
  for (const location of entry.locations) {
    if (location.provider === "ipfs" || !location.url) continue;
    try {
      const bytes = await boundedFetch(location.url, MAX_WEB_PACK_BYTES, "pack");
      if (bytes.length !== declared || (await digest(bytes)) !== entry.sha256) {
        throw new ReaderIntegrityError(`pack bytes do not match the manifest commitment for ${entry.sha256.slice(0, 12)}`);
      }
      budget.total += bytes.length;
      return bytes;
    } catch (error) {
      lastError = error;
      if (error instanceof ReaderLimitError) throw error;
    }
  }
  if (lastError instanceof Error) throw lastError;
  throw new ReaderAccessError("pack has no fetchable public location");
}