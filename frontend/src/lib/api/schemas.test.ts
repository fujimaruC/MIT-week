import { describe, expect, it } from 'vitest';
import { parseDossier, SchemaVersionError } from './schemas';

function minimalDossier(overrides: Record<string, unknown> = {}) {
  return {
    schema_version: 2,
    id: 'AMBA-20261030-7F3K',
    timestamp: '2026-10-30T07:22:15Z',
    input_type: 'URL',
    source_reference: 'contoh',
    degraded: false,
    is_demo: true,
    verdict: {
      score: 78,
      status: 'TAMPERED',
      short_title: 'contoh',
      explanation: 'Penjelasan contoh.',
      explanation_simple: 'Kabar ini dibantah sumber terpercaya. Jangan disebarkan.',
      basis: ['ai', 'fact'],
      not_checked: ['ela', 'link'],
      floors_applied: ['FACT_CONTRADICTED']
    },
    pillars: {
      ela: { available: false, score: 0, reliability: 0, reliability_note: null, anomalies: [] },
      ai: { available: true, score: 82, indicators: [], caveat: 'Indikatif, bukan bukti pasti' },
      fact: { available: true, claim: 'x', status: 'CONTRADICTS', sources: [] },
      link: { available: false, score: 0, final_url: '', redirect_chain: [], reasons: [] }
    },
    tactics: [],
    share_token: '',
    ...overrides
  };
}

describe('parseDossier', () => {
  it('accepts a valid dossier', () => {
    expect(parseDossier(minimalDossier()).verdict.status).toBe('TAMPERED');
  });

  it('rejects a score above 100', () => {
    const bad = minimalDossier();
    bad.verdict.score = 120;
    expect(() => parseDossier(bad)).toThrow();
  });

  it('rejects an unknown status', () => {
    const bad = minimalDossier();
    bad.verdict.status = 'PROBABLY_FINE';
    expect(() => parseDossier(bad)).toThrow();
  });

  it('allows a null score for INSUFFICIENT', () => {
    const base = minimalDossier();
    const d: Record<string, unknown> = {
      ...base,
      verdict: { ...base.verdict, score: null, status: 'INSUFFICIENT' }
    };
    expect(parseDossier(d).verdict.score).toBeNull();
  });

  it('reads the AI pillar provenance and detector, and accepts older payloads without them', () => {
    const old = parseDossier(minimalDossier());
    expect(old.pillars.ai.provenance).toBeNull();
    expect(old.pillars.ai.detector).toBeNull();

    const base = minimalDossier();
    const d = parseDossier({
      ...base,
      pillars: {
        ...base.pillars,
        ai: {
          available: true,
          score: 95,
          reliability: 1,
          indicators: [],
          caveat: 'Indikatif, bukan bukti pasti',
          provenance: { kind: 'AI_GENERATED', source: 'c2pa', generator: 'OpenAI', camera: '', c2pa: true },
          detector: { name: 'sightengine', score: 98 }
        }
      }
    });
    expect(d.pillars.ai.provenance?.kind).toBe('AI_GENERATED');
    expect(d.pillars.ai.detector?.score).toBe(98);

    const bad = minimalDossier();
    (bad.pillars.ai as Record<string, unknown>).provenance = { kind: 'MAYBE', source: '', generator: '', camera: '', c2pa: false };
    expect(() => parseDossier(bad)).toThrow();
  });

  it('reads the text-check evidence of a marked area, null when absent', () => {
    const base = minimalDossier();
    const ela = {
      available: true,
      score: 91,
      reliability: 1,
      reliability_note: null,
      anomalies: [
        { bounding_box: [400, 40, 470, 600], extracted_text: 'Rp 2.000', reason: 'x', evidence: 91 },
        { bounding_box: [10, 10, 20, 20], evidence: null },
        { bounding_box: [10, 10, 20, 20] }
      ]
    };
    const d = parseDossier({ ...base, pillars: { ...base.pillars, ela } });
    expect(d.pillars.ela.anomalies.map((a) => a.evidence)).toEqual([91, null, null]);
    const bad = { ...base, pillars: { ...base.pillars, ela: { ...ela, anomalies: [{ bounding_box: [0, 0, 1, 1], evidence: 150 }] } } };
    expect(() => parseDossier(bad)).toThrow();
  });

  const video = (frames: Array<{ at_sec: number; thumb: string }>) => ({
    status: 'OK',
    reason: '',
    platform: 'tiktok',
    duration_sec: 47.5,
    upload_date: '2026-11-07',
    platform_ai_label: true,
    frames,
    audio_analyzed: true,
    summary: 'Air keruh menerjang jalan kota.',
    transcript: 'Ini Jakarta pagi ini.',
    // The server serialises an empty Go slice as null.
    manipulation: { score: 14, indicators: null },
    context: { score: 58, indicators: ['Frame 3: rambu jalan tidak berbahasa Indonesia'] }
  });

  it('reads the video analysis of a social video link', () => {
    const d = parseDossier(minimalDossier({ video: video([{ at_sec: 8, thumb: 'data:image/jpeg;base64,/9j/4AAQ' }]) }));
    expect(d.video?.status).toBe('OK');
    expect(d.video?.frames).toEqual([{ at_sec: 8, thumb: 'data:image/jpeg;base64,/9j/4AAQ' }]);
    expect(d.video?.manipulation.indicators).toEqual([]);
    expect(d.video?.context.score).toBe(58);
    const unavailable = { ...video([]), status: 'UNAVAILABLE', reason: 'private', frames: null };
    expect(parseDossier(minimalDossier({ video: unavailable })).video?.frames).toEqual([]);
  });

  it('reads the creator disclosure and the web evidence of a video', () => {
    const v = {
      ...video([]),
      creator_disclosure: { level: 'strong', phrase: '#aivideo' },
      web_evidence: [{ title: '[HOAKS] Video ini buatan AI', url: 'https://turnbackhoax.id/x', publisher: 'turnbackhoax.id', says: 'AI' }]
    };
    const d = parseDossier(minimalDossier({ video: v }));
    expect(d.video?.creator_disclosure).toEqual({ level: 'strong', phrase: '#aivideo' });
    expect(d.video?.web_evidence?.[0]?.says).toBe('AI');
    // A server that predates the fields still parses.
    expect(parseDossier(minimalDossier({ video: video([]) })).video?.creator_disclosure).toBeUndefined();
  });

  it('rejects web evidence that is not an https link or has an unknown verdict', () => {
    const item = { title: 't', url: 'https://example.org/a', publisher: 'p', says: 'AI' };
    for (const bad of [
      { ...item, url: 'javascript:alert(1)' },
      { ...item, url: 'http://example.org/a' },
      { ...item, says: 'MAYBE' }
    ]) {
      expect(() => parseDossier(minimalDossier({ video: { ...video([]), web_evidence: [bad] } }))).toThrow();
    }
    expect(() => parseDossier(minimalDossier({ video: { ...video([]), creator_disclosure: { level: 'huge', phrase: '' } } }))).toThrow();
  });

  it('maps a missing video to null, as for every result made before videos', () => {
    expect(parseDossier(minimalDossier()).video).toBeNull();
    expect(parseDossier(minimalDossier({ video: null })).video).toBeNull();
  });

  it('rejects a frame thumbnail that is not a JPEG data URL', () => {
    for (const thumb of [
      'https://cdn.example/frame.jpg',
      'data:image/png;base64,iVBORw0KGgo',
      'data:image/svg+xml;base64,PHN2Zz4=',
      ' data:image/jpeg;base64,/9j/'
    ]) {
      expect(() => parseDossier(minimalDossier({ video: video([{ at_sec: 0, thumb }]) })), thumb).toThrow();
    }
  });

  it('raises SchemaVersionError for a newer contract', () => {
    expect(() => parseDossier(minimalDossier({ schema_version: 3 }))).toThrow(SchemaVersionError);
  });

  it('raises SchemaVersionError with the update message', () => {
    try {
      parseDossier(minimalDossier({ schema_version: 99 }));
    } catch (err) {
      expect((err as Error).message).toBe('Perbarui aplikasi');
    }
  });
});
