export * from "./profile";
export * from "./errors";
export * from "./registry";
export * from "./modules";
export * from "./activity";
export { ensureWalletChain, walletChainId, verifySuite, clearSuiteCache, type SuiteBinding } from "./transport";
export type { Eip1193 } from "./transport";

export function formatInj(amount: string, denom: string): string {
  if (denom !== "inj") return `${amount} ${denom}`;
  const value = amount.padStart(19, "0");
  const whole = value.slice(0, -18);
  const fraction = value.slice(-18).replace(/0+$/, "").slice(0, 6);
  return fraction ? `${whole}.${fraction} INJ` : `${whole} INJ`;
}

export function formatFunds(funds: string): string {
  const match = /^(\d+)inj$/.exec(funds);
  return match ? formatInj(match[1], "inj") : funds;
}

export function timeAgo(seconds: number): string {
  const difference = Date.now() / 1000 - seconds;
  if (difference < 60) return "just now";
  if (difference < 3600) return `${Math.floor(difference / 60)} min ago`;
  if (difference < 86400) return `${Math.floor(difference / 3600)} h ago`;
  if (difference < 86400 * 30) return `${Math.floor(difference / 86400)} d ago`;
  return new Date(seconds * 1000).toISOString().slice(0, 10);
}
