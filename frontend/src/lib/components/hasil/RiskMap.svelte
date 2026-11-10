<script lang="ts">
  import { t, type MessageKey } from '$lib/i18n/index.svelte';
  import type { DossierParsed } from '$lib/api/schemas';

  /**
   * AMBA's own chart: every check's score on one 0-100 scale, with the line
   * where that check starts to flag (the thresholds in backend scoring.go).
   * A bar that crosses its line takes the danger colour; the others keep
   * the one data hue. The fact check has no score, so it reads as a word.
   */
  interface Props {
    dossier: DossierParsed;
  }
  let { dossier }: Props = $props();

  type Row = { key: string; label: MessageKey; score: number | null; threshold: number };

  const rows = $derived.by((): Row[] => {
    const p = dossier.pillars;
    const video = dossier.video?.status === 'OK' ? dossier.video : null;
    const out: Row[] = [];
    out.push({ key: 'ai', label: 'risk.ai', score: p.ai.available ? p.ai.score : null, threshold: video ? 55 : 80 });
    if (video) {
      out.push({ key: 'manip', label: 'risk.manip', score: video.manipulation.score, threshold: 70 });
      out.push({ key: 'ctx', label: 'risk.ctx', score: video.context.score, threshold: 85 });
    }
    if (dossier.input_type === 'FILE') {
      out.push({ key: 'ela', label: 'risk.ela', score: p.ela.available ? p.ela.score : null, threshold: 70 });
    }
    out.push({ key: 'link', label: 'risk.link', score: p.link.available ? p.link.score : null, threshold: 45 });
    return out;
  });

  const factKey = $derived.by((): MessageKey | null => {
    const f = dossier.pillars.fact;
    if (!f.available) return null;
    return `risk.fact.${f.status}` as MessageKey;
  });
  const factTone = $derived(
    dossier.pillars.fact.available
      ? ({ CONTRADICTS: 'danger', SUPPORTS: 'ok', UNVERIFIED: 'neutral' } as const)[dossier.pillars.fact.status]
      : 'neutral'
  );
</script>

<section class="risk" aria-labelledby="risk-title">
  <header class="risk-head">
    <h2 id="risk-title">{t('risk.title')}</h2>
    <p class="note">{t('risk.note')}</p>
  </header>
  <ul class="rows">
    {#each rows as row (row.key)}
      {@const over = row.score !== null && row.score >= row.threshold}
      <li class="row" class:over>
        <span class="name">{t(row.label)}</span>
        <span class="track" aria-hidden="true">
          {#if row.score !== null}
            <span class="fill" style:width="{Math.max(row.score, 1.5)}%"></span>
          {/if}
          <span class="tick" style:left="{row.threshold}%"><span>{t('risk.threshold', { n: row.threshold })}</span></span>
        </span>
        <span class="value tabular">
          {#if row.score === null}<span class="dash">—</span><span class="sr-only">{t('risk.notRun')}</span>
          {:else}{row.score}<small>/100</small>{/if}
        </span>
      </li>
    {/each}
    <li class="row row--fact tone-{factTone}">
      <span class="name">{t('risk.fact')}</span>
      <span class="fact-word">{factKey ? t(factKey) : t('risk.notRun')}</span>
    </li>
  </ul>
</section>

<style>
  .risk { display: grid; gap: var(--space-4); }
  .risk-head { display: grid; gap: 4px; }
  h2 { font-size: var(--text-base); font-weight: 500; }
  .note { margin: 0; color: var(--muted); font-size: var(--text-sm); }
  .rows { list-style: none; margin: 0; padding: 0; display: grid; }
  .row {
    display: grid;
    grid-template-columns: minmax(8.5rem, 11rem) minmax(0, 1fr) 4.5rem;
    align-items: center;
    gap: var(--space-4);
    min-height: 52px;
    border-top: 1px solid var(--border-soft);
  }
  .row:first-child { border-top: 0; }
  .name { font-size: var(--text-sm); color: var(--body-2); }
  .track {
    position: relative;
    height: 10px;
    border-radius: 999px;
    background: var(--surface-2);
  }
  .fill {
    position: absolute;
    inset: 0 auto 0 0;
    border-radius: inherit;
    background: var(--data-1);
    transform-origin: left center;
  }
  .over .fill { background: var(--err); }
  .tick {
    position: absolute;
    top: -6px;
    bottom: -6px;
    width: 0;
    border-left: 1px dashed var(--border-strong);
  }
  .tick span {
    position: absolute;
    top: -18px;
    left: 50%;
    transform: translateX(-50%);
    color: var(--muted);
    font-size: 0.6875rem;
    white-space: nowrap;
  }
  .row:not(:first-child) .tick span { display: none; }
  .value { white-space: nowrap; text-align: right; font-size: var(--text-lg); color: var(--ink); }
  .over .value { color: var(--err); }
  .value small { color: var(--muted); font-size: var(--text-xs); margin-left: 2px; }
  .dash { color: var(--muted); }
  .row--fact { grid-template-columns: minmax(8.5rem, 11rem) minmax(0, 1fr); }
  .fact-word { font-size: var(--text-sm); font-weight: 500; }
  .tone-danger .fact-word { color: var(--err); }
  .tone-ok .fact-word { color: var(--ok); }
  .tone-neutral .fact-word { color: var(--body-2); }

  /* The bars grow once the panel is in view. */
  :global([data-motion='in']) .fill { animation: grow 900ms var(--ease-out) both; }
  @keyframes grow { from { transform: scaleX(0); } }

  @media (max-width: 30rem) {
    .row { grid-template-columns: minmax(0, 1fr) auto; row-gap: 6px; padding-block: 8px; }
    .track { grid-column: 1 / -1; grid-row: 2; }
    .tick span { display: none; }
    .row--fact { grid-template-columns: minmax(0, 1fr); }
  }
</style>
