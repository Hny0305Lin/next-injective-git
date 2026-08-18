import { Archive, Boxes } from "lucide-react";

export type RepositoryContractKind = "evm-v2" | "cosmwasm-v1";

export function ContractTypeBadge({ kind }: { kind: RepositoryContractKind }) {
  const legacy = kind === "cosmwasm-v1";
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
