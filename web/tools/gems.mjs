// Draws the gems into web/public/gems: `node web/tools/gems.mjs`. CI checks the
// files match this script. The herdr plugin (multiversectl/herdr-plugin)
// copies the same files.
//
// A repository is a gem; its colour is its place among the multiverse's
// repositories, sorted (COLOURS[i % 6]), so it is the same in every universe.
// A universe is the ring of its repositories' gems, put together where it is
// shown. Each colour comes in two cuts:
//   <colour>.svg     faceted, glowing, a sparkle at the core: 20px and up
//   <colour>-sm.svg  a plain stone with a dark outline: up to 18px
import { mkdirSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

// [base, light, dark], hues kept apart so they tell apart at 12px.
export const COLOURS = {
  red: ["#ff4757", "#ffc4c9", "#7a0a16"],
  blue: ["#3f7bff", "#c7d8ff", "#0a2380"],
  yellow: ["#ffc61a", "#fff1b0", "#7a5400"],
  green: ["#2fd06a", "#c2f7d4", "#085c25"],
  purple: ["#b05cff", "#e9d1ff", "#43087a"],
  cyan: ["#1fc7e0", "#c0f3fa", "#04566b"],
};
export const ORDER = Object.keys(COLOURS);

const RIM = [[12, 2.8], [16.6, 4.6], [19, 9.4], [18.2, 15.2], [14, 20.4], [9.2, 20], [5.6, 15.8], [5, 9.6], [7.6, 4.8]];
const TABLE = [[12, 7.4], [15, 9], [15.4, 12.8], [12.8, 15.8], [9.6, 15.2], [8.4, 11.6], [9.4, 8.4]];
const pts = (p) => p.map(([x, y]) => `${x},${y}`).join(" ");
const CX = 12, CY = 11.6;

function gradient(c) {
  const [base, light, dark] = COLOURS[c];
  return `<radialGradient id="s" cx="${CX - 1}" cy="${CY - 1.2}" r="9" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="${light}"/><stop offset=".45" stop-color="${base}"/><stop offset="1" stop-color="${dark}"/></radialGradient>`;
}

export function large(c) {
  const [base, light, dark] = COLOURS[c];
  const facets = RIM.map(([x, y], i) => {
    const [tx, ty] = TABLE[Math.round((i * TABLE.length) / RIM.length) % TABLE.length];
    return `M${x} ${y}L${tx} ${ty}`;
  }).join("");
  return `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="1.5 0.5 21 22" fill="none">`
    + `<defs>${gradient(c)}<radialGradient id="g" cx="${CX}" cy="${CY}" r="11.5" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="${base}" stop-opacity=".55"/><stop offset="1" stop-color="${base}" stop-opacity="0"/></radialGradient></defs>`
    + `<circle cx="${CX}" cy="${CY}" r="11.5" fill="url(#g)"/>`
    + `<polygon points="${pts(RIM)}" fill="url(#s)" stroke="${dark}" stroke-opacity=".8" stroke-width=".7" stroke-linejoin="round"/>`
    + `<polygon points="${pts(TABLE)}" fill="${light}" fill-opacity=".18" stroke="#fff" stroke-opacity=".35" stroke-width=".45" stroke-linejoin="round"/>`
    + `<path d="${facets}" stroke="#fff" stroke-opacity=".28" stroke-width=".4"/>`
    + `<path d="M11.4 10.6l.5-2.6.5 2.6 2.6.5-2.6.5-.5 2.6-.5-2.6-2.6-.5z" fill="#fff" fill-opacity=".9"/>`
    + `<circle cx="11.9" cy="11.1" r=".9" fill="#fff"/></svg>\n`;
}

export function small(c) {
  const [, light, dark] = COLOURS[c];
  return `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="4 2 16 19.2" fill="none">`
    + `<defs>${gradient(c)}</defs>`
    + `<polygon points="${pts(RIM)}" fill="url(#s)" stroke="${dark}" stroke-width="1.1" stroke-linejoin="round"/>`
    + `<polygon points="${pts(TABLE)}" fill="${light}" fill-opacity=".3"/>`
    + `<circle cx="10.4" cy="9.4" r="1.5" fill="#fff" fill-opacity=".85"/></svg>\n`;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const dir = new URL("../public/gems/", import.meta.url);
  mkdirSync(dir, { recursive: true });
  for (const c of ORDER) {
    writeFileSync(new URL(`${c}.svg`, dir), large(c));
    writeFileSync(new URL(`${c}-sm.svg`, dir), small(c));
  }
}
