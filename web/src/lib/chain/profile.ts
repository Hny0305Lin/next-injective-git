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
    evmExplorer: "https://testnet.blockscout.injective.network",
  },
};

export const DEFAULT_PROFILE: NetworkProfileId = "injective-testnet";
const LS_KEY = "igit-web-config";
export const CONFIG_CHANGED_EVENT = "igit-config-changed";

export function isSuiteDirectoryConfigured(value: string): boolean {
  return /^0x[0-9a-fA-F]{40}$/.test(value.trim());
}

// Loopback, `.localhost`/`.local`, and RFC1918 LAN hosts are the only origins
// treated as a local deployment. A public origin (igit.xyz, a preview URL, or
// anything else) must not let a visitor point the app at an arbitrary Suite,
// so the Settings form there is read-only and copy-only.
export function isLocalDeploymentHost(hostname: string): boolean {
  const host = hostname.trim().toLowerCase().replace(/^\[|\]$/g, "");
  if (!host) return true; // file:// and similar origins expose no hostname.
  if (host === "localhost" || host.endsWith(".localhost")) return true;
  if (host === "::1" || host === "0.0.0.0") return true;
  if (host.endsWith(".local")) return true;
  const ipv4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host);
  if (!ipv4) return false;
  const [a, b] = ipv4.slice(1).map(Number);
  if ([a, ...ipv4.slice(2).map(Number)].some((part) => part > 255)) return false;
  if (a === 127) return true;
  if (a === 10) return true;
  if (a === 192 && b === 168) return true;
  if (a === 172 && b >= 16 && b <= 31) return true;
  return false;
}

export function isLocalDeployment(): boolean {
  if (typeof window === "undefined" || !window.location) return false;
  return isLocalDeploymentHost(window.location.hostname);
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
      } else if (isLocalDeployment() && !cfg.suiteDirectory) {
        cfg.suiteDirectory = "0xf8844F90887731FFd607E1f59e39a3918F6eAb35";
      }
      return cfg;
    }
  } catch {
    // Malformed and legacy settings are replaced by the fail-closed profile.
  }
  const cfg = configForProfile();
  if (isLocalDeployment() && !cfg.suiteDirectory) {
    cfg.suiteDirectory = "0xf8844F90887731FFd607E1f59e39a3918F6eAb35";
  }
  return cfg;
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
