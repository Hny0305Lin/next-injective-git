import { useMemo } from "react";

/**
 * GitHub-style identicon: a deterministic 5x5 mirrored block pattern with a
 * hash-derived color, computed client-side from a seed string (the on-chain
 * owner address). No avatar upload, no external identicon service.
 *
 * Same idea as GitHub avatars: hash the user identifier, derive a foreground
 * color from the first hash bytes, then toggle the left 3 columns of a 5x5
 * grid from the remaining bytes and mirror them for vertical symmetry.
 */

function sha1Bytes(input: string): number[] {
  const data = new TextEncoder().encode(input);
  const padded = new Uint8Array((((data.length + 8) >> 6) + 1) << 6);
  padded.set(data);
  padded[data.length] = 0x80;
  const view = new DataView(padded.buffer);
  const bitLength = data.length * 8;
  view.setUint32(padded.length - 8, Math.floor(bitLength / 2 ** 32));
  view.setUint32(padded.length - 4, bitLength >>> 0);

  let h0 = 0x67452301;
  let h1 = 0xefcdab89;
  let h2 = 0x98badcfe;
  let h3 = 0x10325476;
  let h4 = 0xc3d2e1f0;
  const w = new Uint32Array(80);

  for (let offset = 0; offset < padded.length; offset += 64) {
    for (let i = 0; i < 16; i += 1) w[i] = view.getUint32(offset + i * 4);
    for (let i = 16; i < 80; i += 1) {
      const x = w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16];
      w[i] = (x << 1) | (x >>> 31);
    }
    let a = h0;
    let b = h1;
    let c = h2;
    let d = h3;
    let e = h4;
    for (let i = 0; i < 80; i += 1) {
      let f: number;
      let k: number;
      if (i < 20) {
        f = (b & c) | (~b & d);
        k = 0x5a827999;
      } else if (i < 40) {
        f = b ^ c ^ d;
        k = 0x6ed9eba1;
      } else if (i < 60) {
        f = (b & c) | (b & d) | (c & d);
        k = 0x8f1bbcdc;
      } else {
        f = b ^ c ^ d;
        k = 0xca62c1d6;
      }
      const next = (((a << 5) | (a >>> 27)) + f + e + k + w[i]) >>> 0;
      e = d;
      d = c;
      c = (b << 30) | (b >>> 2);
      b = a;
      a = next;
    }
    h0 = (h0 + a) >>> 0;
    h1 = (h1 + b) >>> 0;
    h2 = (h2 + c) >>> 0;
    h3 = (h3 + d) >>> 0;
    h4 = (h4 + e) >>> 0;
  }

  const out: number[] = [];
  for (const word of [h0, h1, h2, h3, h4]) {
    out.push((word >>> 24) & 0xff, (word >>> 16) & 0xff, (word >>> 8) & 0xff, word & 0xff);
  }
  return out;
}

type IdenticonPattern = {
  color: string;
  cells: Array<{ x: number; y: number }>;
};

function buildPattern(seed: string): IdenticonPattern {
  const hash = sha1Bytes(seed);
  // Foreground color from the first bytes, kept in a pleasant mid range
  // (the classic identicon look: muted, never near-black or near-white).
  const hue = (hash[0] / 255) * 360;
  const saturation = 50 + (hash[1] % 31); // 50-80%
  const lightness = 40 + (hash[2] % 21); // 40-60%
  const cells: Array<{ x: number; y: number }> = [];
  for (let y = 0; y < 5; y += 1) {
    for (let x = 0; x < 3; x += 1) {
      // Left half (including the middle column) drives the pattern;
      // columns 3 and 4 mirror columns 1 and 0.
      if (hash[3 + y * 3 + x] % 2 !== 1) continue;
      cells.push({ x, y });
      if (x < 2) cells.push({ x: 4 - x, y });
    }
  }
  return {
    color: `hsl(${hue.toFixed(1)} ${saturation}% ${lightness}%)`,
    cells,
  };
}

export function Identicon({
  seed,
  size = 64,
  title,
}: {
  seed: string;
  size?: number;
  title?: string;
}) {
  const pattern = useMemo(() => buildPattern(seed), [seed]);
  const label = title ?? `identicon for ${seed}`;
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 5 5"
      shapeRendering="crispEdges"
      role="img"
      aria-label={label}
      style={{ display: "block" }}
    >
      <title>{label}</title>
      {/* GitHub identicons keep their light background in every theme. */}
      <rect x="0" y="0" width="5" height="5" fill="#f0f0f0" />
      {pattern.cells.map(({ x, y }) => (
        <rect key={`${x}-${y}`} x={x} y={y} width="1" height="1" fill={pattern.color} />
      ))}
    </svg>
  );
}