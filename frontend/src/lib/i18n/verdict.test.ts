import { describe, it, expect } from 'vitest';
import { simpleSentence, aiLines, explanation } from './verdict';
import type { DossierParsed } from '$lib/api/schemas';

/** Just enough of a dossier for the lead sentence: floors, status, video. */
function dossier(floors: string[], video: boolean): DossierParsed {
  return {
    verdict: {
      score: 52,
      status: 'SUSPICIOUS',
      short_title: 'contoh',
      explanation: 'Penjelasan contoh.',
      explanation_simple: 'Kalimat dari server.',
      basis: ['ai', 'link'],
      not_checked: ['ela', 'fact'],
      floors_applied: floors
    },
    video: video ? ({ status: 'OK' } as DossierParsed['video']) : null
  } as DossierParsed;
}

describe('lead sentence', () => {
  it("shows the server's Indonesian sentence unchanged, whatever the floors", () => {
    expect(simpleSentence(dossier(['VIDEO_CONTEXT_MISMATCH'], true))).toBe('Kalimat dari server.');
    expect(simpleSentence(dossier(['AI_LIKELY', 'LINK_STRONG'], false))).toBe('Kalimat dari server.');
  });
});

describe('Jejak AI card for a video link', () => {
  const ai: DossierParsed['pillars']['ai'] = {
    available: true,
    score: 82,
    indicators: [],
    caveat: 'Indikatif, bukan bukti pasti',
    provenance: null,
    detector: null
  };

  it('names the video frames as what was checked, not the preview image', () => {
    expect(aiLines(ai, 'URL', true)).toEqual([
      'Yang diperiksa: beberapa frame dari video tautan ini.',
      'Kemungkinan dibuat dengan AI: tinggi.'
    ]);
    expect(aiLines(ai, 'URL')[0]).toBe('Yang diperiksa: gambar pratinjau tautan ini.');
  });
});

describe('INSUFFICIENT and the not-checked sentence', () => {
  it('shows an old INSUFFICIENT payload as a neutral conclusion, not as missing data', () => {
    const d = dossier([], false);
    d.verdict.status = 'INSUFFICIENT';
    d.verdict.score = null;
    const sentence = simpleSentence(d);
    expect(sentence).toBe('Tidak ada tanda bahaya yang ditemukan, tetapi datanya terbatas.');
    expect(sentence.toLowerCase()).not.toContain('belum cukup');
  });

  it('drops the server sentence that lists what could not be checked', () => {
    const d = dossier([], false);
    d.verdict.explanation =
      'Diperiksa: jejak AI, keamanan tautan. Tidak dapat diperiksa: teks ditimpa (ELA), cek fakta. Hasil ini indikatif, bukan bukti pasti.';
    expect(explanation(d)).toBe('Diperiksa: jejak AI, keamanan tautan. Hasil ini indikatif, bukan bukti pasti.');
  });
});
