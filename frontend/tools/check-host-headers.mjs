#!/usr/bin/env node
/**
 * security-headers.json is the single source of truth for the SPA's HTTP
 * headers (TECH_SPEC §7.4). Every host config must emit each of them with the
 * same value: this fails CI when vercel.json drifts from it.
 */
import fs from 'node:fs';

const source = JSON.parse(fs.readFileSync('security-headers.json', 'utf8'));
const vercel = JSON.parse(fs.readFileSync('vercel.json', 'utf8'));
const global = vercel.headers.find((h) => h.source === '/(.*)');
if (!global) {
  console.error('FAIL  vercel.json has no "/(.*)" header block');
  process.exit(1);
}
const emitted = Object.fromEntries(global.headers.map((h) => [h.key.toLowerCase(), h.value]));

let failed = false;
for (const [key, value] of Object.entries(source)) {
  if (emitted[key.toLowerCase()] !== value) {
    console.error(`FAIL  vercel.json ${key}: expected "${value}", got "${emitted[key.toLowerCase()] ?? '(missing)'}"`);
    failed = true;
  }
}
if (failed) process.exit(1);
console.log(`headers OK — ${Object.keys(source).length} security headers emitted by vercel.json`);
