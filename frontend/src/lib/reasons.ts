import type { DossierParsed } from '$lib/api/schemas';
import { t } from '$lib/i18n/index.svelte';
import { aiLines, linkReason } from '$lib/i18n/verdict';
import tacticsId from '$lib/data/tactics.json';

/** Below this a signal is background noise, not a reason. */
const NOTABLE = 40;
const MAX_REASONS = 4;

/**
 * The two to four things that moved the verdict, in plain words, strongest
 * first. Built only from fields the server already sent, so a reason is never
 * invented. Server-written sentences stay in Indonesian under the EN toggle,
 * except where a code-based line exists.
 */
export function keyReasons(d: DossierParsed): string[] {
  const out: string[] = [];
  const add = (s: string | null | undefined): void => {
    const line = s?.trim();
    if (line && !out.includes(line)) out.push(line);
  };
  const { ela, ai, fact, link } = d.pillars;

  if (fact.available && fact.status === 'CONTRADICTS') add(t('why.fact'));

  if (link.available) for (const r of link.reasons.slice(0, 2)) add(linkReason(r.text));

  if (d.video?.platform_ai_label) add(t('video.aiLabel'));

  if (ai.available) {
    const prov = ai.provenance;
    const declared = prov?.kind === 'AI_GENERATED' || prov?.kind === 'AI_EDITED';
    if (declared || ai.score >= NOTABLE) {
      const lines = aiLines(ai, d.input_type, d.video?.status === 'OK');
      // The first line of a link check only says which image was judged.
      add(d.input_type === 'URL' ? lines[1] : lines[0]);
    }
  }

  if (d.video?.status === 'OK') {
    if (d.video.manipulation.score >= NOTABLE) add(d.video.manipulation.indicators[0] ?? t('why.videoEdited'));
    if (d.video.context.score >= NOTABLE) add(d.video.context.indicators[0] ?? t('why.videoContext'));
  }

  if (ela.available && ela.score >= 50 && ela.anomalies.length) add(t('ev.areas', { n: ela.anomalies.length }));

  if (d.tactics.length) {
    const labels: Record<string, { label: string }> = tacticsId;
    const names = [...new Set(d.tactics.map((x) => labels[x.code]?.label).filter(Boolean))];
    if (names.length) add(t('why.tactics', { list: names.slice(0, 3).join(', ') }));
  }

  if (out.length === 0) {
    if (d.verdict.status === 'SAFE') add(t('why.none'));
    else if (d.verdict.status === 'INSUFFICIENT') add(t('why.insufficient'));
    else add(t('why.mixed'));
  }
  // The trail behind the conclusion, pointed to last (Sir, 10 Nov 2026).
  const research = d.video?.status === 'OK' ? d.video.research : null;
  if (research?.status === 'FOUND' && research.sources.length) {
    out.splice(MAX_REASONS - 1, 0, t('why.research', { n: research.sources.length }));
  }
  return out.slice(0, MAX_REASONS);
}
