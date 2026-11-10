import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import VideoAnalysis from './VideoAnalysis.svelte';
import { id } from '$lib/i18n/id';
import type { DossierParsed } from '$lib/api/schemas';

type Video = NonNullable<DossierParsed['video']>;
const THUMB = 'data:image/jpeg;base64,/9j/4AAQ';

function video(over: Partial<Video> = {}): Video {
  return {
    status: 'OK',
    reason: '',
    platform: 'tiktok',
    duration_sec: 47.6,
    upload_date: '2026-11-07',
    platform_ai_label: true,
    frames: [
      { at_sec: 0, thumb: THUMB },
      { at_sec: 8.4, thumb: THUMB },
      { at_sec: 75, thumb: THUMB }
    ],
    audio_analyzed: true,
    summary: 'Air keruh menerjang jalan kota.',
    transcript: 'Ini Jakarta pagi ini, saudara-saudara.',
    // Scores sit on the band edges: 39 low, 40 medium, 70 high.
    manipulation: { score: 39, indicators: [] },
    context: { score: 70, indicators: ['Frame 3: rambu jalan tidak berbahasa Indonesia'] },
    w5h1: null,
    research: null,
    ...over
  };
}

const ai: DossierParsed['pillars']['ai'] = {
  available: true,
  score: 40,
  indicators: ['Frame 2: tulisan di papan toko berupa huruf acak'],
  caveat: 'Indikatif, bukan bukti pasti',
  provenance: null,
  detector: null
};

const row = (container: HTMLElement, key: string) =>
  container.querySelector<HTMLElement>(`[data-signal="${key}"]`)?.textContent ?? '';

describe('VideoAnalysis', () => {

  it('heads the card with platform, length, upload date and the platform AI label', () => {
    const { container } = render(VideoAnalysis, { video: video(), ai });

    expect(screen.getByRole('heading', { level: 2, name: 'Analisis video' })).toBeTruthy();
    const meta = container.querySelector('.meta')?.textContent ?? '';
    expect(meta).toContain('TikTok');
    expect(meta).toContain('Durasi 0:47');
    expect(meta).toContain('Diunggah 7 November 2026');
    expect(screen.getByText('Platform menandai video ini sebagai konten AI')).toBeTruthy();
  });

  it('shows the frames in a keyboard-reachable strip of images, not buttons', () => {
    render(VideoAnalysis, { video: video(), ai });

    const strip = screen.getByRole('region', { name: 'Frame yang diperiksa' });
    expect(strip.getAttribute('tabindex')).toBe('0');
    expect(within(strip).getAllByRole('img').map((img) => img.getAttribute('alt'))).toEqual([
      'Frame 1, detik 0',
      'Frame 2, detik 8',
      'Frame 3, detik 75'
    ]);
    expect(strip.textContent).toContain('1:15');
    expect(within(strip).queryAllByRole('button')).toEqual([]);
  });

  it('gives the summary, the transcript with its audio note, three levelled signals and the caveat', () => {
    const { container } = render(VideoAnalysis, { video: video(), ai });

    expect(screen.getByText('Air keruh menerjang jalan kota.')).toBeTruthy();
    expect(screen.getByText('Ucapan di video')).toBeTruthy();
    expect(screen.getByText('Suara video ikut dianalisis.')).toBeTruthy();

    expect(row(container, 'ai')).toContain('Jejak AI');
    expect(row(container, 'ai')).toContain('Sedang');
    expect(row(container, 'ai')).toContain('skor 40 dari 100');
    expect(row(container, 'ai')).toContain('Frame 2: tulisan di papan toko berupa huruf acak');
    expect(row(container, 'manipulation')).toContain('Tanda suntingan');
    expect(row(container, 'manipulation')).toContain('Rendah');
    expect(row(container, 'manipulation')).toContain('Tidak ada tanda yang dicatat.');
    // A high context score is a mismatch, and the row says so.
    expect(row(container, 'context')).toContain('Kecocokan 5W1H');
    expect(row(container, 'context')).toContain('Ketidakcocokan tinggi');
    expect(row(container, 'context')).toContain('tidak cocok dengan isi rekamannya');

    expect(screen.getByText(/^Indikatif, bukan bukti pasti/)).toBeTruthy();
  });

  it('says when the audio was not analysed and leaves out an AI check that did not run', () => {
    const { container } = render(VideoAnalysis, {
      video: video({ audio_analyzed: false }),
      ai: { ...ai, available: false }
    });

    expect(screen.getByText('Suara video tidak ikut dianalisis.')).toBeTruthy();
    expect(container.querySelector('[data-signal="ai"]')).toBeNull();
    expect(container.textContent).not.toContain('Tidak dapat diperiksa');
    expect(container.querySelectorAll('.meter')).toHaveLength(2);
  });

  it('says what an unread video was judged on, with no failure note, upload button or frames', () => {
    const { container } = render(VideoAnalysis, {
      video: video({ status: 'UNAVAILABLE', reason: 'private', frames: [], summary: '', transcript: '' }),
      ai
    });

    expect(screen.getByText('Penilaian video ini memakai label dari platform dan pemeriksaan yang tercantum di halaman ini.')).toBeTruthy();
    expect(container.textContent).not.toMatch(/belum bisa|gagal|tidak dapat|privat/i);
    expect(screen.queryByRole('button')).toBeNull();
    expect(screen.queryByRole('img')).toBeNull();
    expect(screen.queryByText('Tanda yang diperiksa')).toBeNull();
  });


  it('never claims certainty in the video copy', () => {
    for (const [key, text] of Object.entries(id).filter(([k]) => k.startsWith('video.'))) {
      expect(/terbukti|100%|dijamin/i.test(text), key).toBe(false);
      expect(text.replace('bukan bukti pasti', '').includes('pasti'), key).toBe(false);
    }
  });
});
