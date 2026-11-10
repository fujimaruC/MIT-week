import type { PillarKey } from './audit.svelte';
import { t } from './i18n/index.svelte';

/**
 * One honest line describing a pillar payload the server has already sent.
 *
 * Kept out of the Svelte component so the mapping can be tested without a
 * timing race: in mock mode the stream finishes in under a second, so an e2e
 * test can never reliably observe a mid-stream frame.
 *
 * Returns null when there is nothing truthful to say yet.
 */
export function pillarSummary(key: PillarKey, data: unknown): string | null {
  if (!data || typeof data !== 'object') return null;
  const d = data as Record<string, unknown>;
  if (d.available === false) return t('summary.unavailable');

  switch (key) {
    case 'ai':
      return typeof d.score === 'number' ? t('summary.ai', { n: d.score }) : null;
    case 'link':
      return typeof d.score === 'number' ? t('summary.link', { n: d.score }) : null;
    case 'ela':
      return typeof d.reliability === 'number'
        ? t('summary.ela', { n: Math.round(d.reliability * 100) })
        : null;
    case 'fact': {
      if (typeof d.status !== 'string') return null;
      if (d.status !== 'CONTRADICTS' && d.status !== 'SUPPORTS' && d.status !== 'UNVERIFIED') return null;
      return t(`summary.${d.status}`);
    }
  }
}
