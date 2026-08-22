import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Truncate a string (typically an address or hash) to show first and last characters
 * @param value - String to truncate
 * @param prefixLength - Number of characters to show at the start (default: 8)
 * @param suffixLength - Number of characters to show at the end (default: 4)
 * @returns Truncated string with ellipsis, or original if already short
 */
export function truncateAddress(value: string, prefixLength = 8, suffixLength = 4): string {
  if (!value) return "";
  const totalShown = prefixLength + suffixLength;
  if (value.length <= totalShown) return value;
  return `${value.slice(0, prefixLength)}…${value.slice(-suffixLength)}`;
}

