import { describe, expect, it } from 'vitest';
import { formatClock, formatDate, formatScore, platformName, statusLabel } from './format';

describe('statusLabel', () => {
  it('never returns a certainty claim', () => {
    const labels = (['SAFE', 'SUSPICIOUS', 'TAMPERED', 'INSUFFICIENT'] as const).map(statusLabel);
    for (const label of labels) {
      expect(label.toLowerCase()).not.toMatch(/pasti|terbukti|100%|dijamin|aman/);
    }
  });

  it('maps SAFE to the spec sentence', () => {
    expect(statusLabel('SAFE')).toBe('Tidak ditemukan tanda bahaya');
  });

  it('maps TAMPERED to the Indonesian label (hoaks, not hoax: Indonesian only, 10 Nov 2026)', () => {
    expect(statusLabel('TAMPERED')).toBe('Terindikasi hoaks atau manipulasi');
  });

  it('maps INSUFFICIENT honestly', () => {
    expect(statusLabel('INSUFFICIENT')).toBe('Tidak ditemukan tanda bahaya');
  });
});

describe('formatScore', () => {
  it('describes the score as risk, never confidence', () => {
    expect(formatScore(92)).toBe('Skor risiko 92 dari 100');
  });

  it('renders an em dash when no score exists', () => {
    expect(formatScore(null)).toBe('Belum ada skor');
  });
});

describe('formatClock', () => {
  it('reads like a player clock: m:ss, whole seconds rounded down', () => {
    expect(formatClock(0)).toBe('0:00');
    expect(formatClock(8.9)).toBe('0:08');
    expect(formatClock(47.6)).toBe('0:47');
    expect(formatClock(75)).toBe('1:15');
    expect(formatClock(599)).toBe('9:59');
  });

  it('adds hours from one hour, and treats a missing duration as zero', () => {
    expect(formatClock(3600)).toBe('1:00:00');
    expect(formatClock(3725)).toBe('1:02:05');
    expect(formatClock(-3)).toBe('0:00');
    expect(formatClock(Number.NaN)).toBe('0:00');
  });
});

describe('formatDate', () => {
  it('writes an upload date in Indonesian', () => {
    expect(formatDate('2026-11-07')).toBe('7 November 2026');
  });

  it('shows nothing for an unknown or impossible date', () => {
    expect(formatDate('')).toBe('');
    expect(formatDate('20261107')).toBe('');
    expect(formatDate('2026-02-31')).toBe('');
  });
});

describe('platformName', () => {
  it('names the social platforms and leaves the open web unnamed', () => {
    expect(platformName('tiktok')).toBe('TikTok');
    expect(platformName('youtube')).toBe('YouTube');
    expect(platformName('x')).toBe('X (Twitter)');
    expect(platformName('web')).toBe('');
    expect(platformName('constructor')).toBe('');
  });
});
