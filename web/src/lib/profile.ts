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
  /** Verified suite protocol version (3n IPFS, 4n BYOS). Set after verifySuite. */
  evmSuiteVersion?: bigint;
}

// A built-in address is added only after deployment evidence has passed the
// release gate. Both verified on-chain SuiteDirectory deployments (v4 and v3)
// are built in since the successor cutover; an empty list is only for
// pre-cutover builds, which must fail closed.
export const NETWORK_PROFILES: Record<NetworkProfileId, NetworkProfile> = {
  "injective-testnet": {
    id: "injective-testnet",
    label: "Injective Testnet",
    evmChainId: 1439,
    evmRpc: "https://k8s.testnet.json-rpc.injective.network/",
    suiteDirectory: "0xf987396475d0a4c96b722e993a95d8720a6292ad,0xf8844F90887731FFd607E1f59e39a3918F6eAb35",
    ipfsGateway: "https://igit-hk.haohanyh.ovh",
    evmExplorer: "https://testnet.blockscout.injective.network",
  },
};

export const DEFAULT_PROFILE: NetworkProfileId = "injective-testnet";
const LS_KEY = "igit-web-config";
export const CONFIG_CHANGED_EVENT = "igit-config-changed";

export function isSuiteDirectoryConfigured(value: string): boolean {
  // Accepts a single address or a comma-separated list of addresses.
  return value.split(",").every((s) => {
    const t = s.trim();
    return t === "" || /^0x[0-9a-fA-F]{40}$/.test(t);
  }) && value.trim() !== "";
}

/** Parse a comma-separated SuiteDirectory string into validated addresses. */
export function parseSuiteDirectories(value: string): string[] {
  return value
    .split(/[;,]/)
    .map((s) => s.trim())
    .filter((s) => /^0x[0-9a-fA-F]{40}$/.test(s))
    .map((s) => s.toLowerCase());
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
        // Auto-upgrade stale overrides: if the saved address is one of the
        // built-in addresses (user saved it from a previous release), replace
        // it with the current built-in list. Custom addresses pass through as-is.
        const PREVIOUS_DEFAULTS = new Set(["0xf8844F90887731FFd607E1f59e39a3918F6eAb35"]);
        const savedAddr = saved.suiteDirectory.trim();
        if (PREVIOUS_DEFAULTS.has(savedAddr)) {
          cfg.suiteDirectory = cfg.suiteDirectory || savedAddr; // use current built-in
        } else {
          cfg.suiteDirectory = savedAddr;
        }
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
    const addresses = suiteDirectory.split(",").map((s: string) => s.trim()).filter(Boolean);
    if (addresses.length > 0 && !addresses.every((a: string) => isSuiteDirectoryConfigured(a))) {
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