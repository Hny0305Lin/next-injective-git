import type { ContractTx } from "../../lib/chain";
import { timeAgo } from "../../lib/chain";

export function shortAddress(value: string, size = 8) {
  return value.length > size * 2 ? `${value.slice(0, size)}...${value.slice(-4)}` : value;
}

export function formatBlock(value: bigint | number | null) {
  return value == null ? "—" : typeof value === "bigint" ? value.toLocaleString("en-US") : value.toLocaleString("en-US");
}

export function formatCheckedAt(value: number | null) {
  return value == null ? "Not checked" : timeAgo(value / 1000);
}

export function formatAction(action: string) {
  return action.replaceAll("_", " ");
}

export function activityContext(row: ContractTx) {
  const attributes = row.attributes;
  const repository = attributes.repo ?? attributes.repository ?? attributes.name;
  if (repository) return repository;
  const owner = attributes.owner ?? attributes.recipient;
  if (owner) return shortAddress(owner, 8);
  return "Suite module event";
}

export function activityBuckets(rows: ContractTx[]) {
  const buckets = Array.from({ length: 12 }, () => 0);
  if (rows.length === 0) return buckets;
  const heights = rows.map((row) => Number(row.height)).filter(Number.isFinite);
  const minimum = Math.min(...heights);
  const maximum = Math.max(...heights);
  const span = Math.max(maximum - minimum, 1);
  rows.forEach((row) => {
    const height = Number(row.height);
    if (!Number.isFinite(height)) return;
    const index = Math.min(11, Math.floor(((height - minimum) / span) * 12));
    buckets[index] += 1;
  });
  return buckets;
}

