import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { buildMockDossier, decodeMockToken, isMockToken, mockAuditStream, videoPlatform } from './mock';

describe('buildMockDossier', () => {
  it('marks every preview result as a demo and signs a decodable preview token', () => {
    const dossier = buildMockDossier('url', { url: 'https://bit.ly/verifikasi-akun' });

    expect(dossier.is_demo).toBe(true);
    expect(isMockToken(dossier.share_token)).toBe(true);
    expect(decodeMockToken(dossier.share_token)?.preview).toBe(true);
  });

  it('does not invent a threat for an official link', () => {
    const dossier = buildMockDossier('url', { url: 'https://www.kemkes.go.id/' });

    expect(dossier.verdict.status).not.toBe('TAMPERED');
    expect(dossier.pillars.link.reasons.map((r) => r.code)).not.toContain('LOOKALIKE');
  });

  it('explains suspicious URL signals', () => {
    const dossier = buildMockDossier('url', {
      url: 'https://bit.ly/verifikasi-akun'
    });

    expect(dossier.pillars.link.available).toBe(true);
    expect(dossier.pillars.link.reasons.map((r) => r.code)).toContain('SHORTENER');
    expect(dossier.verdict.status).not.toBe('SAFE');
    expect(dossier.pillars.ela.available).toBe(false);
  });

  it('simulates a local ELA preview for an uploaded image', () => {
    const file = new File(['not-a-real-image'], 'tangkapan.png', { type: 'image/png' });
    const dossier = buildMockDossier('file', { file });

    expect(dossier.input_type).toBe('FILE');
    expect(dossier.pillars.ela.available).toBe(true);
    expect(dossier.pillars.ela.anomalies).toHaveLength(1);
    expect(dossier.pillars.ela.reliability).toBeLessThan(1);
  });
});

describe('social video links in preview mode', () => {
  // jsdom has no 2D canvas; say so quietly instead of logging "not implemented".
  beforeEach(() => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });
  afterEach(() => vi.restoreAllMocks());

  it('recognises TikTok, YouTube and X video links only', () => {
    expect(videoPlatform('https://www.tiktok.com/@contoh.simulasi/video/7300000000000000001')).toBe('tiktok');
    expect(videoPlatform('https://vt.tiktok.com/ZSabc123/')).toBe('tiktok');
    expect(videoPlatform('https://www.youtube.com/watch?v=dQw4w9WgXcQ')).toBe('youtube');
    expect(videoPlatform('https://youtube.com/shorts/abc123')).toBe('youtube');
    expect(videoPlatform('https://youtu.be/abc123')).toBe('youtube');
    expect(videoPlatform('https://x.com/akun_berita/status/1855000000000000000')).toBe('x');
    expect(videoPlatform('https://twitter.com/akun/status/1')).toBe('x');

    for (const url of [
      'https://www.tiktok.com/@contoh.simulasi',
      'https://www.youtube.com/',
      'https://x.com/akun_berita',
      'https://tiktok.com.hadiah.xyz/@a/video/1',
      'https://bit.ly/verifikasi-akun',
      'bukan tautan'
    ]) {
      expect(videoPlatform(url), url).toBeNull();
    }
  });

  it('returns a realistic video analysis with JPEG frames for each platform', () => {
    for (const [url, platform] of [
      ['https://www.tiktok.com/@contoh.simulasi/video/7300000000000000001', 'tiktok'],
      ['https://www.youtube.com/watch?v=dQw4w9WgXcQ', 'youtube'],
      ['https://x.com/akun_berita/status/1855000000000000000', 'x']
    ]) {
      const dossier = buildMockDossier('url', { url });
      const video = dossier.video;

      expect(dossier.is_demo).toBe(true);
      expect(video?.status).toBe('OK');
      expect(video?.platform).toBe(platform);
      expect(video?.frames.length).toBeGreaterThanOrEqual(4);
      expect(video?.frames.length).toBeLessThanOrEqual(6);
      for (const frame of video?.frames ?? []) {
        expect(frame.thumb.startsWith('data:image/jpeg;base64,')).toBe(true);
        expect(frame.at_sec).toBeLessThan(video?.duration_sec ?? 0);
      }
      expect(video?.summary).not.toBe('');
      expect(video?.transcript).not.toBe('');
      // The video's AI score lives in the AI pillar, as on the server.
      expect(dossier.pillars.ai.available).toBe(true);
      expect(dossier.source_metadata?.platform).toBe(platform);
    }
  });

  it('keeps every other link a plain link check', () => {
    for (const url of ['https://bit.ly/verifikasi-akun', 'https://www.tiktok.com/@contoh.simulasi']) {
      const dossier = buildMockDossier('url', { url });
      expect(dossier.video).toBeNull();
      expect(dossier.source_metadata?.status).toBe('UNAVAILABLE');
      expect(dossier.pillars.ai.available).toBe(false);
    }
  });
});

describe('mockAuditStream', () => {
  it('emits the same stage → partial → verdict → done order as the API', async () => {
    const controller = new AbortController();
    const events = [];
    for await (const event of mockAuditStream(
      'url',
      { url: 'https://bri-verifikasi.xyz/login' },
      controller.signal,
      { step: 0, settle: 0 }
    )) {
      events.push(event);
    }

    expect(events[0]).toMatchObject({ type: 'stage' });
    expect(events.some((event) => event.type === 'partial')).toBe(true);
    expect(events.some((event) => event.type === 'verdict')).toBe(true);
    expect(events.at(-1)).toEqual({ type: 'done' });
  });

  it('stops without yielding a verdict after cancellation', async () => {
    const controller = new AbortController();
    controller.abort();
    const events = [];
    for await (const event of mockAuditStream(
      'url',
      { url: 'https://example.org/' },
      controller.signal,
      { step: 0, settle: 0 }
    )) {
      events.push(event);
    }

    expect(events).toEqual([]);
  });
});
