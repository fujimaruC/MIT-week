import { describe, it, expect } from 'vitest';
import { readLink, platformOf } from './link-input';

describe('readLink', () => {
  it('cuts the link out of share text', () => {
    const r = readLink('Lihat video ini, seru banget! https://vt.tiktok.com/ZSVqcg5hc/ #fyp');
    expect(r.url).toBe('https://vt.tiktok.com/ZSVqcg5hc/');
    expect(r.platform).toBe('tiktok');
    expect(r.extracted).toBe(true);
    expect(r.problem).toBeNull();
  });
  it('accepts a bare link as is', () => {
    const r = readLink('  https://youtu.be/abc123  ');
    expect(r).toMatchObject({ url: 'https://youtu.be/abc123', platform: 'youtube', extracted: false });
  });
  it('adds https to a bare domain', () => {
    expect(readLink('instagram.com/reel/xyz/').url).toBe('https://instagram.com/reel/xyz/');
  });
  it('flags empty input', () => {
    expect(readLink('   ').problem).toBe('empty');
  });
  it('flags plain words', () => {
    expect(readLink('halo apa kabar').problem).toBe('notUrl');
  });
  it.each([
    'javascript:alert(1)',
    'JaVaScRiPt:alert(document.cookie)',
    'data:text/html,<script>alert(1)</script>',
    'vbscript:msgbox(1)',
    'file:///etc/passwd',
    'ftp://example.org/x',
    'javascript:alert(1) https://tiktok.com/x',
    'intent://scan/#Intent;scheme=zxing;end'
  ])('refuses %s', (input) => {
    const r = readLink(input);
    expect(r.url).toBeNull();
    expect(r.problem).toBe('scheme');
  });
  it('rejects overlong links', () => {
    expect(readLink(`https://example.org/${'a'.repeat(2100)}`).problem).toBe('long');
  });
  it('does not treat injection text as a link', () => {
    expect(readLink('<script>alert(1)</script>').url).toBeNull();
    expect(readLink("'; DROP TABLE users;--").url).toBeNull();
  });
});

describe('platformOf', () => {
  it.each([
    ['https://www.youtube.com/shorts/abc', 'youtube'],
    ['https://m.youtube.com/watch?v=1', 'youtube'],
    ['https://vm.tiktok.com/abc', 'tiktok'],
    ['https://www.instagram.com/reel/x', 'instagram'],
    ['https://fb.watch/abc', 'facebook'],
    ['https://web.facebook.com/x', 'facebook'],
    ['https://x.com/a/status/1', 'x'],
    ['https://mobile.twitter.com/a/status/1', 'x']
  ])('%s is %s', (url, platform) => expect(platformOf(url)).toBe(platform));

  it.each([
    'https://tiktok.com.evil.io/abc',
    'https://eviltiktok.com/abc',
    'https://youtube.com@evil.io/abc',
    'https://notyoutube.com',
    'https://x.com.evil.io',
    'https://example.org/?u=https://tiktok.com',
    'javascript:alert(1)'
  ])('%s is not a platform', (url) => expect(platformOf(url)).toBeNull());
});
