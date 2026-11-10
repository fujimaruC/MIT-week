import { describe, it, expect } from 'vitest';
import { id } from './id';
import { t } from './index.svelte';
import { aiLines, aiLevel } from './verdict';
import type { DossierParsed } from '$lib/api/schemas';

describe('i18n (Indonesian only)', () => {
  it('every key has non-empty copy', () => {
    for (const key of Object.keys(id) as Array<keyof typeof id>) {
      expect(id[key], key).not.toBe('');
    }
  });

  it('fills placeholders and falls back to the key for an unknown one', () => {
    expect(t('score.of100', { score: 85 })).toBe('Skor risiko 85 dari 100');
    expect(t('nope.missing' as keyof typeof id)).toBe('nope.missing');
  });

  it('never claims certainty in the interface copy', () => {
    for (const [key, text] of Object.entries(id)) {
      if (key === 'disclaimer.full' || key.startsWith('edu.q1.') || key === 'about.p1') continue;
      expect(/\b(100%|dijamin|pasti benar)\b/i.test(text), `${key}: ${text}`).toBe(false);
    }
  });
});

describe('Jejak AI card', () => {
  const ai = (over: Partial<DossierParsed['pillars']['ai']>): DossierParsed['pillars']['ai'] => ({
    available: true,
    score: 20,
    indicators: [],
    caveat: 'Indikatif, bukan bukti pasti',
    provenance: { kind: 'NONE', source: '', generator: '', camera: '', c2pa: false },
    detector: null,
    ...over
  });

  it('names what the file declares before any visual judgement', () => {
    const lines = aiLines(
      ai({ score: 95, provenance: { kind: 'AI_GENERATED', source: 'c2pa', generator: 'OpenAI', camera: '', c2pa: true } }),
      'FILE'
    );
    expect(lines).toEqual(['Content Credentials (C2PA) di file menyatakan gambar ini dibuat dengan AI (OpenAI).']);
  });

  it('says how likely an image without metadata looks AI-made, and that missing metadata proves nothing', () => {
    const lines = aiLines(ai({ score: 82, detector: { name: 'sightengine', score: 97 } }), 'FILE');
    expect(lines[0]).toBe('Kemungkinan dibuat dengan AI: tinggi.');
    expect(lines[1]).toContain('bukan bukti apa pun');
    expect(lines[2]).toBe('Pemeriksa khusus AI (Sightengine): 97/100.');
    expect(aiLevel(39)).toBe('low');
    expect(aiLevel(40)).toBe('medium');
    expect(aiLevel(70)).toBe('high');
  });

  it('treats camera data as a hint and says which image a link check judged', () => {
    const lines = aiLines(
      ai({ score: 50, provenance: { kind: 'CAMERA', source: 'exif', generator: '', camera: 'Canon EOS 80D', c2pa: false } }),
      'URL'
    );
    expect(lines).toEqual([
      'Yang diperiksa: gambar pratinjau tautan ini.',
      'Kemungkinan dibuat dengan AI: sedang.',
      'Data kamera di file: Canon EOS 80D. Ini petunjuk, bukan bukti, karena data ini bisa ditulis siapa saja.'
    ]);
  });

  it('copes with results made before provenance existed', () => {
    expect(aiLines(ai({ score: 10, provenance: null }), 'FILE')).toEqual(['Kemungkinan dibuat dengan AI: rendah.']);
  });
});
