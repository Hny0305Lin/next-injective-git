import type { Hex } from "viem";
import type { RepoInfo } from "./registry";

export interface DiscoverRepository extends RepoInfo {
  source: "evm-suite-events";
  repo_id: Hex;
  suite_directory: string;
}

export type DiscoverStatus = "all" | RepoInfo["moderation_status"];
export type DiscoverSuiteVersion = "all" | 3 | 4;

export interface DiscoverFilters {
  query?: string;
  owner?: string;
  status?: DiscoverStatus;
  suiteVersion?: DiscoverSuiteVersion;
}

function normalized(value: string | undefined): string {
  return (value ?? "").trim().toLowerCase();
}

export function filterDiscoverRepositories(rows: readonly DiscoverRepository[], filters: DiscoverFilters = {}): DiscoverRepository[] {
  const query = normalized(filters.query);
  const owner = normalized(filters.owner);
  const status = filters.status ?? "all";
  const suiteVersion = filters.suiteVersion ?? "all";
  return rows
    .filter((row) => row.source === "evm-suite-events")
    .filter((row) => !owner || row.owner.toLowerCase().includes(owner))
    .filter((row) => status === "all" || row.moderation_status === status)
    .filter((row) => suiteVersion === "all" || Number(row.suite_version) === suiteVersion)
    .filter((row) => !query || [row.owner, row.name, row.description, row.default_branch].some((value) => value.toLowerCase().includes(query)))
    .sort((left, right) => right.updated_at - left.updated_at || left.name.localeCompare(right.name) || left.owner.localeCompare(right.owner) || left.repo_id.localeCompare(right.repo_id));
}

export function pageDiscoverRepositories(rows: readonly DiscoverRepository[], page: number, pageSize: number): DiscoverRepository[] {
  const safePage = Number.isFinite(page) && page > 0 ? Math.floor(page) : 1;
  const safePageSize = Number.isFinite(pageSize) && pageSize > 0 ? Math.floor(pageSize) : 1;
  return rows.slice((safePage - 1) * safePageSize, safePage * safePageSize);
}
