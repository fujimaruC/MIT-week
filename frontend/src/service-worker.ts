/// <reference lib="webworker" />
/**
 * Service worker with two jobs:
 *
 * 1. PWA share target: accept the OS share POST, hold the item for a few
 *    seconds in Cache Storage, and redirect into /deteksi (TECH_SPEC §8.1).
 * 2. Offline app shell: precache this build's own static files so the demo
 *    cases, Edukasi work without a connection (PRD F8, F15).
 *    Only the app's own files are cached; API responses and user content
 *    never are.
 */
import { immutable, assets } from '$app/manifest';
import { version } from '$app/env';
import {
  SHARE_CACHE,
  SHARE_MAX_AGE_MS,
  firstHttpUrl,
  mediaKey,
  shareKey,
  validateShareFile,
  type ShareMeta
} from './lib/share-target';

const sw = self as unknown as ServiceWorkerGlobalScope;

const APP_CACHE = `amba-app-${version}`;
const SHELL = new URL('./', sw.registration.scope).pathname;
const ASSETS = new Set(
  [...immutable, ...assets].map(({ path }) => new URL(path, sw.registration.scope).pathname)
);

sw.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      try {
        const cache = await caches.open(APP_CACHE);
        await cache.addAll([...ASSETS, SHELL]);
      } catch {
        // Offline support is a bonus; a failed precache must not block install.
      }
      await sw.skipWaiting();
    })()
  );
});

sw.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      for (const key of await caches.keys()) {
        if (key.startsWith('amba-app-') && key !== APP_CACHE) await caches.delete(key);
      }
      await sw.clients.claim();
      await sweep();
    })()
  );
});

sw.addEventListener('message', (event) => {
  if (event.data?.type === 'amba-sweep-share') event.waitUntil(sweep());
});

sw.addEventListener('fetch', (event) => {
  const request = event.request;
  const url = new URL(request.url);

  if (request.method === 'POST') {
    if (url.pathname.endsWith('/share-target')) event.respondWith(handleShare(request));
    return;
  }
  if (request.method !== 'GET' || url.origin !== sw.location.origin) return;

  // Hashed build files and static assets never change for this version.
  if (ASSETS.has(url.pathname)) {
    event.respondWith(cacheFirst(request));
    return;
  }
  // Navigations: always try the network (fresh deploys win), fall back to the
  // cached shell so the SPA still opens offline.
  if (request.mode === 'navigate') {
    event.respondWith(
      (async () => {
        try {
          return await fetch(request);
        } catch {
          const cached = await caches.match(SHELL);
          return cached ?? Response.error();
        }
      })()
    );
  }
});

async function cacheFirst(request: Request): Promise<Response> {
  const cached = await caches.match(request);
  if (cached) return cached;
  const response = await fetch(request);
  if (response.ok) {
    const cache = await caches.open(APP_CACHE);
    void cache.put(request, response.clone());
  }
  return response;
}

async function handleShare(request: Request): Promise<Response> {
  try {
    const form = await request.formData();
    const title = String(form.get('title') ?? '').slice(0, 300);
    const text = String(form.get('text') ?? '').slice(0, 5000);
    const url = String(form.get('url') ?? '').slice(0, 2048);

    const media = form.get('media');
    const file = media instanceof File ? media : null;
    if (file) {
      const problem = validateShareFile(file);
      if (problem) return redirect('invalid');
    }

    const resolvedUrl = url || firstHttpUrl(text) || '';
    if (!resolvedUrl && !text && !file) return redirect('empty');

    const id = crypto.randomUUID();
    const meta: ShareMeta = {
      ts: Date.now(),
      url: resolvedUrl || undefined,
      text: text || undefined,
      title: title || undefined,
      hasFile: !!file,
      fileName: file?.name,
      fileType: file?.type
    };

    const cache = await caches.open(SHARE_CACHE);
    await cache.put(
      shareKey(id),
      new Response(JSON.stringify(meta), { headers: { 'Content-Type': 'application/json' } })
    );
    if (file) {
      await cache.put(
        mediaKey(id),
        new Response(file, { headers: { 'Content-Type': file.type } })
      );
    }

    return redirect(`deteksi?share=${encodeURIComponent(id)}`);
  } catch {
    return redirect('invalid');
  }
}

function redirect(path: string): Response {
  return Response.redirect(new URL(path, sw.registration.scope).href, 303);
}

async function sweep(): Promise<void> {
  try {
    const cache = await caches.open(SHARE_CACHE);
    const keys = await cache.keys();
    const ids = new Set<string>();

    for (const request of keys) {
      const match = new URL(request.url).pathname.match(/\/__share\/([^/]+)$/);
      if (match) ids.add(match[1]);
    }

    for (const id of ids) {
      const response = await cache.match(shareKey(id));
      if (!response) {
        await cache.delete(mediaKey(id));
        continue;
      }
      const meta = (await response.json()) as ShareMeta;
      if (Date.now() - meta.ts > SHARE_MAX_AGE_MS) {
        await cache.delete(shareKey(id));
        await cache.delete(mediaKey(id));
      }
    }
  } catch {
    // A failed sweep must never break the app.
  }
}
