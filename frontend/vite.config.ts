import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';
import { loadEnv } from 'vite';
import type { sveltekit as sveltekitPlugin } from '@sveltejs/kit/vite';
import { securityHeaders } from './tools/vite-security-headers.ts';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';

/**
 * CSP hash of the inline pre-paint script in src/app.html. SvelteKit only
 * hashes the scripts it injects itself, so this one is hashed here, from the
 * file, and can never drift from its content.
 */
function appHtmlScriptHash(): `sha256-${string}` {
  const html = readFileSync(new URL('./src/app.html', import.meta.url), 'utf8');
  const match = html.match(/<script>([\s\S]*?)<\/script>/);
  if (!match) throw new Error('src/app.html: inline pre-paint script not found');
  return `sha256-${createHash('sha256').update(match[1]).digest('base64')}`;
}

/**
 * connect-src must name the real API origin. Hardcoding localhost meant a
 * deployed build silently blocked every audit request, so the origin is
 * derived from the same variable the client uses.
 */
// The CSP directive types are not exported by name, so they are derived from
// the plugin's own config signature. This keeps the array typed without a cast.
type KitConfig = NonNullable<Parameters<typeof sveltekitPlugin>[0]>;
type CspDirectives = NonNullable<KitConfig['csp']>['directives'];
type ConnectSources = NonNullable<NonNullable<CspDirectives>['connect-src']>;
type ConnectSource = ConnectSources[number];

function connectSources(mode: string): ConnectSources {
  const env = loadEnv(mode, process.cwd(), '');
  const base = env.VITE_API_BASE_URL || '/api/v1';
  if (base.startsWith('/')) return ['self'];
  try {
    // In development the Go server is also reachable on both loopback names.
    // `URL.origin` is typed as plain string; CSP host sources are template
    // literal types, so this one cast is unavoidable and safe: origin is
    // always `scheme://host[:port]`.
    const origins: ConnectSources = [new URL(base).origin as ConnectSource];
    if (mode === 'development') origins.push('http://localhost:8080', 'http://127.0.0.1:8080');
    return ['self', ...origins];
  } catch {
    return ['self'];
  }
}

// SvelteKit 3 takes its config through the plugin, not svelte.config.js.
export default defineConfig(({ mode }) => ({
  plugins: [
    sveltekit({
      // SPA: no SSR, single static bundle with a fallback page.
      adapter: adapter({ fallback: 'index.html', strict: false }),
      prerender: { entries: [] },
      alias: { '$lib': 'src/lib' },
      // SvelteKit injects an inline bootstrap script; with mode 'hash' it adds
      // that script's sha256 to script-src automatically. A hardcoded
      // script-src 'self' would block hydration and render a blank page.
      csp: {
        mode: 'hash',
        directives: {
          'default-src': ['self'],
          // Cloudflare's edge injects its Web Analytics beacon into the page;
          // without these two sources every public page logs a CSP error.
          'script-src': ['self', appHtmlScriptHash(), 'https://static.cloudflareinsights.com'],
          'style-src': ['self', 'unsafe-inline'],
          'img-src': ['self', 'data:', 'blob:'],
          'font-src': ['self'],
          'connect-src': [...connectSources(mode), 'https://cloudflareinsights.com'],
          'worker-src': ['self', 'blob:'],
          // Required by Svelte's runtime and SvelteKit's service-worker
          // registration. Narrowing this list further blocks hydration.
          'trusted-types': ['svelte-trusted-html', 'sveltekit-trusted-url'],
          'frame-ancestors': ['none'],
          'base-uri': ['none'],
          'object-src': ['none']
        }
      }
    }),
    securityHeaders()
  ],
  // `vite preview` has no reverse proxy of its own, and a production build calls
  // the same-origin /api/v1. The e2e suite (playwright.config.ts) sets
  // E2E_API_PROXY so those calls reach the Go API it starts; nothing else sets it.
  preview: process.env.E2E_API_PROXY ? { proxy: { '/api': process.env.E2E_API_PROXY } } : undefined,
  test: {
    include: ['src/**/*.{test,spec}.{js,ts}'],
    environment: 'jsdom',
    setupFiles: ['./vitest-setup.ts']
  },
  // Component tests mount Svelte in jsdom, so Vitest must load Svelte's
  // browser build; without this `mount` resolves to the server entry and throws.
  resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined
}));
