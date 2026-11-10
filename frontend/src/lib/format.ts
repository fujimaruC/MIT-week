import type { Status } from './api/types';
import { t, bcp47 } from './i18n/index.svelte';

/**
 * Status labels. The Indonesian wording comes from TECH_SPEC §3.3. They never
 * claim certainty: SAFE is "no danger signs found", not "safe".
 */
export function statusLabel(status: Status): string {
  return t(`status.${status}`);
}

/**
 * The score is a 0-100 RISK score, not a probability or confidence.
 * Never render it with a percent sign.
 */
export function formatScore(score: number | null): string {
  return score === null ? t('score.none') : t('score.of100', { score });
}

const PLATFORMS: Record<string, string> = {
  youtube: 'YouTube',
  tiktok: 'TikTok',
  x: 'X (Twitter)',
  instagram: 'Instagram',
  facebook: 'Facebook',
  threads: 'Threads'
};

/** Display name of a social platform code; "" for the open web or an unknown code. */
export function platformName(platform: string): string {
  return Object.hasOwn(PLATFORMS, platform) ? PLATFORMS[platform] : '';
}

/** A video clock, m:ss (h:mm:ss from one hour), in whole seconds rounded down like a player. */
export function formatClock(seconds: number): string {
  const total = Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds) : 0;
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = String(total % 60).padStart(2, '0');
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`;
}

/** A YYYY-MM-DD date in the interface language; "" when empty or not a real date. */
export function formatDate(iso: string): string {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(iso)) return '';
  const date = new Date(`${iso}T00:00:00Z`);
  // Date rolls 2026-02-31 over into March; a real date survives the round trip.
  if (Number.isNaN(date.getTime()) || date.toISOString().slice(0, 10) !== iso) return '';
  return new Intl.DateTimeFormat(bcp47(), { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(date);
}
