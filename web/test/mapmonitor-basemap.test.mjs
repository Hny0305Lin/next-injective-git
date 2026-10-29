import assert from "node:assert/strict";
import { readFile, stat } from "node:fs/promises";
import test from "node:test";

import {
  BASEMAPS,
  DEFAULT_BASEMAP_ID,
  OSM_ATTRIBUTION,
  OSM_BASEMAP,
  OSM_TILE_URL,
  isUsableTileTemplate,
  resolveBasemap,
} from "../src/lib/basemaps.ts";

async function source(path) {
  return readFile(new URL(path, import.meta.url), "utf8");
}

test("the default basemap is keyless OpenStreetMap with its licence credit", () => {
  assert.equal(DEFAULT_BASEMAP_ID, "osm");
  assert.equal(BASEMAPS.osm, OSM_BASEMAP);
  assert.equal(resolveBasemap().id, "osm");
  assert.equal(resolveBasemap({}).urlTemplate, OSM_TILE_URL);
  assert.match(OSM_TILE_URL, /\{z\}/);
  assert.match(OSM_TILE_URL, /\{x\}/);
  assert.match(OSM_TILE_URL, /\{y\}/);
  // ODbL requires a visible credit that links to the copyright page.
  assert.match(OSM_ATTRIBUTION, /OpenStreetMap/);
  assert.match(OSM_ATTRIBUTION, /https:\/\/www\.openstreetmap\.org\/copyright/);
  assert.equal(OSM_BASEMAP.maxZoom, 19);
});

test("an unknown or empty basemap id falls back to OpenStreetMap", () => {
  assert.equal(resolveBasemap({ basemap: "" }).id, "osm");
  assert.equal(resolveBasemap({ basemap: "   " }).id, "osm");
  assert.equal(resolveBasemap({ basemap: "amap" }).id, "osm");
  assert.equal(resolveBasemap({ basemap: "tencent" }).id, "osm");
  // Case-insensitive lookup keeps hand-written env values working.
  assert.equal(resolveBasemap({ basemap: "OSM" }).id, "osm");
});

test("a custom XYZ tile provider overrides the registry and keeps its credit", () => {
  const provider = resolveBasemap({
    tileUrl: "https://tiles.example.net/{z}/{x}/{y}.png",
    attribution: "&copy; Example Maps",
  });
  assert.equal(provider.id, "custom");
  assert.equal(provider.urlTemplate, "https://tiles.example.net/{z}/{x}/{y}.png");
  assert.equal(provider.attribution, "&copy; Example Maps");
  // The tile URL wins over a registry id, so operators only set one variable.
  assert.equal(
    resolveBasemap({
      basemap: "osm",
      tileUrl: "https://tiles.example.net/{z}/{x}/{y}.png",
      attribution: "Example",
    }).id,
    "custom",
  );
});

test("a custom provider without an attribution line still names its tile host", () => {
  const provider = resolveBasemap({ tileUrl: "https://tiles.example.net/{z}/{x}/{y}.png" });
  assert.equal(provider.id, "custom");
  assert.equal(provider.attribution, "tiles.example.net");
});

test("unusable tile templates never replace the working basemap", () => {
  assert.equal(isUsableTileTemplate("https://t.example/{z}/{x}/{y}.png"), true);
  // Missing Leaflet placeholders or a non-HTTP scheme would render one
  // stretched tile / no tiles at all.
  assert.equal(isUsableTileTemplate("https://t.example/tiles.png"), false);
  assert.equal(isUsableTileTemplate("https://t.example/{z}/{x}.png"), false);
  assert.equal(isUsableTileTemplate("ftp://t.example/{z}/{x}/{y}.png"), false);
  assert.equal(resolveBasemap({ tileUrl: "https://t.example/tiles.png" }).id, "osm");
  assert.equal(resolveBasemap({ tileUrl: "javascript:alert(1)" }).id, "osm");
});

test("MapMonitor renders a single Leaflet engine over the configured basemap", async () => {
  const page = await source("../src/pages/MapMonitor.tsx");
  // One engine only: the AMap/GCJ-02 path and its fallback switch are gone.
  assert.doesNotMatch(page, /amap/i);
  assert.doesNotMatch(page, /loadAMap|createAMapEngine|wgsToGcj|AMAP_KEY/);
  assert.doesNotMatch(page, /VITE_AMAP/);
  // The tile layer comes from the registry, and the attribution control stays
  // on because the tile provider's licence requires the credit.
  assert.match(page, /import \{ ACTIVE_BASEMAP, type BasemapDefinition \} from "\.\.\/lib\/basemaps"/);
  assert.match(page, /function createLeafletEngine\(element: HTMLElement, cb: EngineCallbacks, basemap: BasemapDefinition = ACTIVE_BASEMAP\)/);
  assert.match(page, /L\.tileLayer\(basemap\.urlTemplate, \{/);
  assert.match(page, /attribution: basemap\.attribution/);
  // Leaflet's own control is added explicitly (bottom-left, no "Leaflet"
  // prefix) rather than left at its bottom-right default, where the storage
  // card would cover the required credit.
  assert.match(page, /L\.control\.attribution\(\{ position: "bottomleft", prefix: "" \}\)\.addTo\(map\)/);
  // No hardcoded tile endpoint may survive outside the registry.
  assert.doesNotMatch(page, /tile\.openstreetmap\.org/);
  // Coordinates reach Leaflet as WGS-84 [lat, lng] with no datum shift.
  assert.match(page, /L\.marker\(point\.latlng/);
});

test("the basemap registry is the only place tile endpoints live", async () => {
  const registry = await source("../src/lib/basemaps.ts");
  assert.match(registry, /export const BASEMAPS/);
  assert.match(registry, /export function resolveBasemap/);
  assert.match(registry, /VITE_MAP_BASEMAP/);
  assert.match(registry, /VITE_MAP_TILE_URL/);
  assert.match(registry, /VITE_MAP_ATTRIBUTION/);
  // No AMap loader module and no AMap key variables may remain in the app.
  await assert.rejects(stat(new URL("../src/lib/amap.ts", import.meta.url)));
  const env = await source("../src/vite-env.d.ts");
  assert.doesNotMatch(env, /AMAP/);
  assert.match(env, /VITE_MAP_BASEMAP/);
  // Stylesheets must not carry AMap-only overrides anymore.
  const styles = await source("../src/styles.css");
  assert.doesNotMatch(styles, /amap/i);
});
