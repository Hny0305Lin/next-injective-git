// Basemap registry for the MapMonitor page.
//
// The page renders through Leaflet only. OpenStreetMap is the built-in
// provider and needs no key: it is the default everywhere, including
// production. Other providers stay pluggable without touching the map page:
//
//   VITE_MAP_BASEMAP    provider id from BASEMAPS (default "osm")
//   VITE_MAP_TILE_URL   XYZ template with {z}/{x}/{y}; when set it defines a
//                       "custom" provider and wins over VITE_MAP_BASEMAP
//   VITE_MAP_ATTRIBUTION attribution HTML required by that tile provider
//
// Nothing here loads third-party JavaScript: a provider is a tile URL plus the
// credit line its terms of use require. Unknown ids, or a template without the
// Leaflet placeholders, fall back to OpenStreetMap instead of rendering a
// half-broken map.

export type BasemapDefinition = {
  /** Stable id used by `VITE_MAP_BASEMAP` and in the UI. */
  id: string;
  /** Human-readable provider name (shown in diagnostics/tooltips). */
  label: string;
  /** Leaflet XYZ tile template; must contain {z}, {x} and {y}. */
  urlTemplate: string;
  /** Attribution HTML the provider's terms require; Leaflet renders it. */
  attribution: string;
  /** Deepest zoom the tile provider serves. */
  maxZoom: number;
  /** Optional round-robin subdomains ({s} in the template). */
  subdomains?: string;
};

export const OSM_TILE_URL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png";

// ODbL/attribution requirement of the OSM tile service; the link must stay
// clickable, so this string is rendered as HTML by the attribution control.
export const OSM_ATTRIBUTION =
  '&copy; <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noreferrer">OpenStreetMap</a> contributors';

export const OSM_BASEMAP: BasemapDefinition = {
  id: "osm",
  label: "OpenStreetMap",
  urlTemplate: OSM_TILE_URL,
  attribution: OSM_ATTRIBUTION,
  maxZoom: 19,
};

/**
 * Registry of built-in basemaps keyed by id. Add a provider by appending one
 * entry here (tile URL + required attribution); the map page picks it up via
 * `VITE_MAP_BASEMAP` with no further code change.
 */
export const BASEMAPS: Readonly<Record<string, BasemapDefinition>> = {
  [OSM_BASEMAP.id]: OSM_BASEMAP,
};

export const DEFAULT_BASEMAP_ID = OSM_BASEMAP.id;

/** Ambient values the resolver reads, so it stays pure and testable. */
export type BasemapEnv = {
  basemap?: string;
  tileUrl?: string;
  attribution?: string;
};

function trimmed(value: string | undefined): string {
  return (value ?? "").trim();
}

// Leaflet placeholders are mandatory: a template without them renders one
// tile stretched over the viewport, which looks like a broken map.
const TILE_PLACEHOLDERS = ["{z}", "{x}", "{y}"] as const;

/** Whether `template` is an http(s) XYZ tile URL Leaflet can actually use. */
export function isUsableTileTemplate(template: string): boolean {
  if (!/^https?:\/\//i.test(template)) return false;
  return TILE_PLACEHOLDERS.every((token) => template.includes(token));
}

// Credit line for a provider that ships no attribution text: name the tile
// host instead of rendering tiles with no credit at all.
function hostAttribution(template: string): string {
  try {
    return new URL(template.replace(/\{[^}]+\}/g, "0")).host;
  } catch {
    return "";
  }
}

/**
 * Resolve the basemap from build-time configuration.
 *
 * Precedence: an explicit `VITE_MAP_TILE_URL` (custom provider) over a
 * registry id (default OpenStreetMap) over the OpenStreetMap fallback that
 * also covers unknown ids and unusable templates.
 */
export function resolveBasemap(env: BasemapEnv = {}): BasemapDefinition {
  const tileUrl = trimmed(env.tileUrl);
  if (tileUrl !== "" && isUsableTileTemplate(tileUrl)) {
    const attribution = trimmed(env.attribution);
    return {
      id: "custom",
      label: "Custom tiles",
      urlTemplate: tileUrl,
      attribution: attribution !== "" ? attribution : hostAttribution(tileUrl),
      maxZoom: OSM_BASEMAP.maxZoom,
    };
  }
  const id = trimmed(env.basemap).toLowerCase();
  if (id !== "") {
    const provider = BASEMAPS[id];
    if (provider != null) return provider;
  }
  return OSM_BASEMAP;
}

/** Build-time configuration read from Vite env (empty when unset). */
export function basemapEnvFromVite(): BasemapEnv {
  return {
    basemap: import.meta.env?.VITE_MAP_BASEMAP,
    tileUrl: import.meta.env?.VITE_MAP_TILE_URL,
    attribution: import.meta.env?.VITE_MAP_ATTRIBUTION,
  };
}

/** The basemap this build renders. Resolved once at module load. */
export const ACTIVE_BASEMAP: BasemapDefinition = resolveBasemap(basemapEnvFromVite());
