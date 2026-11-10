#!/usr/bin/env node
/**
 * Lighthouse gate (PRD §5, TECH_SPEC §11): every category >= 90 and
 * accessibility >= 95 on each public route, measured on the production build
 * with Lighthouse's default mobile profile (simulated 4G, mid-range phone).
 *
 * Usage: npm run build && npm run lighthouse
 *   LH_URL=https://amba.example npm run lighthouse   # audit a deployment
 *   CHROME_PATH=/path/to/chrome                       # pick the browser
 *
 * Without LH_URL the script serves ./build with `vite preview` itself.
 *
 * Performance under simulated throttling varies by a few points between runs,
 * so each route is measured RUNS times and the median of each category is
 * compared with the minimum (the approach Lighthouse CI recommends).
 */
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import lighthouse from 'lighthouse';
import * as chromeLauncher from 'chrome-launcher';

const ROUTES = ['/', '/deteksi', '/edukasi', '/tentang'];
const MIN = { performance: 90, accessibility: 95, 'best-practices': 90, seo: 90 };
const PORT = 4174;
const RUNS = Number(process.env.LH_RUNS ?? 5);

const median = (values) => {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)];
};

async function waitFor(url, timeoutMs = 30_000) {
  const end = Date.now() + timeoutMs;
  while (Date.now() < end) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`server at ${url} did not start`);
}

let preview = null;
let base = process.env.LH_URL?.replace(/\/$/, '');
if (!base) {
  if (!fs.existsSync('build/index.html')) {
    console.error('No build output. Run `npm run build` first.');
    process.exit(1);
  }
  preview = spawn('npx', ['vite', 'preview', '--port', String(PORT), '--strictPort'], {
    stdio: 'ignore',
    detached: true
  });
  base = `http://localhost:${PORT}`;
  await waitFor(base);
}

const chrome = await chromeLauncher.launch({
  chromePath: process.env.CHROME_PATH || undefined,
  chromeFlags: ['--headless=new', '--no-sandbox', '--disable-gpu']
});

let failed = false;
const rows = [];
try {
  for (const route of ROUTES) {
    const runs = [];
    for (let i = 0; i < RUNS; i++) {
      const result = await lighthouse(`${base}${route}`, {
        port: chrome.port,
        output: 'json',
        logLevel: 'error',
        onlyCategories: Object.keys(MIN)
      });
      runs.push(result.lhr);
    }
    const scores = {};
    for (const [id, min] of Object.entries(MIN)) {
      const score = median(runs.map((lhr) => Math.round((lhr.categories[id]?.score ?? 0) * 100)));
      scores[id] = score;
      if (score < min) failed = true;
    }
    const lcp = median(runs.map((lhr) => lhr.audits['largest-contentful-paint']?.numericValue ?? 0));
    const cls = Math.max(...runs.map((lhr) => lhr.audits['cumulative-layout-shift']?.numericValue ?? 0));
    if (cls > 0.1) failed = true;
    rows.push({ route, ...scores, lcp_s: (lcp / 1000).toFixed(2), cls_max: cls.toFixed(3) });
  }
} finally {
  await chrome.kill();
  if (preview) {
    try {
      process.kill(-preview.pid);
    } catch {
      // already gone
    }
  }
}

console.table(rows);
if (failed) {
  console.error(`FAIL  minimums: ${Object.entries(MIN).map(([k, v]) => `${k} ${v}`).join(', ')}`);
  process.exit(1);
}
console.log('lighthouse OK — every route meets the minimums');
