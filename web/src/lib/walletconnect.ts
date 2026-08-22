import type EthereumProvider from "@walletconnect/ethereum-provider";
import type { Eip1193 } from "./transport";
import { loadConfig } from "./profile";

export const WALLETCONNECT_ID = "walletconnect";
export const WALLETCONNECT_LABEL = "WalletConnect";
const PROJECT_ID_PLACEHOLDER = "replace-with-your-reown-project-id";

let providerPromise: Promise<EthereumProvider> | null = null;
let pairingPromise: Promise<EthereumProvider> | null = null;
/** Keeps a canceled relay approval from racing a later pairing on the same provider. */
let pairingBusy: Promise<void> | null = null;
let disconnectPromise: Promise<void> | null = null;
let activeProvider: EthereumProvider | null = null;
let activeCancel: (() => void) | null = null;

export class WalletConnectPairingCancelledError extends Error {
  constructor() {
    super("WalletConnect pairing canceled");
    this.name = "WalletConnectPairingCancelledError";
  }
}

function projectId(): string {
  const value = import.meta.env?.VITE_WALLETCONNECT_PROJECT_ID?.trim();
  if (!value || value.toLowerCase() === PROJECT_ID_PLACEHOLDER) {
    throw new Error("WalletConnect is not configured for this site");
  }
  return value;
}

export function walletConnectConfigured(): boolean {
  const value = import.meta.env?.VITE_WALLETCONNECT_PROJECT_ID?.trim();
  return Boolean(value && value.toLowerCase() !== PROJECT_ID_PLACEHOLDER);
}

export async function getWalletConnectProvider(): Promise<EthereumProvider> {
  if (disconnectPromise) await disconnectPromise;
  if (!providerPromise) {
    const cfg = loadConfig();
    providerPromise = import("@walletconnect/ethereum-provider").then(({ default: Provider }) => Provider.init({
        projectId: projectId(),
        // The application cannot create a writable session on any other chain.
        // Requiring 1439 also lets an incompatible wallet reject the proposal
        // before the UI mistakes an empty optional namespace for a connection.
        chains: [cfg.evmChainId],
        rpcMap: { [cfg.evmChainId]: cfg.evmRpc },
        showQrModal: false,
        metadata: {
          name: "igit",
          description: "Git hosting on Injective",
          url: typeof window === "undefined" ? "https://igit.app" : window.location.origin,
          icons: [],
        },
      })).catch((error) => {
      providerPromise = null;
      throw error;
    });
  }
  return providerPromise;
}

/**
 * Starts one pairing attempt and forwards its short-lived URI to the custom UI.
 * A second click shares the first attempt instead of creating a competing URI.
 */
export function connectWalletConnect(onUri: (uri: string) => void): Promise<EthereumProvider> {
  if (pairingPromise) return pairingPromise;
  if (pairingBusy) {
    return Promise.reject(new Error("WalletConnect is still closing the previous pairing; try again shortly"));
  }
  let cancelled = false;
  let rejectCancelled: ((reason: unknown) => void) | null = null;
  const cancelledPromise = new Promise<never>((_, reject) => {
    rejectCancelled = reject;
  });
  const cancel = () => {
    if (cancelled) return;
    cancelled = true;
    rejectCancelled?.(new WalletConnectPairingCancelledError());
    // Current UniversalProvider versions expose this cleanup even though
    // their deprecated abortPairingAttempt hook is a no-op.
    void activeProvider?.signer.cleanupPendingPairings({ deletePairings: true }).catch(() => undefined);
  };
  activeCancel = cancel;
  const operation = (async () => {
    // Race provider initialization as well as the relay approval. Without
    // this, closing the modal while the lazy WalletConnect chunk is loading
    // could still start a pairing after the UI has disappeared.
    const providerLoad = getWalletConnectProvider();
    void providerLoad.catch(() => undefined);
    let provider: EthereumProvider | undefined;
    let existingSession = false;
    let connection: Promise<void> | null = null;
    const displayUri = (uri: string) => onUri(uri);
    let listening = false;
    try {
      provider = await Promise.race([providerLoad, cancelledPromise]);
      if (cancelled) throw new WalletConnectPairingCancelledError();
      activeProvider = provider;
      provider.on("display_uri", displayUri);
      listening = true;
      existingSession = Boolean(provider.session);
      if (!existingSession) {
        connection = provider.connect();
        // A canceled race must not turn the SDK's eventual rejection into an
        // unhandled promise after the custom UI has already closed.
        void connection.catch(() => undefined);
        await Promise.race([connection, cancelledPromise]);
      }
      if (cancelled) throw new WalletConnectPairingCancelledError();
      if (!provider.session) throw new Error("WalletConnect did not create a session");
      return provider;
    } finally {
      // Keep the module busy until a canceled provider.connect() really
      // settles. Otherwise a late approval could disconnect a newer session.
      if (connection) {
        await connection.then(async () => {
          if (cancelled && !existingSession && provider?.session) {
            try {
              await provider.disconnect();
            } catch {
              // The relay may already have discarded the proposal.
            }
          }
        }, () => undefined);
      }
      if (listening) provider?.off("display_uri", displayUri);
      onUri("");
      if (activeCancel === cancel) activeCancel = null;
      if (activeProvider === provider) activeProvider = null;
    }
  })();
  const busy = operation.then(() => undefined, () => undefined);
  pairingBusy = busy;
  void busy.finally(() => {
    if (pairingBusy === busy) pairingBusy = null;
  });
  const exposed = Promise.race([operation, cancelledPromise]);
  pairingPromise = exposed.finally(() => {
    pairingPromise = null;
  });
  return pairingPromise;
}

export async function restoreWalletConnect(): Promise<EthereumProvider | null> {
  if (!walletConnectConfigured()) return null;
  if (pairingBusy) await pairingBusy;
  const provider = await getWalletConnectProvider();
  return provider.session && provider.accounts.length > 0 ? provider : null;
}

export function cancelWalletConnectPairing(): void {
  activeCancel?.();
}

export async function disconnectWalletConnect(provider: Eip1193): Promise<void> {
  const candidate = provider as EthereumProvider;
  if (pairingBusy) await pairingBusy;
  if (disconnectPromise) await disconnectPromise;
  const operation = (async () => {
    if (candidate.session) await candidate.disconnect();
  })();
  const settled = operation.catch(() => undefined);
  disconnectPromise = settled;
  try {
    await settled;
  } finally {
    if (disconnectPromise === settled) disconnectPromise = null;
  }
}
