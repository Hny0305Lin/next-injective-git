import { Archive, Boxes, Database } from "lucide-react";
import { usesByosStorage } from "../lib/suite-compat";

export type RepositoryContractKind = "evm-v2" | "cosmwasm-v1";

export function ContractTypeBadge({
  kind,
  suiteVersion,
}: {
  kind: RepositoryContractKind;
  /** Verified suite protocol version; v4+ repositories resolve their packs from BYOS buckets. */
  suiteVersion?: bigint;
}) {
  const legacy = kind === "cosmwasm-v1";
  const byos = !legacy && suiteVersion !== undefined && usesByosStorage(suiteVersion);
  if (byos) {
    return (
      <span
        className="contract-type-badge contract-evm-v4"
        title="Injective EVM V4 Suite contract — packfiles resolve from BYOS storage buckets through a verified manifest"
      >
        <Database size={11} aria-hidden="true" />
        EVM V4
        <em className="byos-tag">BYOS</em>
      </span>
    );
  }
  const Icon = legacy ? Archive : Boxes;
  const label = legacy ? "CosmWasm V1" : "EVM V2";
  return (
    <span
      className={`contract-type-badge ${legacy ? "contract-cosmwasm" : "contract-evm"}`}
      title={legacy ? "Read-only CosmWasm V1 archive contract" : "Injective EVM V2 Suite contract"}
    >
      <Icon size={11} aria-hidden="true" />
      {label}
    </span>
  );
}
