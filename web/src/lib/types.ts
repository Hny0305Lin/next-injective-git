import type { LucideIcon } from "lucide-react";
import type { ContractTx } from "./chain";

export type SourceState = "loading" | "healthy" | "not-configured" | "unavailable" | "degraded";

export interface SourceSnapshot {
  state: SourceState;
  detail: string;
  checkedAt: number | null;
  error?: string;
}

export interface MonitorSnapshot {
  evm: SourceSnapshot;
  latestBlock: bigint | null;
  activity: ContractTx[];
  activityError: string;
  v1: SourceSnapshot & { snapshotHeight: number | null };
  ipfs: Record<IpfsGatewayId, IpfsGatewaySnapshot>;
}

export type IpfsGatewayId = "hk" | "us";

export interface IpfsGatewayDefinition {
  id: IpfsGatewayId;
  title: string;
  description: string;
  region: string;
  endpoint: string;
  href: string;
  icon: LucideIcon;
}

export interface IpfsGatewaySnapshot extends SourceSnapshot {
  latencyMs: number | null;
  statusCode: number | null;
  probeSource: "browser" | null;
  sampleCount: number;
  responseSamples: number;
  reachableSamples: number;
}

export type PublicProviderStatus = "current" | "legacy";

export interface PublicStorageProvider {
  id: string;
  title: string;
  description: string;
  kind: "Storage provider";
  region: string;
  role: string;
  endpoint: string;
  href: string;
  status: PublicProviderStatus;
  icon: LucideIcon;
}
