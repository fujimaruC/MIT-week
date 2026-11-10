#!/usr/bin/env node
/**
 * Adds <link rel="preload"> for the Latin subsets of the self-hosted fonts to
 * the SPA shell. Without it the fonts are only discovered after the CSS is
 * parsed, arrive after first paint, swap in, and shift the hero (CLS 0.13 on
 * the home page; PRD §5 requires CLS 0). Runs after `vite build`.
 */
import fs from 'node:fs';
import path from 'node:path';

const BUILD = 'build';
const ASSETS = path.join(BUILD, '_app/immutable/assets');
const WANTED = [/^google-sans-flex-latin-wght-normal\..+\.woff2$/];

const files = fs.readdirSync(ASSETS);
const links = WANTED.map((re) => {
  const file = files.find((f) => re.test(f));
  if (!file) {
    console.error(`FAIL  no font matching ${re} in ${ASSETS}`);
    process.exit(1);
  }
  return `<link rel="preload" href="/_app/immutable/assets/${file}" as="font" type="font/woff2" crossorigin>`;
});

let patched = 0;
for (const page of fs.readdirSync(BUILD).filter((f) => f.endsWith('.html'))) {
  const p = path.join(BUILD, page);
  let html = fs.readFileSync(p, 'utf8');
  if (html.includes('rel="preload" href="/_app/immutable/assets/')) continue;
  html = html.replace('<meta charset="utf-8" />', `<meta charset="utf-8" />\n    ${links.join('\n    ')}`);
  fs.writeFileSync(p, html);
  patched++;
}
console.log(`preloaded ${links.length} fonts in ${patched} page(s)`);
