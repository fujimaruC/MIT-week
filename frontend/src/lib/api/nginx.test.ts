import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const conf = readFileSync(resolve(process.cwd(), 'nginx.conf'), 'utf8');

/** nginx drops server-level add_header lines in any location that sets its own. */
describe('nginx.conf security headers', () => {
  const required = [
    'X-Content-Type-Options "nosniff"',
    'Referrer-Policy "no-referrer"',
    'Permissions-Policy "camera=(), microphone=(), geolocation=()"',
    "Content-Security-Policy \"frame-ancestors 'none'\"",
    'Strict-Transport-Security "max-age=31536000"'
  ];
  const assets = conf.slice(conf.indexOf('location /_app/'), conf.indexOf('# SPA routing fallback'));

  it('sets every header at server level', () => {
    for (const h of required) expect(conf, h).toContain(`add_header ${h} always;`);
  });

  it('repeats every header on the hashed assets, which set their own add_header', () => {
    for (const h of required) expect(assets, h).toContain(`add_header ${h} always;`);
  });
});

describe('nginx.conf routing and uploads', () => {
  it('never falls back to a folder: static/edukasi/ shares its name with the /edukasi route', () => {
    expect(conf).toMatch(/try_files \$uri \/index\.html;/);
    expect(conf).not.toContain('$uri/');
  });

  it('redirects with a path only, so the tunnel scheme and port survive', () => {
    expect(conf).toContain('absolute_redirect off;');
  });

  it('lets a 10 MB upload through to the backend', () => {
    const mb = Number(conf.match(/client_max_body_size (\d+)m;/)?.[1]);
    expect(mb).toBeGreaterThan(10);
  });
});
