// AMap (Gaode) JS API 2.0 loader for the MapMonitor page.
//
// The AMap SDK has no npm package: it is injected once as a script tag with
// the enterprise key. Keys come from VITE_AMAP_KEY and the optional
// VITE_AMAP_SECURITY_CODE (企业安全密钥); create/manage them at
// https://console.amap.com — Web端(JS API). When the key is missing,
// MapMonitor falls back to the Leaflet/OpenStreetMap dev engine, which must
// never ship to mainland-facing production.
//
// AMap basemaps use the GCJ-02 datum. Infrastructure coordinates are WGS-84,
// so points inside mainland China are converted before display; outside the
// conversion region both datums coincide for our zoom levels.

declare global {
  interface Window {
    AMap?: unknown;
    _AMapSecurityConfig?: { securityJsCode: string };
  }
}

export const AMAP_KEY: string =
  (import.meta.env.VITE_AMAP_KEY ?? "").trim();

export const AMAP_SECURITY_CODE: string =
  (import.meta.env.VITE_AMAP_SECURITY_CODE ?? "").trim();

// --- WGS-84 -> GCJ-02 (mainland China only; pass-through elsewhere) --------

const CHINA_BOUNDS = { minLat: 3.0, maxLat: 54.0, minLng: 73.5, maxLng: 135.0 };

function inChina(lat: number, lng: number): boolean {
  return lat >= CHINA_BOUNDS.minLat && lat <= CHINA_BOUNDS.maxLat
    && lng >= CHINA_BOUNDS.minLng && lng <= CHINA_BOUNDS.maxLng;
}

function transformLat(x: number, y: number): number {
  let ret = -100 + 2 * x + 3 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * Math.sqrt(Math.abs(x));
  ret += ((20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2) / 3;
  ret += ((20 * Math.sin(y * Math.PI) + 40 * Math.sin((y / 3) * Math.PI)) * 2) / 3;
  ret += ((160 * Math.sin((y / 12) * Math.PI) + 320 * Math.sin((y * Math.PI) / 30)) * 2) / 3;
  return ret;
}

function transformLng(x: number, y: number): number {
  let ret = 300 + x + 2 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * Math.sqrt(Math.abs(x));
  ret += ((20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2) / 3;
  ret += ((20 * Math.sin(x * Math.PI) + 40 * Math.sin((x / 3) * Math.PI)) * 2) / 3;
  ret += ((150 * Math.sin((x / 12) * Math.PI) + 300 * Math.sin((x / 30) * Math.PI)) * 2) / 3;
  return ret;
}

/** WGS-84 -> GCJ-02 inside mainland China; identity coordinates elsewhere. */
export function wgsToGcj(lat: number, lng: number): [number, number] {
  if (!inChina(lat, lng)) return [lat, lng];
  const a = 6378245;
  const ee = 0.00669342162296594323;
  let dLat = transformLat(lng - 105, lat - 35);
  let dLng = transformLng(lng - 105, lat - 35);
  const radLat = (lat / 180) * Math.PI;
  let magic = Math.sin(radLat);
  magic = 1 - ee * magic * magic;
  const sqrtMagic = Math.sqrt(magic);
  dLat = (dLat * 180) / (((a * (1 - ee)) / (magic * sqrtMagic)) * Math.PI);
  dLng = (dLng * 180) / ((a / sqrtMagic) * Math.cos(radLat) * Math.PI);
  return [lat + dLat, lng + dLng];
}

// --- AMap script loader ------------------------------------------------------

/* eslint-disable @typescript-eslint/no-explicit-any */
type AMapNamespace = any;

let loader: Promise<AMapNamespace> | null = null;

export function loadAMap(timeoutMs = 10_000): Promise<AMapNamespace> {
  if (loader != null) return loader;
  loader = new Promise<AMapNamespace>((resolve, reject) => {
    if (window.AMap != null) {
      resolve(window.AMap as AMapNamespace);
      return;
    }
    if (!AMAP_KEY) {
      loader = null;
      reject(new Error("VITE_AMAP_KEY is not configured"));
      return;
    }
    if (AMAP_SECURITY_CODE) {
      window._AMapSecurityConfig = { securityJsCode: AMAP_SECURITY_CODE };
    }
    const timer = window.setTimeout(() => {
      loader = null;
      reject(new Error("AMap JS API failed to load (timeout)"));
    }, timeoutMs);
    const cleanup = () => window.clearTimeout(timer);
    const script = document.createElement("script");
    script.src = `https://webapi.amap.com/maps?v=2.0&key=${encodeURIComponent(AMAP_KEY)}`;
    script.async = true;
    script.onload = () => {
      cleanup();
      if (window.AMap == null) {
        loader = null;
        reject(new Error("AMap JS API loaded without the AMap global (check the key)"));
        return;
      }
      resolve(window.AMap as AMapNamespace);
    };
    script.onerror = () => {
      cleanup();
      loader = null;
      reject(new Error("AMap JS API script failed to load"));
    };
    document.head.appendChild(script);
  });
  return loader;
}
