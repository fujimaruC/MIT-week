import { firstHttpUrl } from './share-target';

/**
 * Reads what a person pasted into the link field. Share buttons put words
 * around the link ("Lihat video ini https://vt.tiktok.com/abc"), so the first
 * http(s) URL is pulled out. The server validates again; this is for fast,
 * friendly feedback, never the only guard.
 */
export type Platform = 'youtube' | 'tiktok' | 'instagram' | 'facebook' | 'x';

export type LinkProblem = 'empty' | 'scheme' | 'notUrl' | 'long';

export interface LinkReading {
  /** The link to check, or null when none could be read. */
  url: string | null;
  platform: Platform | null;
  /** True when the link was cut out of longer text. */
  extracted: boolean;
  problem: LinkProblem | null;
}

export const MAX_URL_LENGTH = 2048;

// Exact registrable domains. A subdomain of one of these counts
// ("vt.tiktok.com"); a lookalike ("tiktok.com.evil.io", "eviltiktok.com") does not.
const DOMAINS: Record<Platform, string[]> = {
  youtube: ['youtube.com', 'youtu.be'],
  tiktok: ['tiktok.com'],
  instagram: ['instagram.com', 'instagr.am'],
  facebook: ['facebook.com', 'fb.watch', 'fb.com'],
  x: ['x.com', 'twitter.com']
};

export function platformOf(url: string): Platform | null {
  let host: string;
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return null;
    host = parsed.hostname.toLowerCase().replace(/\.$/, '');
  } catch {
    return null;
  }
  for (const [platform, domains] of Object.entries(DOMAINS) as Array<[Platform, string[]]>) {
    if (domains.some((d) => host === d || host.endsWith(`.${d}`))) return platform;
  }
  return null;
}

const NONE: LinkReading = { url: null, platform: null, extracted: false, problem: null };

// "tiktok.com/@a/video/1" with no scheme: people type and paste this too.
const BARE_HOST = /^(?:www\.)?[a-z0-9-]+(?:\.[a-z0-9-]+)*\.[a-z]{2,}(?:[/?#]\S*)?$/i;
// Any other scheme (javascript:, data:, file:, ftp:, intent:) is refused.
const OTHER_SCHEME = /(?:^|[\s"'(<])(?:[a-z][a-z0-9+.-]{1,20}):(?!\/\/)\S*/i;

export function readLink(raw: string): LinkReading {
  const text = raw.trim();
  if (!text) return { ...NONE, problem: 'empty' };

  let url = firstHttpUrl(text);
  if (!url && !/\s/.test(text) && BARE_HOST.test(text)) url = `https://${text}`;
  if (!url) {
    const scheme = /^[a-z][a-z0-9+.-]*:/i.test(text) || OTHER_SCHEME.test(text);
    return { ...NONE, problem: scheme ? 'scheme' : 'notUrl' };
  }
  // "javascript:alert(1) https://ok.example" is not a link to forward.
  if (/^\s*(?:javascript|data|vbscript|file):/i.test(text)) return { ...NONE, problem: 'scheme' };
  if (url.length > MAX_URL_LENGTH) return { ...NONE, problem: 'long' };
  try {
    new URL(url);
  } catch {
    return { ...NONE, problem: 'notUrl' };
  }
  return { url, platform: platformOf(url), extracted: url !== text, problem: null };
}
