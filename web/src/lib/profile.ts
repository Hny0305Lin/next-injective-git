export type NetworkProfileId = "injective-testnet";

export interface NetworkProfile {
  id: NetworkProfileId;
  label: string;
  evmChainId: number;
  evmRpc: string;
  suiteDirectory: string;
  ipfsGateway: string;
  evmExplorer: string;
}

export interface AppConfig {
  profile: NetworkProfileId;
  evmChainId: number;
  evmRpc: string;
  suiteDirectory: string;
  ipfsGateway: string;
}

// A built-in address is added only after deployment evidence has passed the
// release gate. Keeping this empty makes pre-cutover builds fail closed.
export const NETWORK_PROFILES: Record<NetworkProfileId, NetworkProfile> = {
  "injective-testnet": {
    id: "injective-testnet",
    label: "Injective Testnet",
    evmChainId: 1439,
    evmRpc: "https://k8s.testnet.json-rpc.injective.network/",
    suiteDirectory: "",
    ipfsGateway: "https://igit-hk.haohanyh.ovh",
    evmExplorer: "https://testnet-injective.cloud.blockscout.com",
  },
};

export const DEFAULT_PROFILE: NetworkProfileId = "injective-testnet";
const LS_KEY = "igit-web-config";
export const CONFIG_CHANGED_EVENT = "igit-config-changed";

export function isSuiteDirectoryConfigured(value: string): boolean {
  return /^0x[0-9a-fA-F]{40}$/.test(value.trim());
}

export function configForProfile(profile: NetworkProfileId = DEFAULT_PROFILE): AppConfig {
  const selected = NETWORK_PROFILES[profile] ?? NETWORK_PROFILES[DEFAULT_PROFILE];
  return {
    profile: selected.id,
    evmChainId: selected.evmChainId,
    evmRpc: selected.evmRpc,
    suiteDirectory: selected.suiteDirectory,
    ipfsGateway: selected.ipfsGateway,
  };
}

export function loadConfig(): AppConfig {
  try {
    const saved = JSON.parse(localStorage.getItem(LS_KEY) ?? "null") as {
      profile?: unknown;
      suiteDirectory?: unknown;
    } | null;
    if (saved?.profile === "injective-testnet") {
      const cfg = configForProfile(saved.profile);
      if (typeof saved.suiteDirectory === "string" && isSuiteDirectoryConfigured(saved.suiteDirectory)) {
        cfg.suiteDirectory = saved.suiteDirectory.trim();
      }
      return cfg;
    }
  } catch {
    // Malformed and legacy settings are replaced by the fail-closed profile.
  }
  return configForProfile();
}

export function saveConfig(cfg: AppConfig): void {
  const suiteDirectory = cfg.suiteDirectory.trim();
  if (suiteDirectory && !isSuiteDirectoryConfigured(suiteDirectory)) {
    throw new Error("SuiteDirectory must be a 0x-prefixed EVM address");
  }
  localStorage.setItem(LS_KEY, JSON.stringify({
    profile: cfg.profile,
    ...(suiteDirectory ? { suiteDirectory } : {}),
  }));
  if (typeof window !== "undefined") window.dispatchEvent(new Event(CONFIG_CHANGED_EVENT));
}

export function networkProfile(cfg: AppConfig): NetworkProfile {
  return NETWORK_PROFILES[cfg.profile] ?? NETWORK_PROFILES[DEFAULT_PROFILE];
}
