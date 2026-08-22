import { bech32 } from "@scure/base";
import { getAddress, hexToBytes, type Address } from "viem";

export function toEvmAddress(value: string): Address {
  const input = value.trim();
  if (/^0x[0-9a-fA-F]{40}$/.test(input)) return getAddress(input);
  try {
    const decoded = bech32.decode(input as `${string}1${string}`, 90);
    const bytes = Uint8Array.from(bech32.fromWords(decoded.words));
    if (decoded.prefix !== "inj" || bytes.length !== 20) throw new Error("wrong prefix or length");
    return getAddress(`0x${Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("")}`);
  } catch {
    throw new Error(`invalid Injective address: ${value}`);
  }
}

export function toInjectiveAddress(value: string): string {
  const address = toEvmAddress(value);
  return bech32.encode("inj", bech32.toWords(hexToBytes(address)), 90);
}

export function sameAddress(left: string, right: string): boolean {
  try {
    return toEvmAddress(left).toLowerCase() === toEvmAddress(right).toLowerCase();
  } catch {
    return false;
  }
}
