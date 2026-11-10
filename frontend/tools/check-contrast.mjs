#!/usr/bin/env node
/**
 * Fails CI if any text/background pair in tokens.css drops below its required
 * contrast: 4.5:1 for body text, 3:1 for large text, UI borders and focus ring.
 * Reads the real token file — no duplicated colour values.
 *
 * Both themes are checked. A token may be redefined under
 * `html[data-theme='dark']` (:root is the light theme), so the parser reads each theme's own block and
 * falls back to :root for anything it does not override.
 */
import fs from 'node:fs';
import path from 'node:path';

const tokenFile = path.resolve('src/lib/styles/tokens.css');
const css = fs.readFileSync(tokenFile, 'utf8');

/** Splits a declaration block into name -> raw value. */
function declarations(block) {
  const out = new Map();
  for (const m of block.matchAll(/--([a-z0-9-]+):\s*([^;]+);/g)) {
    out.set(m[1], m[2].trim());
  }
  return out;
}

/** Returns the declaration text of the first rule whose selector matches. */
function blockFor(selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const m = css.match(new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\}`));
  return m ? m[1] : '';
}

const root = declarations(blockFor(':root'));
const dark = declarations(blockFor("html[data-theme='dark']"));

/** Resolves a token for one theme, following var(--other) aliases. */
function token(name, theme, seen = new Set()) {
  if (seen.has(name)) throw new Error(`token --${name} is circular`);
  seen.add(name);
  const raw = theme.get(name) ?? root.get(name);
  if (!raw) throw new Error(`token --${name} not found in ${tokenFile}`);
  const hex = raw.match(/^#[0-9a-fA-F]{3,8}$/);
  if (hex) return hex[0];
  const alias = raw.match(/^var\(--([a-z0-9-]+)\)$/);
  if (alias) return token(alias[1], theme, seen);
  throw new Error(`token --${name} is neither a hex nor a var() alias: ${raw}`);
}

function rgb(hex) {
  let h = hex.replace('#', '');
  if (h.length === 3) h = h.split('').map((c) => c + c).join('');
  return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
}

function luminance(hex) {
  const [r, g, b] = rgb(hex).map((c) =>
    c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  );
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a, b) {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
}

const surfaces = ['bg', 'surface', 'surface-2'];
const textPairs = [
  ['text', 'bg'], ['text', 'surface'], ['text', 'surface-2'],
  ['text-2', 'bg'], ['text-2', 'surface'], ['text-2', 'surface-2'],
  ['text-3', 'bg'], ['text-3', 'surface'],
  ['accent', 'bg'], ['accent', 'surface'],
  ['accent-2', 'bg'], ['accent-2', 'surface'],
  ['ok', 'bg'], ['warn', 'bg'], ['err', 'bg'],
  ['ok', 'surface'], ['warn', 'surface'], ['err', 'surface'],
  /* Semantic roles: a fill stays pastel in both themes, so text on it is
   * --on-fill; --ink flips between themes, so text on it is --on-ink. */
  ['on-fill', 'safe'], ['on-fill', 'yellow'], ['on-fill', 'coral'],
  ['on-ink', 'ink'],
  /* --brand is the light-blue pill (text --on-brand); --indigo is the blue
   * fill of a finished step (text --on-indigo-text). */
  ['on-brand', 'brand'], ['on-indigo-text', 'indigo'],
  ['ink', 'card'], ['body-2', 'card'], ['muted', 'card'], ['indigo-text', 'card'],
  /* The dashboard frame and its work sheet. */
  ['ink', 'frame'], ['body-2', 'frame'], ['muted', 'frame'], ['ink', 'sheet'], ['body-2', 'sheet'], ['muted', 'sheet'], ['indigo-text', 'sheet'],
  ['ink', 'paper'], ['ink', 'white'],
  ['ink', 'indigo-soft'], ['ink', 'indigo-tint'], ['on-fill', 'safe'],
  ['ink', 'coral-tint'], ['ink', 'yellow-tint'],
  ['indigo-text', 'paper'], ['indigo-text', 'white'], ['indigo-text', 'indigo-tint'],
  ['muted', 'paper'], ['muted', 'white'], ['body-2', 'paper'], ['body-2', 'white']
];

let failures = 0;
const themes = [
  ['light', new Map()],
  ['dark', dark]
];

for (const [name, theme] of themes) {
  for (const [fg, bg] of textPairs) {
    const ratio = contrast(token(fg, theme), token(bg, theme));
    if (ratio < 4.5) {
      console.error(`FAIL  [${name}] ${fg} on ${bg}: ${ratio.toFixed(2)}:1 (need 4.5)`);
      failures++;
    }
  }

  for (const bg of surfaces) {
    const ratio = contrast(token('ink', theme), token(bg, theme));
    if (ratio < 3) {
      console.error(`FAIL  [${name}] border-strong on ${bg}: ${ratio.toFixed(2)}:1 (need 3.0)`);
      failures++;
    }
  }

  const focusRatio = contrast(token('ink', theme), token('bg', theme));
  if (focusRatio < 3) {
    console.error(`FAIL  [${name}] focus ring on bg: ${focusRatio.toFixed(2)}:1 (need 3.0)`);
    failures++;
  }
}

if (failures) {
  console.error(`\n${failures} contrast failure(s).`);
  process.exit(1);
}
console.log(`contrast OK — ${textPairs.length * themes.length + (surfaces.length + 1) * themes.length} pairs checked across both themes`);
