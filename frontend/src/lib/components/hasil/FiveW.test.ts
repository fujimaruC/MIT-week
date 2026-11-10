import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import FiveW from './FiveW.svelte';

const w5h1 = {
  what: 'Banjir menyeret mobil di jalan kota.',
  who: '',
  when: 'Disebut pagi ini.',
  where: 'Disebut Jakarta.',
  why: '',
  how: 'Air naik setinggi atap mobil.'
};

describe('FiveW', () => {
  it('answers the six questions and marks the ones the video does not show', () => {
    render(FiveW, { w5h1, research: null });
    for (const label of ['Apa', 'Siapa', 'Kapan', 'Di mana', 'Mengapa', 'Bagaimana']) {
      expect(screen.getByText(label)).toBeTruthy();
    }
    expect(screen.getByText('Banjir menyeret mobil di jalan kota.')).toBeTruthy();
    expect(screen.getAllByText('Tidak terlihat di video')).toHaveLength(2);
  });

  it('lists trusted sources as links, and never links a non-web address', () => {
    render(FiveW, {
      w5h1,
      research: {
        query: 'banjir jakarta mobil terseret',
        status: 'FOUND',
        sources: [
          { title: 'Cek fakta: video banjir', url: 'https://www.kompas.com/cekfakta/x', domain: 'kompas.com', snippet: 'Video ini bukan di Jakarta.', published: '2026-11-08' },
          { title: 'Sumber aneh', url: 'javascript:alert(1)', domain: 'contoh.id', snippet: '', published: '' }
        ]
      }
    });
    const link = screen.getByRole('link', { name: /Cek fakta: video banjir/ });
    expect(link.getAttribute('href')).toBe('https://www.kompas.com/cekfakta/x');
    expect(link.getAttribute('rel')).toContain('noopener');
    expect(screen.queryByRole('link', { name: /Sumber aneh/ })).toBeNull();
    expect(screen.getByText('Sumber aneh')).toBeTruthy();
    expect(screen.getByText('Kata kunci: banjir jakarta mobil terseret')).toBeTruthy();
  });

  it('says so plainly when the search found nothing or could not run', () => {
    const { unmount } = render(FiveW, { w5h1: null, research: { query: 'x', status: 'NONE', sources: [] } });
    expect(screen.getByText(/Belum ada pemberitaan dari sumber tepercaya/)).toBeTruthy();
    unmount();
    render(FiveW, { w5h1: null, research: { query: '', status: 'UNAVAILABLE', sources: [] } });
    expect(screen.getByText('Penelusuran web tidak tersedia saat ini.')).toBeTruthy();
  });
});
