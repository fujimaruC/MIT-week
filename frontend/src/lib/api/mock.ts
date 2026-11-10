/**
 * Frontend-only stand-in for the Go audit API.
 *
 * It is intentionally honest: every dossier it builds is `is_demo: true`, the
 * UI shows a "Mode pratinjau" badge, and shared preview tokens are rendered as
 * "Laporan contoh" rather than "Terverifikasi AMBA".
 *
 * Replace this module with the real transport when the backend is wired up;
 * the AuditSession already speaks the same SSE event shape.
 */
import { parseDossier, type DossierParsed } from './schemas';
import type { FactStatus, Status, Tactic } from './types';
import type { SSEEvent } from '../sse';

export type MockKind = 'url' | 'file';

export interface MockPayload {
  url?: string;
  file?: File;
}

export interface MockTokenPayload {
  preview: true;
  id: string;
  ts: string;
  status: Status;
  score: number | null;
  title: string;
  claim?: string;
  srcs?: Array<{ pub: string; url: string; m: string }>;
}

const TOKEN_PREFIX = 'mock.v1.';

/* ------------------------------------------------------------------ tokens */

export function isMockToken(token: string): boolean {
  return token.startsWith(TOKEN_PREFIX);
}

function base64urlEncode(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function base64urlDecode(value: string): string {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/');
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
  const binary = atob(padded);
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

/** Decodes a preview token. Returns null for anything that is not one. */
export function decodeMockToken(token: string): MockTokenPayload | null {
  if (!isMockToken(token)) return null;
  try {
    const payload = JSON.parse(base64urlDecode(token.slice(TOKEN_PREFIX.length))) as MockTokenPayload;
    if (!payload || payload.preview !== true || typeof payload.id !== 'string') return null;
    return payload;
  } catch {
    return null;
  }
}

function makeToken(dossier: Omit<DossierParsed, 'share_token'>): string {
  const payload: MockTokenPayload = {
    preview: true,
    id: dossier.id,
    ts: dossier.timestamp,
    status: dossier.verdict.status,
    score: dossier.verdict.score,
    title: dossier.verdict.short_title,
    claim: dossier.pillars.fact.claim || undefined,
    srcs: dossier.pillars.fact.sources.slice(0, 3).map((s) => ({
      pub: s.publisher,
      url: s.url,
      m: s.match
    }))
  };
  return TOKEN_PREFIX + base64urlEncode(JSON.stringify(payload));
}

/* ------------------------------------------------------------- heuristics */

const SHORTENERS = ['bit.ly', 'tinyurl.com', 't.co', 'is.gd', 'cutt.ly', 'shorturl.at', 's.id'];
const LOOKALIKE_BRANDS = [
  'whatsapp',
  'instagram',
  'facebook',
  'bca',
  'bri',
  'mandiri',
  'dana',
  'ovo',
  'gopay',
  'shopee',
  'tokopedia',
  'telkomsel',
  'pln'
];

interface LinkSignals {
  score: number;
  status: Status;
  finalUrl: string;
  redirectChain: string[];
  reasons: Array<{ code: string; text: string }>;
}

function analyzeUrl(raw: string): LinkSignals {
  const reasons: Array<{ code: string; text: string }> = [];
  let host = '';
  let finalUrl = raw;
  let pathAndQuery = '';

  try {
    const parsed = new URL(raw);
    host = parsed.hostname.toLowerCase();
    finalUrl = parsed.href;
    pathAndQuery = `${parsed.pathname}${parsed.search}`;
  } catch {
    reasons.push({ code: 'MALFORMED', text: 'Alamat tautan disusun tidak wajar.' });
  }

  if (host.startsWith('xn--') || host.includes('.xn--')) {
    reasons.push({ code: 'PUNYCODE', text: 'Domain memakai huruf yang mirip nama situs lain.' });
  }
  if (SHORTENERS.some((shortener) => host === shortener || host.endsWith(`.${shortener}`))) {
    reasons.push({ code: 'SHORTENER', text: 'Tautan pendek menyembunyikan alamat tujuan.' });
  }
  if (LOOKALIKE_BRANDS.some((brand) => host.includes(brand) && !host.endsWith(`.${brand}.com`))) {
    reasons.push({ code: 'LOOKALIKE_BRAND', text: 'Domain meniru nama layanan populer.' });
  }
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host)) {
    reasons.push({ code: 'IP_ADDRESS', text: 'Alamat memakai nomor IP, bukan nama situs.' });
  }
  if (/(login|masuk|verifikasi|verify|akun|password|otp|pin|klaim|hadiah|promo)/i.test(`${host}${pathAndQuery}`)) {
    reasons.push({ code: 'CREDENTIAL_REQUEST', text: 'Alamat meminta data akun atau data pribadi.' });
  }
  if (raw.startsWith('http://')) {
    reasons.push({ code: 'NO_TLS', text: 'Tautan tidak memakai koneksi aman (https).' });
  }

  const score = reasons.length === 0 ? 8 : Math.min(95, 18 + reasons.length * 20);
  return {
    score,
    status: score >= 61 ? 'TAMPERED' : score >= 21 ? 'SUSPICIOUS' : 'SAFE',
    finalUrl,
    redirectChain: [finalUrl],
    reasons
  };
}

function basisFor(available: { ai: boolean; fact: boolean; link: boolean; ela: boolean }) {
  const basis: Array<'ela' | 'ai' | 'fact' | 'link'> = [];
  if (available.ela) basis.push('ela');
  if (available.ai) basis.push('ai');
  if (available.fact) basis.push('fact');
  if (available.link) basis.push('link');
  return basis;
}

function notCheckedFor(available: { ai: boolean; fact: boolean; link: boolean; ela: boolean }) {
  const all: Array<'ela' | 'ai' | 'fact' | 'link'> = ['ela', 'ai', 'fact', 'link'];
  return all.filter((key) => !available[key]);
}

function simpleSentence(status: Status, fact: FactStatus): string {
  if (fact === 'CONTRADICTS') return 'Kabar ini dibantah sumber terpercaya. Jangan disebarkan.';
  if (status === 'TAMPERED') return 'Pesan ini memakai pola yang sering muncul pada penipuan. Jangan langsung percaya.';
  if (status === 'SUSPICIOUS') return 'Ada tanda yang perlu diwaspadai. Cek dulu sebelum menyebarkan.';
  if (status === 'SAFE') return 'Tidak ditemukan tanda bahaya, tapi tetap bijak sebelum menyebarkan.';
  return 'Informasinya belum cukup untuk dipastikan.';
}

function explanationFor(status: Status, fact: FactStatus, tactics: Tactic[], link: LinkSignals | null): string {
  if (fact === 'CONTRADICTS') {
    return 'Sumber terverifikasi membantah klaim utama pada konten ini. Pesan juga memakai pola yang sering muncul pada hoaks, seperti tekanan waktu atau ajakan menyebarkan. Ini data contoh dari mode pratinjau.';
  }
  if (status === 'TAMPERED') {
    return 'Konten memakai beberapa pola yang sering muncul pada penipuan: permintaan data pribadi, tekanan waktu, atau tautan yang menyembunyikan tujuan. Jangan klik, jangan kirim data, dan periksa ke sumber resmi. Ini data contoh dari mode pratinjau.';
  }
  if (status === 'SUSPICIOUS') {
    const parts: string[] = [];
    if (tactics.length > 0) parts.push('Ada pola bujukan yang perlu diwaspadai');
    if (link) parts.push('tautan memiliki tanda yang belum jelas tujuannya');
    return `${parts.join(', ') || 'Ada tanda yang belum bisa dipastikan'}. Periksa sumber resminya sebelum menyebarkan. Ini data contoh dari mode pratinjau.`;
  }
  if (status === 'SAFE') {
    return 'Tidak ditemukan pola penipuan yang jelas pada konten ini. Tetap periksa ke sumber resmi, terutama bila pesan meminta tindakan. Ini data contoh dari mode pratinjau.';
  }
  return 'Konten ini belum cukup untuk dinilai. Tidak ada klaim yang bisa dicocokkan dengan sumber terverifikasi. Ini data contoh dari mode pratinjau.';
}

function baseDossier(
  inputType: 'URL' | 'FILE',
  sourceReference: string,
  overrides: {
    status: Status;
    score: number | null;
    shortTitle: string;
    explanation: string;
    explanationSimple: string;
    basis: Array<'ela' | 'ai' | 'fact' | 'link'>;
    notChecked: Array<'ela' | 'ai' | 'fact' | 'link'>;
    floors: string[];
    pillars: DossierParsed['pillars'];
    tactics: Tactic[];
    sourceMetadata?: DossierParsed['source_metadata'];
    video?: DossierParsed['video'];
  }
): DossierParsed {
  const withoutToken: Omit<DossierParsed, 'share_token'> = {
    schema_version: 2,
    id: `AMBA-PREVIEW-${Math.random().toString(36).slice(2, 8).toUpperCase()}`,
    timestamp: new Date().toISOString(),
    input_type: inputType,
    source_reference: sourceReference,
    degraded: false,
    is_demo: true,
    verdict: {
      score: overrides.score,
      status: overrides.status,
      short_title: overrides.shortTitle,
      explanation: overrides.explanation.slice(0, 400),
      explanation_simple: overrides.explanationSimple.slice(0, 140),
      basis: overrides.basis,
      not_checked: overrides.notChecked,
      floors_applied: overrides.floors
    },
    pillars: overrides.pillars,
    tactics: overrides.tactics,
    source_metadata: overrides.sourceMetadata ?? null,
    ocr_text: '',
    video: overrides.video ?? null
  };

  return parseDossier({ ...withoutToken, share_token: makeToken(withoutToken) });
}

function unavailablePillar() {
  return { available: false, score: 0, reliability: 0, reliability_note: null, anomalies: [] };
}

const NO_PROVENANCE = { kind: 'NONE' as const, source: '' as const, generator: '', camera: '', c2pa: false };

function unavailableAI() {
  return {
    available: false,
    score: 0,
    reliability: 0,
    indicators: [],
    caveat: 'Indikatif, bukan bukti pasti',
    provenance: NO_PROVENANCE,
    detector: null
  };
}

function unavailableFact() {
  return { available: false, claim: '', status: 'UNVERIFIED' as const, sources: [] };
}

function unavailableLink() {
  return {
    available: false,
    score: 0,
    final_url: '',
    redirect_chain: [],
    reasons: []
  };
}

/* ---------------------------------------------------------------- video */

type VideoPlatform = 'tiktok' | 'youtube' | 'x';

/** A TikTok, YouTube or X video link; any other link stays a plain link check. */
export function videoPlatform(raw: string): VideoPlatform | null {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  const host = url.hostname.toLowerCase().replace(/^(www|m|mobile)\./, '');
  const path = url.pathname;
  if (host === 'tiktok.com') return /^\/@[^/]+\/video\/\d+/.test(path) ? 'tiktok' : null;
  if (host === 'vm.tiktok.com' || host === 'vt.tiktok.com') return path.length > 1 ? 'tiktok' : null;
  if (host === 'youtube.com') {
    return (path === '/watch' && url.searchParams.has('v')) || path.startsWith('/shorts/') ? 'youtube' : null;
  }
  if (host === 'youtu.be') return path.length > 1 ? 'youtube' : null;
  if (host === 'x.com' || host === 'twitter.com') return /^\/\w+\/status\/\d+/.test(path) ? 'x' : null;
  return null;
}

const JPEG_PREFIX = 'data:image/jpeg;base64,';
/** A 16x9 muddy-brown JPEG for a runtime without a 2D canvas (tests, locked-down browsers). */
const FALLBACK_FRAME = `${JPEG_PREFIX}/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDABALDA4MChAODQ4SERATGCgaGBYWGDEjJR0oOjM9PDkzODdASFxOQERXRTc4UG1RV19iZ2hnPk1xeXBkeFxlZ2P/2wBDARESEhgVGC8aGi9jQjhCY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2P/wAARCAAJABADASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAT/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFQEBAQAAAAAAAAAAAAAAAAAAAwT/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oADAMBAAIRAxEAPwCMBMZ//9k=`;

/**
 * One example keyframe, drawn at runtime so no image ships with the app: a
 * street under a grey sky, an on-screen caption, and flood water that rises
 * with `rise` (0 to 1) from frame to frame.
 */
function drawFrame(width: number, height: number, rise: number): string {
  try {
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) return FALLBACK_FRAME;
    ctx.fillStyle = '#9fb0bf';
    ctx.fillRect(0, 0, width, height);
    ctx.fillStyle = '#4f5965';
    for (let i = 0; i < 5; i++) {
      const top = height * (0.32 + 0.08 * ((i * 3) % 4));
      ctx.fillRect((width / 5) * i + 2, top, width / 5 - 4, height - top);
    }
    const water = height * (0.82 - 0.36 * rise);
    ctx.fillStyle = '#7a6544';
    ctx.fillRect(0, water, width, height - water);
    const size = Math.round(Math.min(width, height) * 0.08);
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(width * 0.08, height * 0.07, width * 0.84, size * 1.8);
    ctx.fillStyle = '#111111';
    ctx.font = `bold ${size}px sans-serif`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('JAKARTA PAGI INI', width / 2, height * 0.07 + size * 0.9, width * 0.8);
    const url = canvas.toDataURL('image/jpeg', 0.7);
    return url.startsWith(JPEG_PREFIX) ? url : FALLBACK_FRAME;
  } catch {
    return FALLBACK_FRAME;
  }
}

const VIDEO_FRAME_TIMES = [0, 8, 16, 24, 32, 40];

/**
 * A social video link: an example flood clip whose frames look AI-made and
 * whose footage only partly matches its caption, so the result page shows
 * every part of the video card (AI label, frames, transcript, three levels).
 */
function videoDossier(url: string, platform: VideoPlatform): DossierParsed {
  const link = analyzeUrl(url);
  // TikTok and Shorts are portrait; the server's thumbnails keep the long side at 240 px.
  const [width, height] = platform === 'tiktok' || url.includes('/shorts/') ? [135, 240] : [240, 135];
  const available = { ela: false, ai: true, fact: false, link: true };
  const last = VIDEO_FRAME_TIMES.length - 1;

  return baseDossier('URL', url, {
    status: 'SUSPICIOUS',
    score: 52,
    shortTitle: 'Video tampak buatan AI',
    explanation:
      'Beberapa frame memperlihatkan ciri video buatan AI, seperti tulisan berupa huruf acak dan benda yang berubah bentuk. Sebagian isi rekaman juga tidak cocok dengan keterangannya. Ini data contoh dari mode pratinjau.',
    explanationSimple: 'Video ini tampak buatan AI. Cek dulu sebelum menyebarkan.',
    basis: basisFor(available),
    notChecked: notCheckedFor(available),
    floors: ['AI_LIKELY'],
    sourceMetadata: {
      status: 'OK',
      platform,
      via: 'oembed',
      title: 'Detik-detik banjir bandang menerjang Jakarta pagi ini!! Sebarkan ke keluarga',
      author: 'contoh.simulasi',
      description: '',
      site_name: ''
    },
    pillars: {
      ela: unavailablePillar(),
      ai: {
        available: true,
        score: 82,
        reliability: 0.6,
        indicators: [
          'Frame 2: tulisan di papan toko berupa huruf acak',
          'Frame 4: mobil berubah bentuk saat terseret arus',
          'Frame 6: gerak air terlalu mulus dan berulang'
        ],
        caveat: 'Indikatif, bukan bukti pasti. Mode pratinjau tidak menonton videonya.',
        provenance: null,
        detector: null
      },
      fact: unavailableFact(),
      link: {
        available: true,
        score: link.score,
        final_url: link.finalUrl,
        redirect_chain: link.redirectChain,
        reasons: link.reasons
      }
    },
    tactics: [],
    video: {
      status: 'OK',
      reason: '',
      platform,
      duration_sec: 47,
      upload_date: '2026-11-07',
      platform_ai_label: platform !== 'x',
      frames: VIDEO_FRAME_TIMES.map((at_sec, i) => ({ at_sec, thumb: drawFrame(width, height, i / last) })),
      audio_analyzed: true,
      summary:
        'Air keruh menerjang jalan kota dan menyeret beberapa mobil. Di layar ada teks "JAKARTA PAGI INI", dan seorang pria terdengar panik sambil meminta video ini disebarkan.',
      transcript:
        'Ya Allah, lihat ini, airnya sudah setinggi atap mobil. Ini Jakarta pagi ini, saudara-saudara. Tolong sebarkan ke keluarga supaya semua waspada.',
      manipulation: { score: 14, indicators: [] },
      context: {
        score: 58,
        indicators: [
          'Frame 3: rambu jalan tidak berbahasa Indonesia, padahal keterangan menyebut Jakarta',
          'Frame 5: bentuk bangunan dan kendaraan tidak seperti di Jakarta'
        ]
      },
      creator_disclosure: { level: 'weak', phrase: '#ai' },
      web_evidence: [
        {
          title: '[HOAKS] Video banjir Jakarta pagi ini adalah hasil AI',
          url: 'https://example.org/cek-fakta/banjir-jakarta-ai',
          publisher: 'turnbackhoax.id',
          says: 'AI'
        }
      ],
      // Preview data only: the mode never watches the video or searches.
      w5h1: {
        what: 'Banjir besar menyeret beberapa mobil di jalan kota.',
        who: 'Seorang pria merekam dan bersuara panik; tidak ada tokoh yang dikenali.',
        when: 'Disebut "pagi ini"; tanggal kejadian tidak terlihat.',
        where: 'Disebut Jakarta, tetapi rambu jalan tidak berbahasa Indonesia.',
        why: 'Tidak dijelaskan di video.',
        how: 'Air keruh naik setinggi atap mobil dan mengalir deras.'
      },
      research: {
        query: 'video banjir Jakarta mobil terseret',
        status: 'FOUND',
        sources: [
          {
            title: '[HOAKS] Video banjir Jakarta pagi ini adalah hasil AI',
            url: 'https://example.org/cek-fakta/banjir-jakarta-ai',
            domain: 'turnbackhoax.id',
            snippet: 'Contoh data pratinjau: video ini dinyatakan bukan rekaman banjir Jakarta.',
            published: '2026-11-08'
          }
        ]
      }
    }
  });
}

/* ------------------------------------------------------------- builders */

function urlDossier(payload: MockPayload): DossierParsed {
  const url = (payload.url ?? '').trim();
  const platform = videoPlatform(url);
  if (platform) return videoDossier(url, platform);
  const link = analyzeUrl(url);
  const status = link.status;
  // The browser preview cannot fetch the link's preview image, so there is
  // no image for the AI check, as on the server when a link has none.
  const available = { ela: false, ai: false, fact: false, link: true };
  const tactics: Tactic[] = [];

  return baseDossier('URL', url, {
    status,
    score: link.score,
    shortTitle:
      status === 'TAMPERED'
        ? 'Tautan meniru halaman masuk'
        : status === 'SUSPICIOUS'
          ? 'Tautan perlu diperiksa'
          : 'Tautan tanpa tanda bahaya',
    explanation: explanationFor(status, 'UNVERIFIED', tactics, link),
    explanationSimple: simpleSentence(status, 'UNVERIFIED'),
    basis: basisFor(available),
    notChecked: notCheckedFor(available),
    // Mirrors the Go scorer: >= 90 is LINK_MALICIOUS, otherwise a link-only
    // verdict of >= 45 (scoring.LinkStrongMin) is LINK_STRONG.
    floors: link.score >= 90 ? ['LINK_MALICIOUS'] : link.score >= 45 ? ['LINK_STRONG'] : [],
    // The browser preview cannot fetch other sites, which is exactly the
    // "could not read" case the result page explains.
    sourceMetadata: {
      status: 'UNAVAILABLE',
      platform: 'web',
      via: '',
      title: '',
      author: '',
      description: '',
      site_name: ''
    },
    pillars: {
      ela: unavailablePillar(),
      ai: unavailableAI(),
      fact: unavailableFact(),
      link: {
        available: true,
        score: link.score,
        final_url: link.finalUrl,
        redirect_chain: link.redirectChain,
        reasons: link.reasons
      }
    },
    tactics
  });
}

function fileDossier(payload: MockPayload): DossierParsed {
  const type = payload.file?.type ?? 'image/png';
  const reliability = type === 'image/png' ? 0.55 : type === 'image/webp' ? 0.62 : 0.72;
  const status: Status = 'SUSPICIOUS';
  const score = 56;
  const available = { ela: true, ai: true, fact: false, link: false };
  const reliabilityNote =
    type === 'image/png'
      ? 'PNG dan tangkapan layar sudah disimpan ulang, sehingga analisis kompresi kurang andal.'
      : 'Analisis kompresi hanya menunjukkan area yang perlu dilihat, bukan bukti suntingan.';

  return baseDossier('FILE', 'Gambar yang diunggah', {
    status,
    score,
    shortTitle: 'Gambar perlu diperiksa',
    explanation:
      'Ada area dengan perbedaan kompresi yang menonjol, tetapi analisis gambar tidak bisa memastikan suntingan. Periksa konteks gambar dan sumber aslinya. Ini data contoh dari mode pratinjau.',
    explanationSimple: simpleSentence(status, 'UNVERIFIED'),
    basis: basisFor(available),
    notChecked: notCheckedFor(available),
    floors: [],
    pillars: {
      ela: {
        available: true,
        score: 58,
        reliability,
        reliability_note: reliabilityNote,
        anomalies: [
          {
            bounding_box: [280, 300, 520, 720],
            reason: 'Area ini memiliki perbedaan kompresi yang menonjol (contoh).',
            // Preview mode does not read the image, so nothing was measured.
            evidence: null
          }
        ]
      },
      ai: {
        available: true,
        score: 30,
        reliability: 0.5,
        indicators: [],
        caveat: 'Indikatif, bukan bukti pasti. Mode pratinjau tidak membaca isi gambar.',
        provenance: NO_PROVENANCE,
        detector: null
      },
      fact: unavailableFact(),
      link: unavailableLink()
    },
    tactics: []
  });
}

/** Builds a deterministic-enough example dossier for any input. */
export function buildMockDossier(kind: MockKind, payload: MockPayload): DossierParsed {
  switch (kind) {
    case 'file':
      return fileDossier(payload);
    case 'url':
    default:
      return urlDossier(payload);
  }
}

/* --------------------------------------------------------------- stream */

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('Aborted', 'AbortError'));
      return;
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      clearTimeout(timer);
      reject(new DOMException('Aborted', 'AbortError'));
    };
    signal.addEventListener('abort', onAbort, { once: true });
  });
}

/**
 * Emits the same stage → partial → verdict → done sequence as the Go SSE
 * endpoint. `timing` lets tests run without waiting.
 */
export async function* mockAuditStream(
  kind: MockKind,
  payload: MockPayload,
  signal: AbortSignal,
  timing: { step?: number; settle?: number } = {}
): AsyncGenerator<SSEEvent, void, unknown> {
  const dossier = buildMockDossier(kind, payload);
  const step = timing.step ?? 220;
  const settle = timing.settle ?? 90;
  const order: Array<'ela' | 'ai' | 'fact' | 'link'> = ['ela', 'ai', 'fact', 'link'];

  for (const pillar of order) {
    if (signal.aborted) return;
    if (!dossier.pillars[pillar].available) {
      yield { type: 'stage', pillar, status: 'skipped' };
      if (step > 0) await delay(settle, signal);
      continue;
    }
    yield { type: 'stage', pillar, status: 'running' };
    if (step > 0) await delay(step, signal);
    yield { type: 'partial', pillar, data: dossier.pillars[pillar] };
    if (settle > 0) await delay(settle, signal);
    yield { type: 'stage', pillar, status: 'done' };
  }

  yield { type: 'verdict', data: dossier };
  if (settle > 0) await delay(settle, signal);
  yield { type: 'done' };
}
